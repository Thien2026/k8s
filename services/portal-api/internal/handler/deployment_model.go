package handler

import (
	"strings"
)

type deploymentRow struct {
	ID                  int64               `json:"id"`
	Environment         string              `json:"environment"`
	ImageTag            string              `json:"image_tag"`
	Status              string              `json:"status"`
	BuildStatus         string              `json:"build_status"`
	RegistryStatus      string              `json:"registry_status"`
	DeployStatus        string              `json:"deploy_status"`
	RuntimeStatus       string              `json:"runtime_status"`
	ErrorPhase          string              `json:"error_phase,omitempty"`
	ErrorMessage        string              `json:"error_message,omitempty"`
	GitHubRunID         int64               `json:"github_run_id,omitempty"`
	GitHubRunURL        string              `json:"github_run_url,omitempty"`
	Image               string              `json:"image,omitempty"`
	RuntimeDetail       string              `json:"runtime_detail,omitempty"`
	RuntimeLog          string              `json:"runtime_log,omitempty"`
	RuntimeLogTruncated bool                `json:"runtime_log_truncated,omitempty"`
	PodName             string              `json:"pod_name,omitempty"`
	CreatedAt           string              `json:"created_at"`
	UpdatedAt           string              `json:"updated_at"`
	FinishedAt          string              `json:"finished_at,omitempty"`
	Stages              []deployStage       `json:"stages"`
	BuildSteps          []buildStepView     `json:"build_steps,omitempty"`
	BuildLog            string              `json:"build_log,omitempty"`
	BuildLogTruncated   bool                `json:"build_log_truncated,omitempty"`
	DeployLog           string              `json:"deploy_log,omitempty"`
	Live                bool                `json:"live,omitempty"`
	Serving             bool                `json:"serving,omitempty"`
	DeployLayout        string              `json:"deploy_layout,omitempty"`
	DeployProfile       string              `json:"deploy_profile,omitempty"`
	GitBranch           string              `json:"git_branch,omitempty"`
	DeployServices      []deployServiceSnap `json:"deploy_services,omitempty"`
	DeployImages        map[string]string   `json:"deploy_images,omitempty"`
	SmokeStatus         string              `json:"smoke_status,omitempty"`
	SmokeDetail         string              `json:"smoke_detail,omitempty"`
	RuntimeSignals      []runtimeSignalTier `json:"runtime_signals,omitempty"`
	HarborScan          *harborScanView     `json:"harbor_scan,omitempty"`
}

type buildStepView struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Number int    `json:"number,omitempty"`
}

type harborScanView struct {
	Status     string           `json:"status"`
	Severity   map[string]int   `json:"severity,omitempty"`
	Total      int              `json:"total"`
	Fixable    int              `json:"fixable"`
	Detail     string           `json:"detail,omitempty"`
	URL        string           `json:"url,omitempty"`
	Items      []harborVulnView `json:"items,omitempty"`
	ItemsTotal int              `json:"items_total,omitempty"`
}

type harborVulnView struct {
	ID         string `json:"id"`
	Severity   string `json:"severity"`
	Package    string `json:"package"`
	Version    string `json:"version"`
	FixVersion string `json:"fix_version,omitempty"`
	Status     string `json:"status,omitempty"`
}

type deployStage struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
	URL    string `json:"url,omitempty"`
	Error  string `json:"error,omitempty"`
}

func deploymentIsTerminal(d deploymentRow) bool {
	switch strings.ToLower(strings.TrimSpace(d.Status)) {
	case "success", "failed":
		return true
	default:
		return false
	}
}

// deploymentIsTrulyActive — deploy đang chạy thật (không tính bản failed kẹt build_status=running).
func deploymentIsTrulyActive(d deploymentRow) bool {
	if deploymentIsTerminal(d) {
		return false
	}
	if d.Live {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(d.Status), "in_progress") {
		return true
	}
	for _, st := range []string{d.BuildStatus, d.DeployStatus, d.RuntimeStatus} {
		switch stageStatus(st) {
		case "running", "pending":
			return true
		}
	}
	return false
}

func deploymentIsOngoingDeploy(d deploymentRow) bool {
	if !deploymentIsTrulyActive(d) || deploymentIsTerminal(d) {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(d.DeployStatus), "success") {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(d.DeployStatus), "running")
}

func stageStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "success", "ok", "live", "ready":
		return "success"
	case "failed", "error", "failure":
		return "failed"
	case "skipped", "skip":
		return "skipped"
	case "in_progress", "building", "deploying", "running", "pending", "queued", "waiting":
		return "running"
	default:
		if raw == "" || raw == "pending" {
			return "pending"
		}
		return raw
	}
}

func deploymentWithoutGitHubBuild(d deploymentRow) bool {
	return d.GitHubRunID == 0 && strings.EqualFold(strings.TrimSpace(d.BuildStatus), "success")
}

func (h *Handler) deploymentStages(d *deploymentRow) []deployStage {
	if d == nil {
		return nil
	}
	reconcileDeploymentRow(d)
	buildLabel := "Build (GitHub Actions)"
	registryLabel := "Push image"
	if deploymentWithoutGitHubBuild(*d) {
		buildLabel = "Build (bỏ qua — promote / image có sẵn)"
		registryLabel = "Image registry (đã có sẵn)"
	}
	if d.HarborScan != nil {
		registryLabel = "Push Harbor + quét CVE (Trivy)"
	}
	stages := []deployStage{
		{ID: "build", Label: buildLabel, Status: stageStatus(d.BuildStatus), URL: d.GitHubRunURL},
		{ID: "registry", Label: registryLabel, Status: stageStatus(d.RegistryStatus), Detail: d.Image},
		{ID: "deploy", Label: "Deploy lên cluster", Status: stageStatus(d.DeployStatus)},
		{ID: "runtime", Label: "Worker / Pod", Status: stageStatus(d.RuntimeStatus), Detail: d.RuntimeDetail},
	}
	if deploymentWithoutGitHubBuild(*d) {
		if stages[0].Status == "pending" || stages[0].Status == "running" {
			stages[0].Status = "success"
		}
		stages[0].URL = ""
		if stages[1].Status == "pending" {
			stages[1].Status = "success"
		}
	}
	if d.HarborScan != nil {
		for i := range stages {
			if stages[i].ID != "registry" {
				continue
			}
			if d.HarborScan.Detail != "" {
				stages[i].Detail = d.HarborScan.Detail
			}
			if d.HarborScan.URL != "" {
				stages[i].URL = d.HarborScan.URL
			}
			switch d.HarborScan.Status {
			case "running", "pending":
				// Trivy quét nền — không kéo stage registry về "running" (tránh UI treo "ĐANG QUÉT").
				if stages[i].Status == "success" {
					note := strings.TrimSpace(d.HarborScan.Detail)
					if note == "" {
						note = "Trivy đang quét image (nền)"
					} else {
						note = note + " (nền)"
					}
					if stages[i].Detail != "" && !strings.Contains(stages[i].Detail, note) {
						stages[i].Detail = stages[i].Detail + " · " + note
					} else if stages[i].Detail == "" {
						stages[i].Detail = note
					}
				} else if stages[i].Status != "failed" {
					stages[i].Status = "running"
				}
			case "failed":
				stages[i].Status = "failed"
				if stages[i].Error == "" {
					stages[i].Error = d.HarborScan.Detail
				}
			}
		}
	}
	if allBuildStepsSuccess(d.BuildSteps) && stages[0].Status != "failed" {
		stages[0].Status = "success"
		if stageStatus(d.BuildStatus) != "failed" {
			d.BuildStatus = "success"
		}
	}
	if d.ErrorPhase != "" {
		for i := range stages {
			if stages[i].ID == d.ErrorPhase {
				stages[i].Status = "failed"
				stages[i].Error = d.ErrorMessage
			}
		}
	}
	for i := range stages {
		if stages[i].ID != "runtime" {
			continue
		}
		detail := strings.ToLower(d.RuntimeDetail)
		if strings.Contains(detail, "crashloop") || strings.Contains(detail, "error") || strings.Contains(detail, "backoff") {
			stages[i].Status = "failed"
		}
		if d.RuntimeStatus == "failed" || d.Status == "failed" {
			stages[i].Status = "failed"
		}
		if stages[i].Status == "failed" && stages[i].Error == "" && strings.TrimSpace(d.ErrorMessage) != "" {
			stages[i].Error = d.ErrorMessage
		}
	}
	for i := range stages {
		if stages[i].Status == "skipped" && stages[i].Detail == "" {
			stages[i].Detail = "bỏ qua — bước trước thất bại"
		}
	}
	return stages
}
