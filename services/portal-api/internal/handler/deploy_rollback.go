package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/plugins"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) RollbackProjectDeploy(w http.ResponseWriter, r *http.Request) {
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
		ImageTag    string `json:"image_tag"`
		Environment string `json:"environment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload không hợp lệ"})
		return
	}
	tag := strings.TrimSpace(body.ImageTag)
	env := strings.ToLower(strings.TrimSpace(body.Environment))
	if tag == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "image_tag bắt buộc"})
		return
	}
	if env == "" {
		env = "dev"
	}
	if env != "dev" && env != "prod" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment phải là dev hoặc prod"})
		return
	}
	if env == "prod" && u.Role != auth.RoleAdmin && u.Role != auth.RoleTechLead {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Chỉ admin/tech_lead được rollback prod"})
		return
	}
	if !h.hasSuccessfulDeployForTag(r.Context(), p.ID, env, tag) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Chỉ rollback về bản đã từng deploy thành công trong lịch sử",
		})
		return
	}
	rancherOn, _ := h.plugins.Enabled(r.Context(), plugins.Rancher)
	if !rancherOn || h.rancher == nil || !h.rancher.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Rancher chưa sẵn sàng"})
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
	result["rollback"] = true
	result["image_tag"] = tag
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) PatchProjectAutoDeploy(w http.ResponseWriter, r *http.Request) {
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
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Enabled == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "enabled (boolean) bắt buộc"})
		return
	}
	repo, err := h.getProjectRepo(r.Context(), p.ID)
	if err != nil || repo.WorkflowSyncedAt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Chưa kết nối GitHub / chưa sync workflow — kết nối repo trước",
		})
		return
	}
	_, err = h.db.Exec(r.Context(), `
		UPDATE project_repos SET auto_deploy_enabled=$1, updated_at=now() WHERE project_id=$2`,
		*body.Enabled, p.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":              "ok",
		"auto_deploy_enabled": *body.Enabled,
		"auto_deploy":         *body.Enabled,
	})
}
