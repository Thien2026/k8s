package handler

import (
	"context"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
	"github.com/Thien2026/k8s/services/portal-api/internal/rancher"
)

// runtimeSignalTier — một tầng kiểm tra runtime (rollout / events / smoke / log).
type runtimeSignalTier struct {
	ID     string   `json:"id"`
	Label  string   `json:"label"`
	Status string   `json:"status"`
	Detail string   `json:"detail,omitempty"`
	Items  []string `json:"items,omitempty"` // dòng chi tiết (events) — UI expand
}

type runtimeHealthVerdict struct {
	Status   string
	Detail   string
	PodName  string
	ErrorMsg string
	Tiers    []runtimeSignalTier
	LogHint  string
}

func tierStatusFromStage(st string) string {
	switch stageStatus(st) {
	case "success":
		return "success"
	case "failed":
		return "failed"
	case "skipped":
		return "skipped"
	case "running":
		return "running"
	default:
		return "pending"
	}
}

// runtimeLogHint trích dòng log hữu ích cho UI — không quyết định pass/fail.
func runtimeLogHint(log string) string {
	log = strings.TrimSpace(log)
	if log == "" {
		return ""
	}
	lines := strings.Split(log, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if runtimeLogLineLooksError(line) {
			return line
		}
	}
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
}

func runtimeLogLineLooksError(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	if lower == "" {
		return false
	}
	if strings.Contains(lower, "listening on") || strings.Contains(lower, "server listening") {
		return false
	}
	return strings.Contains(lower, "error") ||
		strings.Contains(lower, "fatal") ||
		strings.Contains(lower, "panic") ||
		strings.Contains(lower, "traceback") ||
		strings.Contains(lower, "thiếu biến môi trường") ||
		strings.Contains(lower, "npm err")
}

func evaluateDeploymentRollout(ds rancher.DeploymentRolloutStatus) (status, detail, errMsg string) {
	if ds.IsFailed() {
		msg := ds.FailureMessage()
		if msg == "" {
			msg = ds.Summary()
		}
		return "failed", ds.Summary(), msg
	}
	if ds.IsReady() {
		return "success", ds.Summary(), ""
	}
	return "running", ds.Summary(), ""
}

func deployPodLabelSelector(serviceName, imageTag string) string {
	if strings.TrimSpace(serviceName) == "" {
		serviceName = "app"
	}
	tag := deploy.ImageTagLabelValue(imageTag)
	return "app=" + serviceName + "," + deploy.LabelImageTag + "=" + tag
}

func pickFirstPodName(pods []rancher.ResourceRow) string {
	if len(pods) == 0 {
		return ""
	}
	return pods[0].Name
}

// evaluateK8sPodTier — bổ trợ pod; nguồn sự thật rollout là Deployment (Rancher).
func evaluateK8sPodTier(pods []rancher.ResourceRow) (status, detail, podName, errMsg string) {
	if len(pods) == 0 {
		return "running", "Đang chờ pod image mới…", "", ""
	}

	var details []string
	allHealthy := true
	anyPending := false
	bestPod := pods[0].Name

	for _, pod := range pods {
		details = append(details, podSummaryLine(pod))
		if podIsUnhealthy(pod) {
			allHealthy = false
			bestPod = pod.Name
		} else if isPodPendingPhase(pod) {
			allHealthy = false
			anyPending = true
			bestPod = pod.Name
		} else if !strings.EqualFold(strings.TrimSpace(pod.Status), "Running") {
			allHealthy = false
			bestPod = pod.Name
		}
	}
	detailStr := strings.Join(details, "; ")

	if !allHealthy {
		for _, pod := range pods {
			if podIsUnhealthy(pod) {
				return "failed", detailStr, pod.Name, "Pod " + pod.Name + ": " + pod.Status
			}
		}
		if anyPending {
			return "running", detailStr, bestPod, ""
		}
		return "failed", detailStr, bestPod, "Pod chưa healthy"
	}

	for _, pod := range pods {
		if !podIsReadyAndHealthy(pod) {
			if strings.EqualFold(strings.TrimSpace(pod.Status), "running") && !pod.Ready && !podIsUnhealthy(pod) {
				return "running", detailStr + " — chờ readiness probe /health", pod.Name, ""
			}
			return "failed", detailStr, pod.Name, "Pod " + pod.Name + " chưa Ready (readiness probe)"
		}
	}
	return "success", detailStr, bestPod, ""
}

