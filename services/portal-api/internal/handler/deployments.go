package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	gh "github.com/Thien2026/k8s/services/portal-api/internal/github"
	"github.com/Thien2026/k8s/services/portal-api/internal/rancher"
	"github.com/go-chi/chi/v5"
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

// deploymentSupersededByNewerSuccess — bản in_progress cũ sau khi đã có deploy dev success mới hơn.
func deploymentSupersededByNewerSuccess(items []deploymentRow, env string, idx int) bool {
	if idx <= 0 || idx >= len(items) {
		return false
	}
	env = strings.ToLower(strings.TrimSpace(env))
	for j := 0; j < idx; j++ {
		if strings.ToLower(strings.TrimSpace(items[j].Environment)) != env {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(items[j].Status), "success") {
			return true
		}
	}
	return false
}

func normalizeStaleDeploymentRow(d *deploymentRow) {
	if d == nil || !strings.EqualFold(strings.TrimSpace(d.Status), "failed") {
		return
	}
	switch stageStatus(d.BuildStatus) {
	case "running", "pending", "":
		if strings.EqualFold(strings.TrimSpace(d.ErrorPhase), "build") {
			d.BuildStatus = "failed"
		} else {
			d.BuildStatus = "success"
		}
	}
}

func pickCurrentDeploymentIndex(items []deploymentRow, env, servingTag string) int {
	env = strings.ToLower(strings.TrimSpace(env))
	if env == "" {
		env = "dev"
	}
	for i := range items {
		if strings.ToLower(strings.TrimSpace(items[i].Environment)) != env {
			continue
		}
		if deploymentIsTrulyActive(items[i]) && !deploymentSupersededByNewerSuccess(items, env, i) {
			return i
		}
	}
	servingTag = strings.TrimSpace(servingTag)
	if servingTag != "" {
		for i := range items {
			if strings.ToLower(strings.TrimSpace(items[i].Environment)) != env {
				continue
			}
			if strings.TrimSpace(items[i].ImageTag) == servingTag {
				return i
			}
		}
	}
	bestSuccess := -1
	best := -1
	for i := range items {
		if strings.ToLower(strings.TrimSpace(items[i].Environment)) != env {
			continue
		}
		d := items[i]
		if strings.EqualFold(strings.TrimSpace(d.Status), "success") {
			if bestSuccess < 0 || strings.TrimSpace(d.CreatedAt) > strings.TrimSpace(items[bestSuccess].CreatedAt) {
				bestSuccess = i
			}
		}
		if best < 0 || strings.TrimSpace(d.CreatedAt) > strings.TrimSpace(items[best].CreatedAt) {
			best = i
		}
	}
	if bestSuccess >= 0 {
		return bestSuccess
	}
	return best
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

func (h *Handler) upsertDeployment(ctx context.Context, projectID int64, env, imageTag string) (int64, error) {
	env = strings.ToLower(strings.TrimSpace(env))
	if env == "" {
		env = "dev"
	}
	tag := strings.TrimSpace(imageTag)
	var id int64
	err := h.db.QueryRow(ctx, `
		SELECT id FROM project_deployments
		WHERE project_id=$1 AND environment=$2 AND image_tag=$3
		ORDER BY id DESC LIMIT 1`, projectID, env, tag).Scan(&id)
	if err == nil {
		_, _ = h.db.Exec(ctx, `
			UPDATE project_deployments SET build_status='running', status='in_progress', updated_at=now()
			WHERE id=$1`, id)
		return id, nil
	}
	err = h.db.QueryRow(ctx, `
		INSERT INTO project_deployments (project_id, environment, image_tag, build_status, status)
		VALUES ($1, $2, $3, 'running', 'in_progress')
		RETURNING id`, projectID, env, tag).Scan(&id)
	return id, err
}

func (h *Handler) markDeploymentBuildStarted(ctx context.Context, projectID int64, env, imageTag string) {
	_, _ = h.upsertDeployment(ctx, projectID, env, imageTag)
}

// markDeploymentDeploySkipped — CI đã build image nhưng hook không apply cluster (auto-deploy tắt).
func (h *Handler) markDeploymentDeploySkipped(ctx context.Context, projectID int64, env, imageTag, reason string) {
	id, err := h.getOrCreateDeploymentID(ctx, projectID, env, imageTag)
	if err != nil || id <= 0 {
		return
	}
	_, _ = h.db.Exec(ctx, `
		UPDATE project_deployments SET
			status='failed', build_status='success', registry_status='success',
			deploy_status='skipped', runtime_status='skipped',
			error_phase='deploy', error_message=$1,
			runtime_detail=$1, updated_at=now(), finished_at=now()
		WHERE id=$2`, reason, id)
}

func (h *Handler) markDeploymentFailed(ctx context.Context, id int64, phase, msg string) {
	_, _ = h.db.Exec(ctx, `
		UPDATE project_deployments SET
			status='failed', error_phase=$1, error_message=$2,
			build_status=CASE WHEN $1='build' THEN 'failed' ELSE build_status END,
			registry_status=CASE WHEN $1='registry' THEN 'failed' ELSE registry_status END,
			deploy_status=CASE WHEN $1='deploy' THEN 'failed' ELSE deploy_status END,
			runtime_status=CASE WHEN $1='runtime' THEN 'failed' ELSE runtime_status END,
			updated_at=now(), finished_at=now()
		WHERE id=$3`, phase, msg, id)
}

func (h *Handler) clearFalseBuildFailure(ctx context.Context, id int64) {
	_, _ = h.db.Exec(ctx, `
		UPDATE project_deployments SET
			build_status='success',
			error_phase=CASE WHEN error_phase='build' THEN '' ELSE error_phase END,
			error_message=CASE WHEN error_phase='build' OR error_message ILIKE 'GitHub Actions:%' THEN '' ELSE error_message END,
			status=CASE WHEN status='failed' AND deploy_status='success' AND runtime_status IN ('success','pending','running') THEN 'in_progress' ELSE status END,
			finished_at=CASE WHEN status='failed' AND deploy_status='success' THEN NULL ELSE finished_at END,
			updated_at=now()
		WHERE id=$1`, id)
}

func (h *Handler) markDeploymentDeployPhase(ctx context.Context, id int64, image string) {
	_, _ = h.db.Exec(ctx, `
		UPDATE project_deployments SET
			build_status='success', registry_status='success',
			deploy_status='running', runtime_status='pending',
			image=$1, updated_at=now()
		WHERE id=$2`, image, id)
}

func (h *Handler) markDeploymentSuccess(ctx context.Context, id int64, runtimeDetail string) {
	_, _ = h.db.Exec(ctx, `
		UPDATE project_deployments SET
			status='success',
			build_status='success', registry_status='success',
			deploy_status='success', runtime_status='success',
			runtime_detail=$1, error_phase='', error_message='',
			updated_at=now(), finished_at=now()
		WHERE id=$2`, runtimeDetail, id)
}

func (h *Handler) getOrCreateDeploymentID(ctx context.Context, projectID int64, env, imageTag string) (int64, error) {
	var id int64
	err := h.db.QueryRow(ctx, `
		SELECT id FROM project_deployments
		WHERE project_id=$1 AND environment=$2 AND image_tag=$3
		ORDER BY id DESC LIMIT 1`, projectID, env, imageTag).Scan(&id)
	if err == nil {
		return id, nil
	}
	return h.upsertDeployment(ctx, projectID, env, imageTag)
}

// beginRollbackDeployment — mỗi lần rollback tạo row mới (không reuse bản cũ) để tracking/UI đúng.
func (h *Handler) beginRollbackDeployment(ctx context.Context, projectID int64, env, imageTag string) (int64, error) {
	env = strings.ToLower(strings.TrimSpace(env))
	if env == "" {
		env = "dev"
	}
	tag := strings.TrimSpace(imageTag)
	var id int64
	err := h.db.QueryRow(ctx, `
		INSERT INTO project_deployments (project_id, environment, image_tag, build_status, registry_status, deploy_status, status)
		VALUES ($1, $2, $3, 'success', 'success', 'running', 'in_progress')
		RETURNING id`, projectID, env, tag).Scan(&id)
	return id, err
}

func (h *Handler) hasSuccessfulDeployForTag(ctx context.Context, projectID int64, env, imageTag string) bool {
	env = strings.ToLower(strings.TrimSpace(env))
	if env == "" {
		env = "dev"
	}
	tag := strings.TrimSpace(imageTag)
	if tag == "" {
		return false
	}
	var ok bool
	_ = h.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM project_deployments
			WHERE project_id=$1 AND environment=$2 AND image_tag=$3 AND status='success'
		)`, projectID, env, tag).Scan(&ok)
	return ok
}

func imageMatchesDeploy(image, tag, wantImage string) bool {
	image = normalizeListedImage(image)
	tag = strings.TrimSpace(tag)
	if image == "" {
		return false
	}
	if tag == "" {
		tag = "latest"
	}
	if strings.Contains(image, tag) {
		return true
	}
	if wantImage != "" && strings.Contains(image, wantImage) {
		return true
	}
	short := tag
	if len(short) > 12 {
		short = short[:12]
	}
	if len(short) >= 7 && strings.Contains(image, short) {
		return true
	}
	return false
}

func normalizeListedImage(img string) string {
	img = strings.TrimSpace(img)
	if i := strings.Index(img, " (+"); i > 0 {
		return img[:i]
	}
	return img
}

func appPodsForDeploy(list rancher.ResourceList, tag, wantImage string) []rancher.ResourceRow {
	var matched, candidates []rancher.ResourceRow
	for _, pod := range list.Items {
		if !strings.HasPrefix(pod.Name, "app-") {
			continue
		}
		img := normalizeListedImage(pod.Images)
		if img != "" && !imageMatchesDeploy(img, tag, wantImage) {
			continue
		}
		if img == "" {
			candidates = append(candidates, pod)
			continue
		}
		matched = append(matched, pod)
	}
	if len(matched) > 0 {
		return matched
	}
	if len(candidates) == 1 {
		return candidates
	}
	for _, pod := range candidates {
		if podIsUnhealthy(pod) {
			matched = append(matched, pod)
		}
	}
	if len(matched) > 0 {
		return matched
	}
	return candidates
}

func (h *Handler) runtimeStatusForDeploy(ctx context.Context, p projectRow, env, imageTag string) (status, detail, podName, errMsg string) {
	if h.argoEnabled() {
		appName := h.argoAppName(p.Slug, env)
		appSt, err := h.argoApplicationStatus(ctx, appName)
		if err == nil {
			st, dt, em := argoRuntimeVerdict(appSt)
			if link := h.argoDashboardURL(appName); link != "" {
				if dt == "" {
					dt = link
				} else {
					dt = dt + " · " + link
				}
			}
			return st, dt, "", em
		}
	}
	if h.rancher == nil || !h.rancher.Enabled() {
		return "pending", "Rancher chưa sẵn sàng", "", ""
	}
	ns := h.projectNamespace(p, env)
	deployName, wantImage := h.runtimeWorkload(ctx, p, env, imageTag)
	dep, err := h.rancher.GetDeploymentDetail(ctx, "", ns, deployName)
	pods := h.matchedDeployPods(ctx, p, env, imageTag, deployName)
	podName = pickFirstPodName(pods)
	if err != nil {
		return "running", "Rancher deployment: " + err.Error(), podName, ""
	}
	st, detail, errMsg := evaluateDeploymentRollout(dep.DeploymentRolloutStatus)
	imgSt, imgDetail := evaluateDeploymentImage(dep.ContainerImage, imageTag, wantImage, dep.DeploymentRolloutStatus)
	st, detail, errMsg = mergeImageIntoRollout(st, detail, errMsg, imgSt, imgDetail)
	if podName != "" && st == "success" {
		detail = strings.TrimSpace(detail + " · pod " + podName)
	}
	return st, detail, podName, errMsg
}

func podIsReadyAndHealthy(pod rancher.ResourceRow) bool {
	if podIsUnhealthy(pod) {
		return false
	}
	return pod.Ready && strings.EqualFold(strings.TrimSpace(pod.Status), "running")
}

func podIsUnhealthy(pod rancher.ResourceRow) bool {
	st := strings.ToLower(strings.TrimSpace(pod.Status))
	switch {
	case strings.Contains(st, "crashloop"),
		strings.Contains(st, "backoff"),
		strings.Contains(st, "error"),
		strings.Contains(st, "imagepull"),
		st == "failed",
		st == "unknown":
		return true
	case st == "running" && pod.Restarts > 0 && !pod.Ready:
		return true
	}
	return false
}

func (h *Handler) pickRuntimePodForDeploy(ctx context.Context, p projectRow, env, imageTag string) string {
	if h.rancher == nil || !h.rancher.Enabled() {
		return ""
	}
	ns := p.NamespaceDev
	if env == "prod" {
		ns = p.NamespaceProd
	}
	list, err := h.rancher.ListK8s(ctx, "", "pods", ns, 1, 100)
	if err != nil {
		return ""
	}
	tag := strings.TrimSpace(imageTag)
	_, wantImage := h.runtimeWorkload(ctx, p, env, tag)

	var best string
	bestScore := -1
	for _, pod := range appPodsForDeploy(list, tag, wantImage) {
		score := 0
		if podIsUnhealthy(pod) {
			score += 100 + pod.Restarts
		} else if pod.Status == "Running" {
			score += 10
		}
		if score > bestScore {
			bestScore = score
			best = pod.Name
		}
	}
	return best
}

func (h *Handler) enrichRuntimeLogs(ctx context.Context, p projectRow, env string, d *deploymentRow) {
	if h.rancher == nil || !h.rancher.Enabled() || d == nil {
		return
	}
	podName := strings.TrimSpace(d.PodName)
	if picked := h.pickRuntimePodForDeploy(ctx, p, env, d.ImageTag); picked != "" {
		podName = picked
		d.PodName = picked
	}
	if podName == "" {
		st, detail, name, errMsg := h.runtimeStatusForDeploy(ctx, p, env, d.ImageTag)
		if errMsg != "" {
			return
		}
		d.RuntimeStatus = st
		d.RuntimeDetail = detail
		podName = name
		d.PodName = name
	}
	if podName == "" {
		return
	}
	ns := p.NamespaceDev
	if env == "prod" {
		ns = p.NamespaceProd
	}
	for _, container := range []string{"app", ""} {
		logs, err := h.rancher.GetPodLogs(ctx, "", ns, podName, container, 2000)
		if err == nil && strings.TrimSpace(logs) != "" {
			text, truncated := gh.TruncateLogFull(logs, 256*1024)
			d.RuntimeLog = text
			d.RuntimeLogTruncated = truncated
			return
		}
	}
	if d.RuntimeStatus == "failed" || d.ErrorPhase == "runtime" || strings.TrimSpace(d.ErrorMessage) != "" {
		d.RuntimeLog = h.buildRuntimeLogFallback(ctx, p, ns, podName, d)
		return
	}
	d.RuntimeLog = "(stdout trống)\n"
}

func (h *Handler) buildRuntimeLogFallback(ctx context.Context, p projectRow, ns, podName string, d *deploymentRow) string {
	var b strings.Builder
	if msg := strings.TrimSpace(d.ErrorMessage); msg != "" {
		b.WriteString("Lỗi: ")
		b.WriteString(msg)
		b.WriteString("\n\n")
	}
	fmt.Fprintf(&b, "Pod: %s\n", podName)
	if detail := strings.TrimSpace(d.RuntimeDetail); detail != "" {
		fmt.Fprintf(&b, "Trạng thái: %s\n", detail)
	}
	if img := strings.TrimSpace(d.Image); img != "" {
		fmt.Fprintf(&b, "Image: %s\n", img)
	}

	list, err := h.rancher.ListK8s(ctx, "", "pods", ns, 1, 100)
	if err == nil {
		for _, pod := range list.Items {
			if pod.Name != podName {
				continue
			}
			fmt.Fprintf(&b, "Phase: %s", pod.Status)
			if pod.Restarts > 0 {
				fmt.Fprintf(&b, " · restarts=%d", pod.Restarts)
			}
			if pod.Node != "" {
				fmt.Fprintf(&b, " · node=%s", pod.Node)
			}
			b.WriteString("\n")
			if pod.Images != "" {
				fmt.Fprintf(&b, "Container: %s\n", pod.Images)
			}
			break
		}
	}

	events, err := h.rancher.ListNamespaceEvents(ctx, "", ns, podName, 50)
	if err == nil && len(events) > 0 {
		b.WriteString("\n─── Kubernetes events ───\n")
		for i := len(events) - 1; i >= 0; i-- {
			ev := events[i]
			line := strings.TrimSpace(ev.Reason)
			if line == "" {
				line = ev.Status
			}
			if msg := strings.TrimSpace(ev.Message); msg != "" {
				line += ": " + msg
			}
			if line != "" {
				b.WriteString(line)
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n─── Container stdout ───\n")
	b.WriteString("(trống — app chưa ghi log hoặc container chưa start)\n")
	return b.String()
}

func (h *Handler) enrichDeployClusterLog(ctx context.Context, p projectRow, env string, d *deploymentRow) {
	if h.rancher == nil || !h.rancher.Enabled() || d == nil {
		return
	}
	ns := p.NamespaceDev
	if env == "prod" {
		ns = p.NamespaceProd
	}
	events, err := h.rancher.ListNamespaceEvents(ctx, "", ns, "", 500)
	if err != nil || len(events) == 0 {
		return
	}
	var b strings.Builder
	b.WriteString("─── Full Kubernetes events ───\n")
	count := 0
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		ts := strings.TrimSpace(ev.Created)
		if ts == "" {
			ts = "-"
		}
		obj := strings.TrimSpace(ev.Object)
		if obj == "" {
			obj = strings.TrimSpace(ev.Kind)
		}
		if obj == "" {
			obj = "-"
		}
		line := strings.TrimSpace(ev.Reason)
		if line == "" {
			line = ev.Status
		}
		if line == "" {
			line = "-"
		}
		if msg := strings.TrimSpace(ev.Message); msg != "" {
			line += ": " + msg
		}
		b.WriteString(ts)
		b.WriteString(" | ")
		b.WriteString(obj)
		b.WriteString(" | ")
		b.WriteString(line)
		b.WriteString("\n")
		count++
		if count >= 300 {
			break
		}
	}
	if count > 0 {
		d.DeployLog = b.String()
	}
}

func (h *Handler) refreshDeploymentRuntime(ctx context.Context, p projectRow, d *deploymentRow) {
	reconcileBuildFromSteps(d)
	if stageStatus(d.BuildStatus) == "failed" {
		reconcileDeploymentRow(d)
		return
	}
	if !allBuildStepsSuccess(d.BuildSteps) && d.BuildStatus != "success" && d.DeployStatus != "success" && d.DeployStatus != "running" {
		reconcileDeploymentRow(d)
		return
	}
	if stageStatus(d.DeployStatus) == "pending" || stageStatus(d.DeployStatus) == "skipped" {
		if stageStatus(d.BuildStatus) == "success" || allBuildStepsSuccess(d.BuildSteps) {
			d.Live = false
		}
		reconcileDeploymentRow(d)
		return
	}
	if strings.EqualFold(strings.TrimSpace(d.Status), "success") &&
		strings.EqualFold(strings.TrimSpace(d.RuntimeStatus), "success") {
		if live, _ := h.deploymentTrafficLive(ctx, p, d.Environment, d.ImageTag); !live {
			h.applyTrafficGate(ctx, p, d)
			reconcileDeploymentRow(d)
			return
		}
		d.Live = false
		if strings.TrimSpace(d.DeployLog) == "" || strings.TrimSpace(d.RuntimeLog) == "" {
			v := h.assessRuntimeHealth(ctx, p, d)
			d.RuntimeSignals = v.Tiers
		}
		return
	}
	if deploymentIsTerminal(*d) && strings.EqualFold(strings.TrimSpace(d.RuntimeStatus), "success") {
		d.Live = false
		return
	}
	// Bản ghi failed tạm (smoke 503 lúc rollout, deadline K8s…) — reassess khi cluster đã ready.
	if deploymentIsTerminal(*d) && deploymentFailedMayRecover(*d) {
		d.SmokeStatus = ""
		d.SmokeDetail = ""
	} else if deploymentIsTerminal(*d) && strings.EqualFold(strings.TrimSpace(d.Status), "failed") &&
		strings.Contains(strings.ToLower(d.ErrorMessage), "minimum availability") {
		// fall through to reassess
	} else if deploymentIsTerminal(*d) {
		d.Live = false
		return
	}
	v := h.assessRuntimeHealth(ctx, p, d)
	h.applyRuntimeVerdict(ctx, p, d, v)
}

const deployHistoryLimit = 50

func (h *Handler) listProjectDeployments(ctx context.Context, projectID int64, env string, limit int) ([]deploymentRow, error) {
	if limit < 1 {
		limit = deployHistoryLimit
	}
	env = strings.ToLower(strings.TrimSpace(env))
	if env == "" {
		env = "dev"
	}
	rows, err := h.db.Query(ctx, `
		SELECT id, environment, image_tag, status,
			build_status, registry_status, deploy_status, runtime_status,
			COALESCE(error_phase,''), COALESCE(error_message,''),
			github_run_id, COALESCE(github_run_url,''), COALESCE(image,''), COALESCE(runtime_detail,''),
			created_at, updated_at, finished_at
		FROM project_deployments
		WHERE project_id=$1 AND environment=$2
		ORDER BY created_at DESC
		LIMIT $3`, projectID, env, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []deploymentRow
	for rows.Next() {
		var d deploymentRow
		var finished *time.Time
		var created, updated time.Time
		if err := rows.Scan(
			&d.ID, &d.Environment, &d.ImageTag, &d.Status,
			&d.BuildStatus, &d.RegistryStatus, &d.DeployStatus, &d.RuntimeStatus,
			&d.ErrorPhase, &d.ErrorMessage,
			&d.GitHubRunID, &d.GitHubRunURL, &d.Image, &d.RuntimeDetail,
			&created, &updated, &finished,
		); err != nil {
			return nil, err
		}
		d.CreatedAt = created.UTC().Format(time.RFC3339)
		d.UpdatedAt = updated.UTC().Format(time.RFC3339)
		if finished != nil {
			d.FinishedAt = finished.UTC().Format(time.RFC3339)
		}
		normalizeStaleDeploymentRow(&d)
		out = append(out, d)
	}
	return out, rows.Err()
}

func workflowFileForProject(slug string) string {
	return "platform-deploy-" + slug + ".yml"
}

func (h *Handler) fetchGitHubWorkflowRuns(ctx context.Context, userID int64, owner, repo, slug string, limit int) ([]gh.WorkflowRun, error) {
	ghToken, _, err := h.getGitHubToken(ctx, userID)
	if err != nil || ghToken == "" {
		return nil, err
	}
	client := h.githubClient()
	runs, err := client.ListWorkflowRuns(ctx, ghToken, owner, repo, workflowFileForProject(slug), limit)
	if err == nil && len(runs) > 0 {
		return runs, nil
	}
	return client.ListRepoWorkflowRuns(ctx, ghToken, owner, repo, limit)
}

func (h *Handler) attachGitHubRunForDeployment(ctx context.Context, userID int64, p projectRow, repo projectRepoRow, d *deploymentRow) {
	if d == nil || d.GitHubRunID > 0 {
		return
	}
	owner := strings.TrimSpace(repo.GitHubOwner)
	ghRepo := strings.TrimSpace(repo.GitHubRepo)
	if owner == "" || ghRepo == "" {
		return
	}
	runs, err := h.fetchGitHubWorkflowRuns(ctx, userID, owner, ghRepo, p.Slug, 5)
	if err != nil || len(runs) == 0 {
		return
	}
	tag := strings.TrimSpace(d.ImageTag)
	for _, run := range runs {
		if tag != "" && !strings.EqualFold(strings.TrimSpace(run.HeadSHA), tag) {
			continue
		}
		d.GitHubRunID = run.ID
		d.GitHubRunURL = run.HTMLURL
		if d.ImageTag == "" {
			d.ImageTag = run.HeadSHA
		}
		return
	}
	// Chưa có image_tag — gắn run mới nhất đang chạy hoặc vừa tạo.
	run := runs[0]
	if strings.ToLower(run.Status) != "completed" || d.Status == "in_progress" {
		d.GitHubRunID = run.ID
		d.GitHubRunURL = run.HTMLURL
		if d.ImageTag == "" {
			d.ImageTag = run.HeadSHA
		}
	}
}

func (h *Handler) deploymentRowFromRun(ctx context.Context, p projectRow, deployEnv string, run gh.WorkflowRun, withRuntime bool) deploymentRow {
	bs := mapGitHubRunStatus(run.Status, run.Conclusion)
	h.enrichProjectRegistry(ctx, &p)
	repo, _ := h.getProjectRepo(ctx, p.ID)
	params := h.buildDeployParams(ctx, p, repo, deployEnv, run.HeadSHA, false)
	image := deployImageRef(params)
	d := deploymentRow{
		Environment:  deployEnv,
		ImageTag:     run.HeadSHA,
		GitHubRunID:  run.ID,
		GitHubRunURL: run.HTMLURL,
		BuildStatus:  bs,
		Image:        image,
		CreatedAt:    run.CreatedAt,
		UpdatedAt:    run.UpdatedAt,
	}
	if bs == "success" {
		d.RegistryStatus = "success"
		if !withRuntime {
			d.Status = "in_progress"
			d.DeployStatus = "pending"
			d.RuntimeStatus = "pending"
			return d
		}
		rs, detail, podName, errMsg := h.runtimeStatusForDeploy(ctx, p, deployEnv, run.HeadSHA)
		d.RuntimeDetail = detail
		d.PodName = podName
		if errMsg != "" {
			d.Status = "failed"
			d.DeployStatus = "success"
			d.RuntimeStatus = "failed"
			d.ErrorPhase = "runtime"
			d.ErrorMessage = errMsg
		} else if rs == "success" {
			d.Status = "success"
			d.DeployStatus = "success"
			d.RuntimeStatus = "success"
		} else {
			d.Status = "in_progress"
			d.DeployStatus = "success"
			d.RuntimeStatus = rs
			if rs == "pending" {
				d.DeployStatus = "running"
			}
		}
	} else if bs == "failed" {
		d.Status = "failed"
		d.ErrorPhase = "build"
		if run.Conclusion != "" {
			d.ErrorMessage = "GitHub Actions: " + run.Conclusion
		} else {
			d.ErrorMessage = "Build thất bại trên GitHub Actions"
		}
	} else {
		d.Status = "in_progress"
	}
	return d
}

func deploymentMergeKey(d deploymentRow) string {
	env := strings.ToLower(strings.TrimSpace(d.Environment))
	if env == "" {
		env = "dev"
	}
	tag := strings.TrimSpace(d.ImageTag)
	if tag != "" {
		return env + ":" + tag
	}
	if d.ID > 0 {
		return env + ":id:" + strconv.FormatInt(d.ID, 10)
	}
	return env + ":unknown"
}

func mergeDeploymentItems(dbItems, ghItems []deploymentRow) []deploymentRow {
	byKey := map[string]deploymentRow{}
	for _, d := range dbItems {
		byKey[deploymentMergeKey(d)] = d
	}
	for _, g := range ghItems {
		if g.ImageTag == "" {
			continue
		}
		key := deploymentMergeKey(g)
		if existing, ok := byKey[key]; ok {
			if existing.GitHubRunURL == "" {
				existing.GitHubRunURL = g.GitHubRunURL
			}
			if existing.GitHubRunID == 0 {
				existing.GitHubRunID = g.GitHubRunID
			}
			gBS := strings.ToLower(strings.TrimSpace(g.BuildStatus))
			if gBS == "failed" || gBS == "cancelled" {
				existing.BuildStatus = g.BuildStatus
				existing.Status = "failed"
				if strings.TrimSpace(existing.ErrorPhase) == "" {
					existing.ErrorPhase = "build"
				}
				if strings.TrimSpace(existing.ErrorMessage) == "" && strings.TrimSpace(g.ErrorMessage) != "" {
					existing.ErrorMessage = g.ErrorMessage
				}
				propagateSkippedDownstream(&existing)
			} else if existing.BuildStatus == "pending" || existing.BuildStatus == "running" || existing.BuildStatus == "" {
				existing.BuildStatus = g.BuildStatus
			}
			if g.Status == "in_progress" && !deploymentIsTerminal(existing) {
				existing.Status = "in_progress"
				existing.Live = true
			}
			if existing.RegistryStatus == "pending" && g.RegistryStatus == "success" {
				existing.RegistryStatus = g.RegistryStatus
			}
			if existing.RuntimeDetail == "" {
				existing.RuntimeDetail = g.RuntimeDetail
			}
			if existing.Status == "in_progress" && g.Status != "" && g.Status != "in_progress" {
				existing.Status = g.Status
			}
			if deploymentIsTerminal(existing) {
				existing.Live = false
				normalizeStaleDeploymentRow(&existing)
			}
			byKey[key] = existing
			continue
		}
		byKey[key] = g
	}
	out := make([]deploymentRow, 0, len(byKey))
	for _, d := range byKey {
		out = append(out, d)
	}
	// sort by CreatedAt desc
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].CreatedAt > out[i].CreatedAt {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func (h *Handler) enrichDeploymentsFromGitHub(ctx context.Context, u auth.User, p projectRow, repo projectRepoRow, items []deploymentRow) []deploymentRow {
	if strings.TrimSpace(repo.GitHubOwner) == "" || strings.TrimSpace(repo.GitHubRepo) == "" {
		return items
	}
	runs, err := h.fetchGitHubWorkflowRuns(ctx, u.ID, repo.GitHubOwner, repo.GitHubRepo, p.Slug, 10)
	if err != nil || len(runs) == 0 {
		return items
	}
	deployEnv := strings.TrimSpace(repo.DeployEnvironment)
	if deployEnv == "" {
		deployEnv = "dev"
	}
	var ghItems []deploymentRow
	for i, run := range runs {
		ghItems = append(ghItems, h.deploymentRowFromRun(ctx, p, deployEnv, run, i == 0))
	}
	return mergeDeploymentItems(items, ghItems)
}

func mapGitHubStepStatus(status, conclusion string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	conclusion = strings.ToLower(strings.TrimSpace(conclusion))
	if status == "completed" {
		if conclusion == "skipped" {
			return "skipped"
		}
		if conclusion == "success" || conclusion == "" {
			return "success"
		}
		if conclusion == "failure" || conclusion == "cancelled" || conclusion == "timed_out" {
			return "failed"
		}
		return "success"
	}
	if status == "in_progress" {
		return "running"
	}
	if status == "queued" || status == "pending" || status == "waiting" {
		return "pending"
	}
	return "pending"
}

func (h *Handler) enrichBuildLive(ctx context.Context, userID int64, owner, repo string, d *deploymentRow) {
	if d == nil || d.GitHubRunID == 0 {
		return
	}
	terminal := deploymentIsTerminal(*d)
	buildDone := strings.EqualFold(strings.TrimSpace(d.BuildStatus), "success")
	var runFailMsg string
	ghToken, _, err := h.getGitHubToken(ctx, userID)
	if err != nil || ghToken == "" {
		return
	}
	client := h.githubClient()
	if run, err := client.GetWorkflowRun(ctx, ghToken, owner, repo, d.GitHubRunID); err == nil {
		bs := mapGitHubRunStatus(run.Status, run.Conclusion)
		d.BuildStatus = bs
		buildDone = bs == "success"
		if strings.ToLower(strings.TrimSpace(run.Status)) != "completed" {
			d.Live = true
			terminal = false
		}
		if bs == "failed" {
			runFailMsg = "GitHub Actions: " + strings.TrimSpace(run.Conclusion)
			if runFailMsg == "GitHub Actions: " {
				runFailMsg = "Build thất bại trên GitHub Actions"
			}
		}
	}
	jobs, err := client.GetWorkflowRunJobs(ctx, ghToken, owner, repo, d.GitHubRunID)
	if err != nil || len(jobs) == 0 {
		reconcileDeploymentRow(d)
		return
	}
	maxBytes := 512 * 1024
	if d.BuildStatus == "failed" || d.Status == "failed" || d.ErrorPhase == "build" {
		maxBytes = 2 * 1024 * 1024
	}
	var steps []buildStepView
	var logBuf strings.Builder
	truncated := false
	appendJobLog := func(jobName, logText string) {
		if strings.TrimSpace(logText) == "" {
			return
		}
		text, cut := gh.TruncateLogFull(logText, maxBytes)
		if cut {
			truncated = true
		}
		if logBuf.Len() > 0 {
			logBuf.WriteString("\n\n═══ " + jobName + " ═══\n\n")
		}
		logBuf.WriteString(text)
	}
	// Job failed trước để dễ thấy lỗi.
	type jobLog struct {
		name string
		id   int64
		fail bool
		done bool
	}
	var ordered []jobLog
	for _, job := range jobs {
		jl := jobLog{name: job.Name, id: job.ID, fail: strings.EqualFold(job.Conclusion, "failure")}
		jl.done = strings.EqualFold(job.Status, "completed") || jl.fail
		ordered = append(ordered, jl)
		if !terminal && (strings.ToLower(job.Status) == "in_progress" || strings.ToLower(job.Status) == "queued") {
			d.Live = true
		}
		for _, step := range job.Steps {
			name := strings.TrimSpace(step.Name)
			if name == "" {
				continue
			}
			st := mapGitHubStepStatus(step.Status, step.Conclusion)
			if !terminal && st == "running" {
				d.Live = true
			}
			steps = append(steps, buildStepView{Name: name, Status: st, Number: step.Number})
		}
	}
	for _, pass := range []bool{true, false} {
		for _, jl := range ordered {
			if jl.fail != pass {
				continue
			}
			if !jl.done && !d.Live && !terminal {
				continue
			}
			logText, err := client.DownloadJobLog(ctx, ghToken, owner, repo, jl.id)
			if err == nil {
				appendJobLog(jl.name, logText)
			}
		}
	}
	steps = finalizeBuildStepsTruth(steps)
	d.BuildSteps = steps
	reconcileBuildFromSteps(d)
	buildDone = stageStatus(d.BuildStatus) == "success"
	if !buildDone && runFailMsg != "" {
		d.Status = "failed"
		d.ErrorPhase = "build"
		d.ErrorMessage = runFailMsg
		if d.ID > 0 {
			h.markDeploymentFailed(ctx, d.ID, "build", runFailMsg)
		}
	} else if buildDone && d.ID > 0 {
		h.clearFalseBuildFailure(ctx, d.ID)
	}
	if logBuf.Len() == 0 && d.GitHubRunID > 0 {
		if runLog, err := client.DownloadRunLog(ctx, ghToken, owner, repo, d.GitHubRunID); err == nil {
			text, cut := gh.TruncateLogFull(runLog, maxBytes)
			logBuf.WriteString(text)
			truncated = cut
		}
	}
	if logBuf.Len() > 0 {
		d.BuildLog = logBuf.String()
		d.BuildLogTruncated = truncated
	} else if d.Live && !buildDone && !allBuildStepsSuccess(steps) {
		var running []string
		for _, s := range steps {
			if s.Status == "running" {
				running = append(running, s.Name)
			}
		}
		if len(running) > 0 {
			d.BuildLog = "▶ Đang chạy: " + strings.Join(running, " → ") + "\n\n(Log GitHub Actions cập nhật khi từng step hoàn tất.)"
		} else {
			d.BuildLog = "▶ Build đang chạy trên GitHub Actions…\n"
		}
	} else if (buildDone || allBuildStepsSuccess(steps)) && d.GitHubRunURL != "" {
		d.BuildLog = "Build đã xong. Xem log đầy đủ trên GitHub Actions:\n" + d.GitHubRunURL + "\n"
	}
	if !terminal && (d.BuildStatus == "running" || d.Status == "in_progress") {
		d.Live = true
	}
	if terminal {
		d.Live = false
	}
	reconcileDeploymentRow(d)
}

func mapGitHubRunStatus(status, conclusion string) string {
	status = strings.ToLower(status)
	conclusion = strings.ToLower(conclusion)
	if status == "completed" {
		if conclusion == "success" {
			return "success"
		}
		return "failed"
	}
	if status == "in_progress" || status == "queued" || status == "waiting" {
		return "running"
	}
	return "pending"
}

func (h *Handler) enrichHarborScan(ctx context.Context, p projectRow, d *deploymentRow, withDetails bool) {
	if strings.ToLower(strings.TrimSpace(p.RegistryProvider)) != "harbor" {
		return
	}
	if h.harbor == nil || !h.harbor.Enabled() {
		return
	}
	tag := strings.TrimSpace(d.ImageTag)
	if tag == "" {
		return
	}
	projectName := strings.TrimSpace(p.HarborProject)
	if projectName == "" {
		projectName = p.Slug
	}
	_ = h.harbor.EnableAutoScan(ctx, projectName)
	repo, _ := h.getProjectRepo(ctx, p.ID)
	params := h.buildDeployParams(ctx, p, repo, d.Environment, tag, false)
	artifactRepo := params.PrimaryService().Name
	ov, err := h.harbor.ArtifactScanOverview(ctx, projectName, artifactRepo, tag)
	if err != nil || ov == nil {
		return
	}
	scan := &harborScanView{
		Status:   ov.Status,
		Total:    ov.Total,
		Fixable:  ov.Fixable,
		Severity: ov.Severity,
		Detail:   ov.Detail,
		URL:      h.harbor.ArtifactUIURL(projectName, artifactRepo, tag),
	}
	if withDetails && ov.Status == "success" && ov.Total > 0 {
		vulns, err := h.harbor.ArtifactVulnerabilities(ctx, projectName, artifactRepo, tag, 50)
		if err == nil && len(vulns) > 0 {
			scan.ItemsTotal = ov.Total
			for _, v := range vulns {
				scan.Items = append(scan.Items, harborVulnView{
					ID:         v.ID,
					Severity:   v.Severity,
					Package:    v.Package,
					Version:    v.Version,
					FixVersion: v.FixVersion,
					Status:     v.Status,
				})
			}
		}
	}
	d.HarborScan = scan
	if ov.Status == "failed" {
		d.RegistryStatus = "failed"
	}
}

func (h *Handler) GetProjectDeployActivity(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	env := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("environment")))
	if env == "" {
		env = "dev"
	}
	scope := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("scope")))
	if scope == "" {
		scope = "current"
	}
	repo, _ := h.getProjectRepo(r.Context(), p.ID)
	listLimit := deployHistoryLimit
	if scope == "current" {
		listLimit = 8
	}
	items, err := h.listProjectDeployments(r.Context(), p.ID, env, listLimit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	items = h.enrichDeploymentsFromGitHub(r.Context(), u, p, repo, items)
	servingTag := h.clusterServingImageTag(r.Context(), p, env)
	currentIdx := pickCurrentDeploymentIndex(items, env, servingTag)
	var current *deploymentRow
	pollSec := 5
	if currentIdx >= 0 {
		if scope == "current" {
			if items[currentIdx].GitHubRunID == 0 {
				h.attachGitHubRunForDeployment(r.Context(), u.ID, p, repo, &items[currentIdx])
			}
			if items[currentIdx].GitHubRunID > 0 {
				h.enrichBuildLive(r.Context(), u.ID, repo.GitHubOwner, repo.GitHubRepo, &items[currentIdx])
			} else if deploymentIsTrulyActive(items[currentIdx]) {
				items[currentIdx].Live = true
				pollSec = 2
			}
			cur := &items[currentIdx]
			if stageStatus(cur.BuildStatus) == "success" || allBuildStepsSuccess(cur.BuildSteps) ||
				stageStatus(cur.DeployStatus) != "pending" || deploymentIsTerminal(*cur) {
				h.refreshDeploymentRuntime(r.Context(), p, cur)
			}
			h.enrichHarborScan(r.Context(), p, &items[currentIdx], true)
			if deploymentNeedsFastPoll(items[currentIdx]) {
				pollSec = 2
			} else if deploymentIsTerminal(items[currentIdx]) {
				items[currentIdx].Live = false
				pollSec = 0
			}
		}
		items[currentIdx].Stages = h.deploymentStages(&items[currentIdx])
		current = &items[currentIdx]
	}
	for i := range items {
		if i != currentIdx {
			if scope == "history" && i < 5 {
				h.enrichHarborScan(r.Context(), p, &items[i], false)
			}
			items[i].Stages = h.deploymentStages(&items[i])
		}
	}
	var filtered []deploymentRow
	if scope == "current" {
		if current != nil {
			filtered = []deploymentRow{*current}
		}
	} else {
		filtered = append(filtered, items...)
		if current != nil {
			found := false
			for _, d := range filtered {
				if d.ID == current.ID || (d.ImageTag != "" && d.ImageTag == current.ImageTag && d.Environment == current.Environment) {
					found = true
					break
				}
			}
			if !found {
				filtered = append([]deploymentRow{*current}, filtered...)
			}
		}
	}
	markServing := func(d *deploymentRow) {
		if d == nil {
			return
		}
		d.Serving = strings.EqualFold(d.Status, "success") && servingTag != "" &&
			strings.TrimSpace(d.ImageTag) == servingTag
	}
	markServing(current)
	for i := range filtered {
		markServing(&filtered[i])
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"environment":       env,
		"scope":             scope,
		"current":           current,
		"items":             filtered,
		"serving_image_tag": servingTag,
		"workflow_url": func() string {
			if repo.GitHubOwner != "" && repo.GitHubRepo != "" {
				return fmt.Sprintf("https://github.com/%s/%s/actions", repo.GitHubOwner, repo.GitHubRepo)
			}
			return ""
		}(),
		"poll_interval_sec": func() int {
			if scope == "history" {
				return 0
			}
			return pollSec
		}(),
	})
}

// DeployEventHook — GitHub Actions báo build bắt đầu (đầu workflow).
func (h *Handler) DeployEventHook(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.Header.Get("X-Platform-Deploy-Token"))
	if token == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "thiếu deploy token"})
		return
	}
	var body struct {
		Event       string `json:"event"`
		ImageTag    string `json:"image_tag"`
		Environment string `json:"environment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload không hợp lệ"})
		return
	}
	p, err := h.getProjectByDeployToken(r.Context(), token)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "token không hợp lệ"})
		return
	}
	env := strings.ToLower(strings.TrimSpace(body.Environment))
	if env == "" {
		env = "dev"
	}
	tag := strings.TrimSpace(body.ImageTag)
	switch strings.TrimSpace(body.Event) {
	case "build_started":
		h.markDeploymentBuildStarted(r.Context(), p.ID, env, tag)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "event không hỗ trợ"})
	}
}
