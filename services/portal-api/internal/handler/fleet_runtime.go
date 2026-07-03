package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
	"github.com/Thien2026/k8s/services/portal-api/internal/rancher"
)

func imageTagsMatch(want, got string) bool {
	want = strings.TrimSpace(want)
	got = strings.TrimSpace(got)
	if want == "" || got == "" {
		return want == got
	}
	return want == got || strings.HasPrefix(want, got) || strings.HasPrefix(got, want)
}

func (h *Handler) listServicePods(ctx context.Context, p projectRow, env, serviceName string) []rancher.ResourceRow {
	if h.rancher == nil || !h.rancher.Enabled() {
		return nil
	}
	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" {
		serviceName = "app"
	}
	ns := h.projectNamespace(p, env)
	list, err := h.rancher.ListK8s(ctx, "", "pods", ns, 1, 120)
	if err != nil {
		return nil
	}
	prefix := serviceName + "-"
	var out []rancher.ResourceRow
	for _, pod := range list.Items {
		if strings.HasPrefix(pod.Name, prefix) {
			out = append(out, pod)
		}
	}
	return out
}

func (h *Handler) serviceWorkloadExists(ctx context.Context, p projectRow, env, serviceName string) bool {
	if h.serviceHasReadyPods(ctx, p, env, serviceName) {
		return true
	}
	if h.rancher == nil || !h.rancher.Enabled() {
		return false
	}
	ns := h.projectNamespace(p, env)
	_, err := h.rancher.GetDeploymentDetail(ctx, "", ns, strings.TrimSpace(serviceName))
	return err == nil
}

func (h *Handler) serviceHasReadyPods(ctx context.Context, p projectRow, env, serviceName string) bool {
	for _, pod := range h.listServicePods(ctx, p, env, serviceName) {
		if podIsReadyAndHealthy(pod) {
			return true
		}
	}
	return false
}

// filterFleetRolloutServices — chỉ bắt buộc rollout các workload đã có trên cluster;
// nếu chưa có gì (lần đầu) thì chỉ yêu cầu service public (Ingress).
func filterFleetRolloutServices(services []deploy.ServiceDef, exists func(name string) bool) []deploy.ServiceDef {
	var onCluster []deploy.ServiceDef
	for _, svc := range services {
		if exists(strings.TrimSpace(svc.Name)) {
			onCluster = append(onCluster, svc)
		}
	}
	if len(onCluster) > 0 {
		return onCluster
	}
	var pub []deploy.ServiceDef
	for _, svc := range services {
		if deploy.IsServicePublic(svc) {
			pub = append(pub, svc)
		}
	}
	if len(pub) > 0 {
		return pub
	}
	return services
}

func (h *Handler) fleetServicesForRollout(ctx context.Context, p projectRow, env string, services []deploy.ServiceDef) []deploy.ServiceDef {
	return filterFleetRolloutServices(services, func(name string) bool {
		return h.serviceWorkloadExists(ctx, p, env, name)
	})
}

// evaluateFleetRolloutForParams — fleet check theo profile deploy thực tế (rollback snapshot), không phải Console.
func (h *Handler) evaluateFleetRolloutForParams(ctx context.Context, p projectRow, params deploy.Params, imageTag string) (status, detail, errMsg string) {
	if !params.IsMultiService() {
		return "", "", ""
	}
	tag := strings.TrimSpace(imageTag)
	if tag == "" {
		return "", "", ""
	}
	services := params.EffectiveServices()
	if len(services) < 2 {
		return "", "", ""
	}
	if h.clusterRunsSingleAppOnly(ctx, p, params.Environment, services) {
		return "", "", ""
	}
	toCheck := h.fleetServicesForRollout(ctx, p, params.Environment, services)
	h.enrichProjectRegistry(ctx, &p)
	ns := h.projectNamespace(p, params.Environment)

	var parts []string
	blocked := false
	rolling := false
	for _, svc := range toCheck {
		svcSt, svcDetail, svcErr := h.serviceFleetRolloutStatus(ctx, p, ns, svc.Name, tag)
		if svcDetail != "" {
			parts = append(parts, svc.Name+": "+svcDetail)
		}
		switch stageStatus(svcSt) {
		case "failed":
			blocked = true
			if svcErr != "" {
				errMsg = svcErr
			}
		case "running", "pending":
			rolling = true
		}
	}
	detail = strings.Join(parts, "; ")
	if rolling && !blocked && h.clusterLegacyAppMatchesTag(ctx, p, params.Environment, tag) {
		msg := "Console đã cấu hình multi-service nhưng cluster vẫn chạy deployment app đơn — sync workflow GitHub rồi deploy lại"
		if detail != "" {
			msg = detail + " — " + msg
		}
		return "failed", detail, msg
	}
	if blocked {
		if detail == "" {
			detail = "pod mới crash hoặc lỗi"
		}
		if errMsg == "" {
			errMsg = "Rollout fleet: " + detail
		}
		return "failed", detail, errMsg
	}
	if rolling {
		if detail == "" {
			detail = "đang chờ toàn bộ service lên image mới"
		}
		return "running", detail, ""
	}
	return "success", fmt.Sprintf("Fleet rollout OK (%d service)", len(toCheck)), ""
}

