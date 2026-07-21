package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/go-chi/chi/v5"
)

// GetProjectQuota GET /admin/projects/{slug}/quota — xem quota dev+prod của 1 project.
func (h *Handler) GetProjectQuota(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, err := h.getProjectBySlug(r.Context(), slug)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project không tồn tại"})
		return
	}
	dev, err := h.loadProjectEnvQuota(r.Context(), p.ID, "dev")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	prod, err := h.loadProjectEnvQuota(r.Context(), p.ID, "prod")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"slug": p.Slug,
		"dev":  dev,
		"prod": prod,
	})
}

// PatchProjectQuota PATCH /admin/projects/{slug}/quota — admin nâng/hạ quota (override) 1 env.
// Body: {"environment":"dev|prod","storage_gb":..,"memory_mb":..,"cpu_m":..,"max_pods":..,"max_pvcs":..}
func (h *Handler) PatchProjectQuota(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	u, _ := auth.UserFromContext(r.Context())
	p, err := h.getProjectBySlug(r.Context(), slug)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project không tồn tại"})
		return
	}
	var body struct {
		Environment string `json:"environment"`
		StorageGB   *int   `json:"storage_gb"`
		MemoryMB    *int   `json:"memory_mb"`
		CPUm        *int   `json:"cpu_m"`
		MaxPods     *int   `json:"max_pods"`
		MaxPVCs     *int   `json:"max_pvcs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body không hợp lệ"})
		return
	}
	env := strings.TrimSpace(body.Environment)
	if env != "dev" && env != "prod" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment phải là dev hoặc prod"})
		return
	}
	cur, err := h.loadProjectEnvQuota(r.Context(), p.ID, env)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	next := cur
	if body.StorageGB != nil {
		next.StorageGB = *body.StorageGB
	}
	if body.MemoryMB != nil {
		next.MemoryMB = *body.MemoryMB
	}
	if body.CPUm != nil {
		next.CPUm = *body.CPUm
	}
	if body.MaxPods != nil {
		next.MaxPods = *body.MaxPods
	}
	if body.MaxPVCs != nil {
		next.MaxPVCs = *body.MaxPVCs
	}
	// Ràng buộc cơ bản (khớp CHECK trong migration).
	if next.StorageGB < 1 || next.StorageGB > 2000 ||
		next.MemoryMB < 128 || next.MemoryMB > 65536 ||
		next.CPUm < 100 || next.CPUm > 64000 ||
		next.MaxPods < 1 || next.MaxPods > 500 ||
		next.MaxPVCs < 1 || next.MaxPVCs > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "giá trị quota ngoài khoảng cho phép"})
		return
	}
	_, err = h.db.Exec(r.Context(), `
		UPDATE project_env_quota
		SET storage_gb=$3, memory_mb=$4, cpu_m=$5, max_pods=$6, max_pvcs=$7,
		    is_override=true, updated_by=$8, updated_at=now()
		WHERE project_id=$1 AND environment=$2`,
		p.ID, env, next.StorageGB, next.MemoryMB, next.CPUm, next.MaxPods, next.MaxPVCs, u.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	next.IsOverride = true
	// Apply ResourceQuota mới lên namespace ngay.
	applyErr := h.syncProjectEnvQuota(r.Context(), p, env)
	auditAction(r.Context(), h, r, "project.quota.override", slug, map[string]any{
		"environment": env, "storage_gb": next.StorageGB, "memory_mb": next.MemoryMB,
		"cpu_m": next.CPUm, "max_pods": next.MaxPods, "max_pvcs": next.MaxPVCs, "by": u.Email,
	})
	resp := map[string]any{"status": "ok", "environment": env, "quota": next}
	if applyErr != nil {
		resp["warning"] = "Lưu DB OK nhưng apply ResourceQuota lỗi: " + applyErr.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}

// PostBackfillProjectQuotas POST /admin/projects/quota/backfill — apply ResourceQuota cho mọi project.
func (h *Handler) PostBackfillProjectQuotas(w http.ResponseWriter, r *http.Request) {
	applied, warns := h.backfillAllProjectQuotas(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"applied":  applied,
		"warnings": warns,
	})
}
