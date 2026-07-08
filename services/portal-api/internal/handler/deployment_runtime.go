package handler

import (
	"context"
	"fmt"
	"strings"

	gh "github.com/Thien2026/k8s/services/portal-api/internal/github"
	"github.com/Thien2026/k8s/services/portal-api/internal/rancher"
)

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
	deployName, wantImage := h.runtimeWorkload(ctx, p, env, imageTag, nil)
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
	_, wantImage := h.runtimeWorkload(ctx, p, env, tag, nil)

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
	if h.repairHistoricalDeploymentRow(ctx, p, d) {
		return
	}
	if h.isHistoricalDeploymentRow(ctx, p, d) {
		d.Live = false
		reconcileDeploymentRow(d)
		return
	}
	if isTrafficGateStaleRow(*d) {
		// Row kẹt "Chưa xác định" — luôn seal thành success (cluster đã chuyển sang tag khác/serving khác).
		d.Status = "success"
		d.RuntimeStatus = "success"
		d.RuntimeDetail = "Đã deploy thành công — cluster hiện phục vụ bản khác"
		d.Live = false
		_, _ = h.db.Exec(ctx, `
			UPDATE project_deployments SET status='success', runtime_status='success',
				runtime_detail='Đã deploy thành công — cluster hiện phục vụ bản khác',
				error_phase='', error_message='', updated_at=now(),
				finished_at=COALESCE(finished_at, now())
			WHERE id=$1`, d.ID)
		reconcileDeploymentRow(d)
		return
	}
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
		if h.isHistoricalDeploymentRow(ctx, p, d) {
			d.Live = false
			return
		}
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
	if v.Status == "failed" && strings.EqualFold(strings.TrimSpace(d.Status), "success") && h.isHistoricalDeploymentRow(ctx, p, d) {
		d.Live = false
		reconcileDeploymentRow(d)
		return
	}
	h.applyRuntimeVerdict(ctx, p, d, v)
}
