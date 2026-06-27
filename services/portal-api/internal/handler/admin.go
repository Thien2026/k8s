package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) AdminListUsers(w http.ResponseWriter, r *http.Request) {
	list, err := h.auth.ListUsers(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if list == nil {
		list = []auth.User{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list, "total": len(list)})
}

func (h *Handler) AdminCreateUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		Role        string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload không hợp lệ"})
		return
	}
	email := strings.TrimSpace(strings.ToLower(body.Email))
	if email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email bắt buộc"})
		return
	}
	role := body.Role
	if role == "" {
		role = auth.RoleDev
	}
	if !auth.ValidRole(role) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role không hợp lệ"})
		return
	}
	if role == auth.RoleAdmin {
		actor := auth.MustUser(r.Context())
		if actor.Role != auth.RoleAdmin {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "chỉ admin tạo được tài khoản admin"})
			return
		}
	}
	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	id, err := h.auth.CreateUser(r.Context(), email, strings.TrimSpace(body.DisplayName), hash, role)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "email đã tồn tại"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	actor := auth.MustUser(r.Context())
	auditAction(r.Context(), h, r, "user.create", email, map[string]any{"role": role, "by": actor.Email})
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": id, "email": email, "display_name": body.DisplayName, "role": role,
	})
}

func (h *Handler) AdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id không hợp lệ"})
		return
	}
	var body struct {
		DisplayName string `json:"display_name"`
		Role        string `json:"role"`
		Active      *bool  `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload không hợp lệ"})
		return
	}
	if body.Role != "" && !auth.ValidRole(body.Role) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role không hợp lệ"})
		return
	}
	if body.Role == auth.RoleAdmin {
		actor := auth.MustUser(r.Context())
		if actor.Role != auth.RoleAdmin {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "chỉ admin gán role admin"})
			return
		}
	}
	if body.Active != nil && !*body.Active {
		target, err := h.auth.GetUserRowByID(r.Context(), id)
		if err == nil && target.Role == auth.RoleAdmin {
			n, _ := h.auth.CountActiveAdmins(r.Context())
			if n <= 1 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "không thể vô hiệu admin cuối cùng"})
				return
			}
		}
		_ = h.auth.RevokeAllSessions(r.Context(), id)
	}
	if err := h.auth.UpdateUser(r.Context(), id, strings.TrimSpace(body.DisplayName), body.Role, body.Active); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	actor := auth.MustUser(r.Context())
	auditAction(r.Context(), h, r, "user.update", idStr, map[string]any{
		"role": body.Role, "active": body.Active, "by": actor.Email,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) AdminAuditLog(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(r.Context(), `
		SELECT a.id, COALESCE(u.email,''), a.action, a.resource, a.ip_address, a.created_at
		FROM audit_log a
		LEFT JOIN users u ON u.id = a.user_id
		ORDER BY a.created_at DESC
		LIMIT 100`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	type row struct {
		ID        int64  `json:"id"`
		Email     string `json:"email"`
		Action    string `json:"action"`
		Resource  string `json:"resource"`
		IP        string `json:"ip_address"`
		CreatedAt string `json:"created_at"`
	}
	var items []row
	for rows.Next() {
		var item row
		var created time.Time
		if err := rows.Scan(&item.ID, &item.Email, &item.Action, &item.Resource, &item.IP, &created); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		item.CreatedAt = created.UTC().Format(time.RFC3339)
		items = append(items, item)
	}
	if items == nil {
		items = []row{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
