package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
	"github.com/Thien2026/k8s/services/portal-api/internal/platformcontract"
	"github.com/Thien2026/k8s/services/portal-api/internal/plugins"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) PromoteProjectDeploy(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	if !auth.CanWriteK8s(u.Role) {
		writeAccessDenied(w)
		return
	}
	if u.Role != auth.RoleAdmin && u.Role != auth.RoleTechLead {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Chỉ admin/tech_lead được promote lên prod"})
		return
	}
	var body struct {
		ImageTag string `json:"image_tag"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload không hợp lệ"})
		return
	}
	tag := strings.TrimSpace(body.ImageTag)
	if tag == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "image_tag bắt buộc"})
		return
	}
	var devStatus string
	err := h.db.QueryRow(r.Context(), `
		SELECT status FROM project_deployments
		WHERE project_id=$1 AND environment='dev' AND image_tag=$2
		ORDER BY id DESC LIMIT 1`, p.ID, tag).Scan(&devStatus)
	if err != nil || devStatus != "success" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Chỉ promote bản đã deploy dev thành công (status=success)",
		})
		return
	}
	if err := h.validatePromoteLayoutMatch(r.Context(), p.ID, tag); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if h.devUsesRedisAddon(r.Context(), p.ID) && !h.redisProdAddonReady(r.Context(), p.ID) {
		if err := h.promoteRedisAddonFromDev(r.Context(), p); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{
				"error": "Không provision được Redis prod: " + err.Error(),
			})
			return
		}
	}
	if h.minioDevAddonReady(r.Context(), p.ID) && !h.minioProdAddonReady(r.Context(), p.ID) {
		if err := h.promoteMinioAddonFromDev(r.Context(), p); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{
				"error": "Không provision được MinIO prod: " + err.Error(),
			})
			return
		}
	}
	if !h.ensurePromoteReady(w, r, p) {
		return
	}
	rancherOn, _ := h.plugins.Enabled(r.Context(), plugins.Rancher)
	if !rancherOn || h.rancher == nil || !h.rancher.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Rancher chưa sẵn sàng"})
		return
	}
	h.enrichProjectRegistry(r.Context(), &p)
	result, err := h.applyProjectDeploy(r.Context(), p, "prod", tag, "", true, false)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	result["promoted_from"] = "dev"
	result["image_tag"] = tag
	writeJSON(w, http.StatusOK, result)
}

type promoteReadinessItem struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	OK     bool   `json:"ok"`
	Level  string `json:"level,omitempty"` // required | warn
	Group  string `json:"group,omitempty"` // dev_image | prod
	Detail string `json:"detail,omitempty"`
	Tab    string `json:"tab,omitempty"`
}

func (h *Handler) GetPromoteReadiness(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	items, ready := h.promoteReadiness(r.Context(), p)
	latestTag := h.latestSuccessfulDeployTag(r.Context(), p.ID, "dev")
	resp := map[string]any{
		"ready":              ready,
		"items":              items,
		"latest_success_tag": latestTag,
		"can_promote":        latestTag != "",
	}
	if br := h.projectEnvReadinessPayload(r.Context(), p, "dev", "build"); br != nil {
		resp["build_readiness"] = br
	}
	if rr := h.projectEnvReadinessPayload(r.Context(), p, "prod", "runtime"); rr != nil {
		resp["runtime_readiness"] = rr
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) promoteReadiness(ctx context.Context, p projectRow) ([]promoteReadinessItem, bool) {
	var items []promoteReadinessItem
	allOK := true

	markRequired := func(it *promoteReadinessItem) {
		it.Level = "required"
		if !it.OK {
			allOK = false
		}
	}
	markWarn := func(it *promoteReadinessItem) {
		it.Level = "warn"
	}

	nsProd := strings.TrimSpace(p.NamespaceProd)
	nsItem := promoteReadinessItem{
		ID:    "namespace_prod",
		Label: "Namespace prod",
		OK:    nsProd != "",
		Group: "prod",
		Tab:   "settings",
	}
	if nsItem.OK {
		nsItem.Detail = nsProd
		if nsProd == strings.TrimSpace(p.NamespaceDev) {
			nsItem.OK = false
			nsItem.Detail = nsProd + " — trùng namespace dev (nên tách riêng)"
		}
	} else {
		nsItem.Detail = "Chưa cấu hình namespace prod trong Cài đặt project"
	}
	markRequired(&nsItem)
	items = append(items, nsItem)

	devEnv, _ := h.listProjectEnvVars(ctx, p.ID, "dev", "runtime")
	prodEnv, _ := h.listProjectEnvVars(ctx, p.ID, "prod", "runtime")
	devKeys := map[string]bool{}
	for _, row := range devEnv {
		devKeys[row.Key] = true
	}
	var missing []string
	for k := range devKeys {
		if isAddonManagedRuntimeEnvKey(k) {
			continue
		}
		found := false
		for _, row := range prodEnv {
			if row.Key == k {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, k)
		}
	}
	envItem := promoteReadinessItem{
		ID:    "env_vars",
		Label: "Env vars prod",
		Group: "prod",
		Tab:   "env",
	}
	if len(devKeys) == 0 {
		envItem.OK = true
		envItem.Detail = "Dev chưa có env — prod không bắt buộc (nên thêm nếu app cần)"
	} else if len(missing) == 0 {
		envItem.OK = true
		envItem.Detail = fmt.Sprintf("Đủ %d biến giống dev", len(devKeys))
	} else {
		envItem.OK = false
		envItem.Detail = "Thiếu trên prod: " + strings.Join(missing, ", ")
	}
	markRequired(&envItem)
	items = append(items, envItem)

	buildCheck, _ := h.evaluateBuildConfigCached(ctx, p, "dev")
	buildItem := promoteReadinessItem{
		ID:    "build_contract_dev",
		Label: "Build contract (dev)",
		Group: "dev_image",
		Tab:   "env",
	}
	if !buildCheck.ContractFound {
		buildItem.OK = true
		buildItem.Detail = "Chưa có .platform/build.yaml trên GitHub"
	} else if !buildCheck.Ready {
		buildItem.OK = false
		if msg := platformcontract.FormatCheckError(buildCheck); msg != "" {
			buildItem.Detail = msg
		} else {
			buildItem.Detail = "Cấu hình build dev chưa sẵn sàng"
		}
	} else {
		buildItem.OK = true
		buildItem.Detail = "Đủ theo .platform/build.yaml"
	}
	markRequired(&buildItem)
	items = append(items, buildItem)

	repo, _ := h.getProjectRepo(ctx, p.ID)
	wfItem := promoteReadinessItem{
		ID:    "workflow_sync",
		Label: "Workflow GitHub",
		Group: "dev_image",
		Tab:   "deploy",
	}
	if reason := h.workflowStaleReason(ctx, p.ID, repo); reason != "" {
		wfItem.OK = false
		wfItem.Detail = reason
	} else {
		layout := h.getProjectLayout(ctx, p.ID)
		_, svcKey := h.currentWorkflowProfile(ctx, p.ID, repo)
		wfItem.OK = true
		wfItem.Detail = "Khớp Console · " + workflowProfileLabel(layout, svcKey)
	}
	markRequired(&wfItem)
	items = append(items, wfItem)

	if h.getProjectLayout(ctx, p.ID) == deploy.LayoutMulti {
		multiItem := promoteReadinessItem{
			ID:    "multi_fleet_dev",
			Label: "Fleet dev (multi)",
			Group: "dev_image",
			Tab:   "deploy",
		}
		tag := h.latestSuccessfulDeployTag(ctx, p.ID, "dev")
		if tag == "" {
			multiItem.OK = false
			multiItem.Detail = "Chưa có bản dev success — deploy multi lên dev trước"
		} else {
			multiItem.OK = true
			multiItem.Detail = "Tag " + shortImageTag(tag) + " — promote sẽ đưa cả api+web cùng tag lên prod"
		}
		markRequired(&multiItem)
		items = append(items, multiItem)
	}

	rtCheck, _ := h.evaluateRuntimeConfigCached(ctx, p, "prod")
	rtItem := promoteReadinessItem{
		ID:    "runtime_contract",
		Label: "Runtime contract (prod)",
		Group: "prod",
		Tab:   "env",
	}
	if !rtCheck.ContractFound {
		rtItem.OK = true
		rtItem.Detail = "Chưa có .platform/runtime.yaml trên GitHub"
	} else if !rtCheck.Ready {
		rtItem.OK = false
		if msg := platformcontract.FormatCheckError(rtCheck); msg != "" {
			rtItem.Detail = msg
		} else {
			rtItem.Detail = "Cấu hình runtime prod chưa sẵn sàng"
		}
	} else {
		rtItem.OK = true
		rtItem.Detail = "Đủ theo .platform/runtime.yaml"
	}
	markRequired(&rtItem)
	items = append(items, rtItem)

	domains, _ := h.listProjectDomains(ctx, p.ID)
	var prodHosts []string
	for _, d := range domains {
		if strings.EqualFold(d.Environment, "prod") {
			prodHosts = append(prodHosts, d.Hostname)
		}
	}
	domItem := promoteReadinessItem{
		ID:    "domain_prod",
		Label: "Domain prod",
		Group: "prod",
		Tab:   "domains",
	}
	if len(prodHosts) > 0 {
		domItem.OK = true
		domItem.Detail = strings.Join(prodHosts, ", ")
	} else {
		domItem.OK = false
		domItem.Detail = "Chưa có domain prod — thêm tại tab Domains"
	}
	markRequired(&domItem)
	items = append(items, domItem)

	redisItem := h.redisAddonPromoteReadiness(ctx, p)
	markRequired(&redisItem)
	items = append(items, redisItem)

	minioItem := h.minioAddonPromoteReadiness(ctx, p)
	markRequired(&minioItem)
	items = append(items, minioItem)

	h.enrichProjectRegistry(ctx, &p)
	if strings.EqualFold(strings.TrimSpace(p.RegistryProvider), "harbor") && h.harbor != nil && h.harbor.Enabled() {
		var devTag string
		_ = h.db.QueryRow(ctx, `
			SELECT image_tag FROM project_deployments
			WHERE project_id=$1 AND environment='dev' AND status='success'
			ORDER BY id DESC LIMIT 1`, p.ID).Scan(&devTag)
		scanItem := promoteReadinessItem{
			ID:    "harbor_scan",
			Label: "Bảo mật image (Trivy)",
			Group: "dev_image",
			Tab:   "deploy",
		}
		if devTag == "" {
			scanItem.OK = true
			scanItem.Detail = "Chưa có bản dev success để quét — promote vẫn được nếu đủ điều kiện khác"
		} else {
			projectName := strings.TrimSpace(p.HarborProject)
			if projectName == "" {
				projectName = p.Slug
			}
			ov, err := h.harbor.ArtifactScanOverview(ctx, projectName, "app", devTag)
			if err != nil || ov == nil {
				scanItem.OK = true
				scanItem.Detail = "Chưa đọc được kết quả quét — không chặn promote"
			} else {
				crit := ov.Severity["Critical"]
				high := ov.Severity["High"]
				scanItem.Detail = ov.Detail
				scanItem.OK = crit == 0
				if crit > 0 {
					scanItem.Detail += " — nên sửa base image trước promote (không chặn cứng)"
				} else if high > 0 {
					scanItem.Detail += " — có High, cân nhắc trước prod"
				}
			}
		}
		markWarn(&scanItem)
		items = append(items, scanItem)
	}

	return items, allOK
}

func (h *Handler) ensurePromoteReady(w http.ResponseWriter, r *http.Request, p projectRow) bool {
	items, ready := h.promoteReadiness(r.Context(), p)
	if ready {
		return true
	}
	var hints []string
	for _, it := range items {
		if !it.OK && it.Detail != "" {
			hints = append(hints, it.Label+": "+it.Detail)
		}
	}
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"error": "Chưa đủ cấu hình prod — hoàn tất checklist trước khi promote",
		"items": items,
		"ready": false,
		"hints": hints,
	})
	return false
}

func (h *Handler) latestSuccessfulDeployTag(ctx context.Context, projectID int64, env string) string {
	env = strings.ToLower(strings.TrimSpace(env))
	if env == "" {
		env = "dev"
	}
	layout := h.getProjectLayout(ctx, projectID)
	var tag string
	_ = h.db.QueryRow(ctx, `
		SELECT image_tag FROM project_deployments
		WHERE project_id=$1 AND environment=$2 AND status='success'
		  AND COALESCE(deploy_layout,'single')=$3
		ORDER BY id DESC LIMIT 1`, projectID, env, layout).Scan(&tag)
	return strings.TrimSpace(tag)
}