// evaluateFleetRollout — multi-service: dùng Deployment status (Rancher) làm nguồn sự thật, không đoán từ pod list.
func (h *Handler) evaluateFleetRollout(ctx context.Context, p projectRow, env, imageTag string) (status, detail, errMsg string) {
	repo, _ := h.getProjectRepo(ctx, p.ID)
	services, layout := h.loadDeployServices(ctx, p.ID, repo)
	if layout != deploy.LayoutMulti || len(services) < 2 {
		return "", "", ""
	}
	tag := strings.TrimSpace(imageTag)
	if tag == "" {
		return "", "", ""
	}
	// Rollback single-app: cluster chỉ còn deployment app — bỏ qua fleet multi.
	if h.clusterRunsSingleAppOnly(ctx, p, env, services) {
		return "", "", ""
	}
	toCheck := h.fleetServicesForRollout(ctx, p, env, services)
	h.enrichProjectRegistry(ctx, &p)
	ns := h.projectNamespace(p, env)

	var parts []string
	blocked := false
	rolling := false
	for _, svc := range toCheck {
		svcSt, svcDetail, svcErr := h.serviceFleetRolloutStatus(ctx, p, ns, svc.Name, tag)
		if svcDetail != "" {
			parts = append(parts, svc.Name+": "+svcDetail)
		}
		switch stageStatus(svcSt) {
		case "failed":
			blocked = true
			if svcErr != "" {
				errMsg = svcErr
			}
		case "running", "pending":
			rolling = true
		}
	}
	detail = strings.Join(parts, "; ")
	if rolling && !blocked && h.clusterLegacyAppMatchesTag(ctx, p, env, tag) {
		msg := "Console đã cấu hình multi-service nhưng cluster vẫn chạy deployment app đơn — sync workflow GitHub rồi deploy lại"
		if detail != "" {
			msg = detail + " — " + msg
		}
		return "failed", detail, msg
	}
	if blocked {
		if detail == "" {
			detail = "pod mới crash hoặc lỗi"
		}
		if errMsg == "" {
			errMsg = "Rollout fleet: " + detail
		}
		return "failed", detail, errMsg
	}
	if rolling {
		if detail == "" {
			detail = "đang chờ toàn bộ service lên image mới"
		}
		return "running", detail, ""
	}
	return "success", fmt.Sprintf("Fleet rollout OK (%d service)", len(toCheck)), ""
}

func (h *Handler) serviceFleetRolloutStatus(ctx context.Context, p projectRow, ns, serviceName, tag string) (status, detail, errMsg string) {
	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" {
		return "failed", "tên service trống", ""
	}
	if h.rancher != nil && h.rancher.Enabled() {
		dep, err := h.rancher.GetDeploymentDetail(ctx, "", ns, serviceName)
		if err == nil {
			wantImage := strings.TrimSpace(p.Registry.ImagePrefix) + "/" + serviceName + ":" + tag
			rolloutSt, rolloutDetail, rolloutErr := evaluateDeploymentRollout(dep.DeploymentRolloutStatus)
			imgSt, imgDetail := evaluateDeploymentImage(dep.ContainerImage, tag, wantImage, dep.DeploymentRolloutStatus)
			rolloutSt, rolloutDetail, rolloutErr = mergeImageIntoRollout(rolloutSt, rolloutDetail, rolloutErr, imgSt, imgDetail)
			return rolloutSt, rolloutDetail, rolloutErr
		}
	}
	// Fallback: pod list (khi không đọc được Deployment)
	pods := h.listServicePods(ctx, p, envFromNamespace(p, ns), serviceName)
	if len(pods) == 0 {
		return "running", "không có pod", ""
	}
	healthyNew := 0
	legacyReady := 0
	for _, pod := range pods {
		pt := imageTagFromImageRef(pod.Images)
		if imageTagsMatch(tag, pt) {
			if podIsUnhealthy(pod) {
				return "failed", pod.Name + " " + pod.Status, "Pod " + pod.Name + " unhealthy"
			}
			if podIsReadyAndHealthy(pod) {
				healthyNew++
			} else {
				return "running", pod.Name + " chưa Ready", ""
			}
		} else if pod.Status == "Running" && pod.Ready {
			legacyReady++
		}
	}
	if healthyNew > 0 {
		return "success", depRolloutSummary(healthyNew), ""
	}
	if legacyReady > 0 {
		return "running", "pod cũ vẫn phục vụ traffic", ""
	}
	return "running", "chưa có pod image mới", ""
}