func podSummaryLine(pod rancher.ResourceRow) string {
	s := pod.Name + ": " + pod.Status
	if pod.Ready {
		s += " (Ready)"
	} else if strings.EqualFold(strings.TrimSpace(pod.Status), "Running") {
		s += " (NotReady)"
	}
	if pod.Restarts > 0 {
		s += ", restarts=" + itoa(pod.Restarts)
	}
	return s
}

func isPodPendingPhase(pod rancher.ResourceRow) bool {
	st := strings.TrimSpace(pod.Status)
	return st == "Pending" || st == "ContainerCreating"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func mergeRuntimeHealth(k8sSt, k8sDetail, podName, k8sErr, smokeSt, smokeDetail, logHint string, tiers []runtimeSignalTier) runtimeHealthVerdict {
	v := runtimeHealthVerdict{
		PodName: podName,
		Tiers:   tiers,
		LogHint: logHint,
		Detail:  k8sDetail,
	}

	appendLog := func(msg string) string {
		if logHint == "" {
			return msg
		}
		if msg == "" {
			return "Log: " + logHint
		}
		return msg + " · Log: " + logHint
	}

	switch stageStatus(k8sSt) {
	case "failed":
		v.Status = "failed"
		v.ErrorMsg = appendLog(k8sErr)
		if v.ErrorMsg == "" {
			v.ErrorMsg = appendLog(k8sDetail)
		}
		return v
	case "running", "pending":
		v.Status = "running"
		if v.Detail == "" {
			v.Detail = "Đang chờ deployment rollout (Rancher)…"
		}
		return v
	}

	// Deployment ready — tầng 2: smoke
	switch stageStatus(smokeSt) {
	case "failed":
		v.Status = "failed"
		v.ErrorMsg = appendLog("Smoke check: " + smokeDetail)
		return v
	case "running", "pending":
		v.Status = "running"
		v.Detail = "Deployment ready — đang kiểm tra HTTPS công khai…"
		return v
	}

	v.Status = "success"
	v.Detail = "Deployment rollout OK (Rancher)"
	if smokeSt == "success" && smokeDetail != "" {
		v.Detail += " · " + smokeDetail
	} else if smokeSt == "skipped" {
		v.Detail += " · Smoke HTTPS bỏ qua (chưa có domain)"
	}
	return v
}

func (h *Handler) runtimeWorkload(ctx context.Context, p projectRow, env, tag string) (deployName, wantImage string) {
	repo, _ := h.getProjectRepo(ctx, p.ID)
	params := h.buildDeployParams(ctx, p, repo, env, tag, false)
	primary := params.PrimaryService()
	h.enrichProjectRegistry(ctx, &p)
	if strings.TrimSpace(tag) == "" {
		tag = "latest"
	}
	wantImage = strings.TrimSpace(p.Registry.ImagePrefix) + "/" + primary.Name + ":" + tag
	return primary.Name, wantImage
}

func (h *Handler) assessRuntimeHealth(ctx context.Context, p projectRow, d *deploymentRow) runtimeHealthVerdict {
	if d == nil {
		return runtimeHealthVerdict{Status: "pending"}
	}
	h.enrichDeployClusterLog(ctx, p, d.Environment, d)
	h.enrichRuntimeLogs(ctx, p, d.Environment, d)

	ns := h.projectNamespace(p, d.Environment)
	tag := strings.TrimSpace(d.ImageTag)
	if tag == "" {
		tag = "latest"
	}
	deployName, wantImage := h.runtimeWorkload(ctx, p, d.Environment, tag)

	var k8sSt, k8sDetail, k8sErr string
	var depDetail rancher.DeploymentDetail
	var depErr error
	if h.rancher != nil && h.rancher.Enabled() {
		depDetail, depErr = h.rancher.GetDeploymentDetail(ctx, "", ns, deployName)
		if depErr != nil {
			k8sSt = "running"
			k8sDetail = "Rancher deployment: " + depErr.Error()
		} else {
			k8sSt, k8sDetail, k8sErr = evaluateDeploymentRollout(depDetail.DeploymentRolloutStatus)
			imgSt, imgDetail := evaluateDeploymentImage(depDetail.ContainerImage, tag, wantImage, depDetail.DeploymentRolloutStatus)
			k8sSt, k8sDetail, k8sErr = mergeImageIntoRollout(k8sSt, k8sDetail, k8sErr, imgSt, imgDetail)
			if !deployImageTagLabelMatches(tag, depDetail.PodImageTagLabel) && stageStatus(k8sSt) != "failed" {
				k8sDetail = strings.TrimSpace(k8sDetail + " · label tag=" + depDetail.PodImageTagLabel)
			}
		}
	} else {
		k8sSt = "pending"
		k8sDetail = "Rancher chưa sẵn sàng"
	}

	pods := h.matchedDeployPods(ctx, p, d.Environment, d.ImageTag, deployName)
	podName := pickFirstPodName(pods)
	if podName != "" {
		d.PodName = podName
	}
	if podName != "" && stageStatus(k8sSt) == "running" {
		k8sDetail = strings.TrimSpace(k8sDetail + " · " + podSummaryLine(pods[0]))
	}
	d.RuntimeDetail = k8sDetail

	var runtimeEvents []rancher.ResourceRow
	if h.rancher != nil && h.rancher.Enabled() {
		runtimeEvents, _ = h.rancher.ListNamespaceEvents(ctx, "", ns, podName, 100)
		if len(runtimeEvents) == 0 {
			runtimeEvents, _ = h.rancher.ListNamespaceEvents(ctx, "", ns, "", 100)
		}
		if hint := eventFailureHint(runtimeEvents, podName); hint != "" && k8sErr == "" && stageStatus(k8sSt) == "running" {
			k8sDetail = strings.TrimSpace(k8sDetail + " · Event: " + hint)
			d.RuntimeDetail = k8sDetail
		}
	}

	logHint := runtimeLogHint(d.RuntimeLog)
	logTierStatus := "pending"
	logTierDetail := "Chưa có log pod"
	if strings.TrimSpace(d.RuntimeLog) != "" {
		if logHint != "" {
			logTierStatus = "info"
			logTierDetail = logHint
		} else {
			logTierStatus = "success"
			logTierDetail = "Log pod có sẵn — không thấy dòng lỗi rõ ràng"
		}
	}

	tiers := []runtimeSignalTier{
		{
			ID:     "k8s",
			Label:  "Deployment rollout (Rancher)",
			Status: tierStatusFromStage(k8sSt),
			Detail: k8sDetail,
		},
	}

	evView := buildDeployRuntimeEvents(runtimeEvents, podName)
	tiers = append(tiers, runtimeSignalTier{
		ID:     "events",
		Label:  "Kubernetes events (Rancher)",
		Status: evView.Status,
		Detail: evView.Detail,
		Items:  evView.Items,
	})

	smokeSt := strings.TrimSpace(d.SmokeStatus)
	smokeDetail := strings.TrimSpace(d.SmokeDetail)
	if stageStatus(k8sSt) == "success" {
		host := h.primaryDomainHost(ctx, p.ID, d.Environment)
		repo, _ := h.getProjectRepo(ctx, p.ID)
		paths := h.smokePathsForProject(ctx, p.ID, repo)
		smokeSt, smokeDetail = h.smokeCheckHTTP(ctx, host, paths)
		d.SmokeStatus = smokeSt
		d.SmokeDetail = smokeDetail
	} else {
		smokeSt = "skipped"
		smokeDetail = "Chờ deployment rollout (Rancher) trước khi kiểm tra HTTPS"
	}

	tiers = append(tiers, runtimeSignalTier{
		ID:     "smoke",
		Label:  "HTTPS công khai",
		Status: tierStatusFromStage(smokeSt),
		Detail: smokeDetail,
	})
	tiers = append(tiers, runtimeSignalTier{
		ID:     "log",
		Label:  "Log ứng dụng (tham khảo)",
		Status: logTierStatus,
		Detail: logTierDetail,
	})

	return mergeRuntimeHealth(k8sSt, k8sDetail, podName, k8sErr, smokeSt, smokeDetail, logHint, tiers)
}

func (h *Handler) matchedDeployPods(ctx context.Context, p projectRow, env, imageTag, serviceName string) []rancher.ResourceRow {
	if h.rancher == nil || !h.rancher.Enabled() {
		return nil
	}
	if strings.TrimSpace(serviceName) == "" {
		serviceName = "app"
	}
	ns := h.projectNamespace(p, env)
	tag := strings.TrimSpace(imageTag)
	if tag == "" {
		tag = "latest"
	}
	h.enrichProjectRegistry(ctx, &p)
	wantImage := strings.TrimSpace(p.Registry.ImagePrefix) + "/" + serviceName + ":" + tag

	pods, err := h.rancher.ListPods(ctx, "", ns, deployPodLabelSelector(serviceName, tag), 20)
	if err != nil || len(pods) == 0 {
		pods, _ = h.rancher.ListPods(ctx, "", ns, "app="+serviceName, 20)
	}
	if len(pods) == 0 {
		list, listErr := h.rancher.ListK8s(ctx, "", "pods", ns, 1, 100)
		if listErr == nil {
			return appPodsForDeploy(list, tag, wantImage)
		}
		return nil
	}
	if len(pods) == 1 {
		return pods
	}
	filtered := appPodsForDeploy(rancher.ResourceList{Items: pods}, tag, wantImage)
	if len(filtered) > 0 {
		return filtered
	}
	return pods
}

func (h *Handler) applyRuntimeVerdict(ctx context.Context, d *deploymentRow, v runtimeHealthVerdict) {
	if d == nil {
		return
	}
	d.RuntimeStatus = v.Status
	if v.Detail != "" {
		d.RuntimeDetail = v.Detail
	}
	d.RuntimeSignals = v.Tiers

	switch v.Status {
	case "failed":
		d.Status = "failed"
		d.ErrorPhase = "runtime"
		d.ErrorMessage = v.ErrorMsg
		d.Live = false
		_, _ = h.db.Exec(ctx, `
			UPDATE project_deployments SET runtime_status='failed', runtime_detail=$1,
				status='failed', error_phase='runtime', error_message=$2, updated_at=now(), finished_at=now()
			WHERE id=$3`, d.RuntimeDetail, d.ErrorMessage, d.ID)
	case "success":
		d.Status = "success"
		d.DeployStatus = "success"
		d.RuntimeStatus = "success"
		d.ErrorPhase = ""
		d.ErrorMessage = ""
		d.Live = false
		_, _ = h.db.Exec(ctx, `
			UPDATE project_deployments SET runtime_status='success', runtime_detail=$1,
				deploy_status='success', status='success', error_phase='', error_message='', updated_at=now(),
				finished_at=COALESCE(finished_at, now())
			WHERE id=$2`, d.RuntimeDetail, d.ID)
	case "running", "pending":
		if !deploymentIsTerminal(*d) {
			d.Live = true
		}
		_, _ = h.db.Exec(ctx, `
			UPDATE project_deployments SET runtime_status=$1, runtime_detail=$2, updated_at=now()
			WHERE id=$3`, v.Status, d.RuntimeDetail, d.ID)
	}
	reconcileDeploymentRow(d)
}
