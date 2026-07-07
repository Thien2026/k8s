package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/plugins"
	"github.com/go-chi/chi/v5"
)

// ApplyProjectResources — reapply manifest với tag đang chạy (không rebuild image).
func (h *Handler) ApplyProjectResources(w http.ResponseWriter, r *http.Request) {
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
	var body struct {
		Environment string `json:"environment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload không hợp lệ"})
		return
	}
	env := strings.ToLower(strings.TrimSpace(body.Environment))
	if env == "" {
		env = "dev"
	}
	if env != "dev" && env != "prod" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment phải là dev hoặc prod"})
		return
	}
	if env == "prod" && u.Role != auth.RoleAdmin && u.Role != auth.RoleTechLead {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Chỉ admin/tech_lead được áp dụng resources trên prod"})
		return
	}
	rancherOn, _ := h.plugins.Enabled(r.Context(), plugins.Rancher)
	if !rancherOn || h.rancher == nil || !h.rancher.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Rancher chưa sẵn sàng"})
		return
	}
	tag := strings.TrimSpace(h.clusterServingImageTag(r.Context(), p, env))
	if tag == "" {
		tag = h.latestSuccessfulDeployTag(r.Context(), p.ID, env)
	}
	if tag == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Chưa có bản deploy thành công — deploy lần đầu trước khi áp dụng CPU/RAM",
		})
		return
	}
	if !h.hasSuccessfulDeployForTag(r.Context(), p.ID, env, tag) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Tag " + tag + " chưa từng deploy thành công — chọn bản deploy hợp lệ",
		})
		return
	}
	h.enrichProjectRegistry(r.Context(), &p)
	if err := h.validateRollbackLayoutAllowed(r.Context(), p, env, tag); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	result, err := h.applyProjectDeploy(r.Context(), p, env, tag, "", true, true)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	result["resources_apply"] = true
	result["image_tag"] = tag
	result["message"] = "Đang áp dụng cấu hình CPU/RAM mới — pod sẽ restart"
	writeJSON(w, http.StatusOK, result)
}