func depRolloutSummary(ready int) string {
	if ready <= 1 {
		return "1 pod Ready"
	}
	return fmt.Sprintf("%d pod Ready", ready)
}

func envFromNamespace(p projectRow, ns string) string {
	if strings.TrimSpace(ns) == strings.TrimSpace(p.NamespaceProd) {
		return "prod"
	}
	return "dev"
}

// clusterRunsSingleAppOnly — layout Console multi nhưng cluster chỉ còn workload app (rollback single).
func (h *Handler) clusterRunsSingleAppOnly(ctx context.Context, p projectRow, env string, services []deploy.ServiceDef) bool {
	if !h.serviceHasReadyPods(ctx, p, env, "app") {
		return false
	}
	for _, svc := range services {
		name := strings.TrimSpace(svc.Name)
		if name == "" || name == "app" {
			continue
		}
		if h.serviceHasReadyPods(ctx, p, env, name) {
			return false
		}
	}
	return true
}

func (h *Handler) clusterLegacyAppMatchesTag(ctx context.Context, p projectRow, env, imageTag string) bool {
	tag := strings.TrimSpace(imageTag)
	if tag == "" {
		return false
	}
	for _, pod := range h.listServicePods(ctx, p, env, "app") {
		if podIsReadyAndHealthy(pod) && imageTagsMatch(tag, imageTagFromImageRef(pod.Images)) {
			return true
		}
	}
	return false
}

func (h *Handler) deploymentTrafficLive(ctx context.Context, p projectRow, env, imageTag string) (live bool, detail string) {
	tag := strings.TrimSpace(imageTag)
	if tag == "" {
		return false, ""
	}
	serving := strings.TrimSpace(h.clusterServingImageTag(ctx, p, env))
	if serving == "" {
		return false, "Chưa xác định image đang phục vụ trên cluster"
	}
	if imageTagsMatch(tag, serving) {
		return true, ""
	}
	return false, fmt.Sprintf("Cluster phục vụ %s — bản %s chưa thay traffic", shortImageTag(serving), shortImageTag(tag))
}

func shortImageTag(tag string) string {
	tag = strings.TrimSpace(tag)
	if len(tag) > 7 {
		return tag[:7]
	}
	return tag
}

func (h *Handler) clusterRunsFleet(ctx context.Context, p projectRow, env string) bool {
	return h.serviceWorkloadExists(ctx, p, env, "api") && h.serviceWorkloadExists(ctx, p, env, "web")
}

func (h *Handler) applyTrafficGate(ctx context.Context, p projectRow, d *deploymentRow) bool {
	if d == nil {
		return true
	}
	if deploy.NormalizeLayout(d.DeployLayout) == deploy.LayoutSingle && h.clusterRunsFleet(ctx, p, d.Environment) {
		return true
	}
	serving := strings.TrimSpace(h.clusterServingImageTag(ctx, p, d.Environment))
	// Không thể xác định tag đang phục vụ → không downgrade row nào.
	if serving == "" {
		return true
	}
	if !imageTagsMatch(d.ImageTag, serving) {
		// Tag khác đang phục vụ → hàng này đã lịch sử, giữ nguyên trạng thái.
		return true
	}
	live, detail := h.deploymentTrafficLive(ctx, p, d.Environment, d.ImageTag)
	if live {
		return true
	}
	d.Status = "in_progress"
	d.DeployStatus = "success"
	d.RuntimeStatus = "running"
	if detail != "" {
		d.RuntimeDetail = detail
	}
	d.Serving = false
	d.Live = true
	d.ErrorPhase = ""
	d.ErrorMessage = ""
	_, _ = h.db.Exec(ctx, `
		UPDATE project_deployments SET status='in_progress', deploy_status='success', runtime_status='running',
			runtime_detail=$1, error_phase='', error_message='', updated_at=now()
		WHERE id=$2 AND status='success'`, d.RuntimeDetail, d.ID)
	return false
}
