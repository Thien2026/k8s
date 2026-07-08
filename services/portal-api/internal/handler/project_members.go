package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) ListProjectMembers(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	members, err := h.listProjectMembers(r.Context(), p.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": members})
}

func (h *Handler) AddProjectMember(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	if !h.canManageProject(r.Context(), u, p.ID) {
		writeAccessDenied(w)
		return
	}
	var body struct {
		UserID int64  `json:"user_id"`
		Role   string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.UserID < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id bắt buộc"})
		return
	}
	role := body.Role
	if role == "" {
		role = "dev"
	}
	if role != "dev" && role != "readonly" && role != "owner" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role không hợp lệ"})
		return
	}
	_, err := h.db.Exec(r.Context(),
		`INSERT INTO project_members (project_id, user_id, role) VALUES ($1, $2, $3)
		 ON CONFLICT (project_id, user_id) DO UPDATE SET role = EXCLUDED.role`,
		p.ID, body.UserID, role)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
}

func (h *Handler) RemoveProjectMember(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	if !h.canManageProject(r.Context(), u, p.ID) {
		writeAccessDenied(w)
		return
	}
	var uid int64
	if _, err := fmt.Sscanf(chi.URLParam(r, "userId"), "%d", &uid); err != nil || uid < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user id không hợp lệ"})
		return
	}
	var role string
	_ = h.db.QueryRow(r.Context(),
		`SELECT role FROM project_members WHERE project_id=$1 AND user_id=$2`, p.ID, uid).Scan(&role)
	if role == "owner" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "không thể xóa owner"})
		return
	}
	_, _ = h.db.Exec(r.Context(), `DELETE FROM project_members WHERE project_id=$1 AND user_id=$2`, p.ID, uid)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
