package handler

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	gh "github.com/Thien2026/k8s/services/portal-api/internal/github"
)

func deploymentMergeKey(d deploymentRow) string {
	env := strings.ToLower(strings.TrimSpace(d.Environment))
	if env == "" {
		env = "dev"
	}
	tag := strings.TrimSpace(d.ImageTag)
	if tag != "" {
		return env + ":" + strings.ToLower(tag)
	}
	if d.ID > 0 {
		return env + ":id:" + strconv.FormatInt(d.ID, 10)
	}
	return env + ":unknown"
}

func findDeploymentMergeKey(byKey map[string]deploymentRow, g deploymentRow) (string, deploymentRow, bool) {
	key := deploymentMergeKey(g)
	if existing, ok := byKey[key]; ok {
		return key, existing, true
	}
	env := strings.ToLower(strings.TrimSpace(g.Environment))
	if env == "" {
		env = "dev"
	}
	for k, existing := range byKey {
		if !strings.HasPrefix(k, env+":") {
			continue
		}
		if imageTagsMatch(existing.ImageTag, g.ImageTag) {
			return k, existing, true
		}
	}
	return key, deploymentRow{}, false
}

func mergeGhDeployment(existing, g deploymentRow) deploymentRow {
	if strings.EqualFold(strings.TrimSpace(existing.Status), "success") && deploymentIsTerminal(existing) {
		if existing.GitHubRunURL == "" {
			existing.GitHubRunURL = g.GitHubRunURL
		}
		if existing.GitHubRunID == 0 {
			existing.GitHubRunID = g.GitHubRunID
		}
		existing.Live = false
		return existing
	}
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
	return existing
}

func deploymentItemsSorted(items []deploymentRow) []deploymentRow {
	out := append([]deploymentRow(nil), items...)
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].CreatedAt > out[i].CreatedAt {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func mergeDeploymentItems(dbItems, ghItems []deploymentRow) []deploymentRow {
	byKey := map[string]deploymentRow{}
	for _, d := range dbItems {
		key := deploymentMergeKey(d)
		existing, ok := byKey[key]
		if !ok {
			byKey[key] = d
			continue
		}
		dRank := deploymentHistoryRank(d)
		eRank := deploymentHistoryRank(existing)
		if dRank > eRank || (dRank == eRank && d.ID > existing.ID) {
			byKey[key] = d
		}
	}
	for _, g := range ghItems {
		if g.ImageTag == "" {
			continue
		}
		key, existing, ok := findDeploymentMergeKey(byKey, g)
		if ok {
			byKey[key] = mergeGhDeployment(existing, g)
			continue
		}
		byKey[key] = g
	}
	return deploymentItemsSorted(mapValuesDeployment(byKey))
}

func mergeDeploymentEnrichOnly(dbItems, ghItems []deploymentRow) []deploymentRow {
	byKey := map[string]deploymentRow{}
	for _, d := range dbItems {
		key := deploymentMergeKey(d)
		existing, ok := byKey[key]
		if !ok {
			byKey[key] = d
			continue
		}
		dRank := deploymentHistoryRank(d)
		eRank := deploymentHistoryRank(existing)
		if dRank > eRank || (dRank == eRank && d.ID > existing.ID) {
			byKey[key] = d
		}
	}
	for _, g := range ghItems {
		if g.ImageTag == "" {
			continue
		}
		key, existing, ok := findDeploymentMergeKey(byKey, g)
		if !ok {
			continue
		}
		byKey[key] = mergeGhDeployment(existing, g)
	}
	return deploymentItemsSorted(mapValuesDeployment(byKey))
}

func mapValuesDeployment(m map[string]deploymentRow) []deploymentRow {
	out := make([]deploymentRow, 0, len(m))
	for _, d := range m {
		out = append(out, d)
	}
	return out
}

func ghRunAfterProjectCreated(projectCreatedAt, runCreatedAt string) bool {
	if strings.TrimSpace(projectCreatedAt) == "" || strings.TrimSpace(runCreatedAt) == "" {
		return true
	}
	pc, err1 := time.Parse(time.RFC3339, projectCreatedAt)
	rc, err2 := time.Parse(time.RFC3339, runCreatedAt)
	if err1 != nil || err2 != nil {
		return true
	}
	return !rc.Before(pc.Add(-2 * time.Minute))
}

func (h *Handler) enrichDeploymentsFromGitHub(ctx context.Context, u auth.User, p projectRow, repo projectRepoRow, items []deploymentRow, scope string) []deploymentRow {
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
		if !ghRunAfterProjectCreated(p.CreatedAt, run.CreatedAt) {
			continue
		}
		matched := false
		for _, it := range items {
			if imageTagsMatch(it.ImageTag, run.HeadSHA) {
				matched = true
				break
			}
		}
		inFlight := !strings.EqualFold(strings.TrimSpace(run.Status), "completed")
		if !matched && !inFlight {
			continue
		}
		withRuntime := matched || (i == 0 && inFlight)
		ghItems = append(ghItems, h.deploymentRowFromRun(ctx, p, deployEnv, run, withRuntime))
	}
	if scope == "history" {
		return mergeDeploymentEnrichOnly(items, ghItems)
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
