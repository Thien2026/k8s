package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/go-chi/chi/v5"
)

type projectBackupArtifact struct {
	ID           int64      `json:"artifact_id"`
	RunID        int64      `json:"run_id"`
	Environment  string     `json:"environment"`
	SourceBucket string     `json:"source_bucket"`
	ObjectPrefix string     `json:"object_prefix"`
	Status       string     `json:"status"`
	ObjectCount  *int64     `json:"object_count,omitempty"`
	TotalBytes   *int64     `json:"total_bytes,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	TargetName   string     `json:"target_name"`
}

// ListProjectBackups chỉ trả artifact đã mirror thành công, được scope cứng theo project/env.
func (h *Handler) ListProjectBackups(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireProjectAccess(w, r, chi.URLParam(r, "slug"))
	if !ok {
		return
	}
	env := strings.TrimSpace(r.URL.Query().Get("environment"))
	if env != "dev" && env != "prod" {
		env = "dev"
	}
	rows, err := h.db.Query(r.Context(), `
		SELECT a.id,a.backup_run_id,a.environment,a.source_bucket,a.object_prefix,a.status,a.object_count,a.total_bytes,
		       a.created_at,r.finished_at,t.name
		FROM project_backup_artifacts a
		JOIN backup_runs r ON r.id=a.backup_run_id
		JOIN backup_targets t ON t.id=r.target_id
		WHERE a.project_id=$1 AND a.environment=$2 AND a.status='success' AND r.status='success'
		ORDER BY a.created_at DESC LIMIT 100`, p.ID, env)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	items := []projectBackupArtifact{}
	for rows.Next() {
		var v projectBackupArtifact
		if err := rows.Scan(&v.ID, &v.RunID, &v.Environment, &v.SourceBucket, &v.ObjectPrefix, &v.Status, &v.ObjectCount, &v.TotalBytes, &v.CreatedAt, &v.FinishedAt, &v.TargetName); err == nil {
			items = append(items, v)
		}
	}
	u, _ := auth.UserFromContext(r.Context())
	writeJSON(w, 200, map[string]any{
		"environment": env, "namespace": map[string]string{"dev": p.NamespaceDev, "prod": p.NamespaceProd}[env],
		// Project tab là inventory/recovery scope, không phải nơi dev tự tạo backup.
		// Restore vào sandbox chỉ admin platform được phép enqueue.
		"can_restore": u.Role == auth.RoleAdmin, "items": items,
	})
}

// CreateProjectBackupRestore chỉ enqueue restore sandbox, prefix do server sinh nên client không thể ghi đè app data.
func (h *Handler) CreateProjectBackupRestore(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireProjectAccess(w, r, chi.URLParam(r, "slug"))
	if !ok {
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	if u.Role != auth.RoleAdmin {
		writeAccessDenied(w)
		return
	}
	var body struct {
		ArtifactID        int64  `json:"artifact_id"`
		TargetEnvironment string `json:"target_environment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ArtifactID < 1 {
		writeJSON(w, 400, map[string]string{"error": "artifact_id hợp lệ là bắt buộc"})
		return
	}
	if body.TargetEnvironment != "dev" && body.TargetEnvironment != "prod" {
		writeJSON(w, 400, map[string]string{"error": "target_environment phải là dev hoặc prod"})
		return
	}
	var sourceBucket string
	if err := h.db.QueryRow(r.Context(), `SELECT a.source_bucket
		FROM project_backup_artifacts a JOIN backup_runs r ON r.id=a.backup_run_id
		WHERE a.id=$1 AND a.project_id=$2 AND a.status='success' AND r.status='success'`,
		body.ArtifactID, p.ID).Scan(&sourceBucket); err != nil {
		writeJSON(w, 404, map[string]string{"error": "backup artifact không tồn tại hoặc chưa sẵn sàng"})
		return
	}
	ns := p.NamespaceDev
	if body.TargetEnvironment == "prod" {
		ns = p.NamespaceProd
	}
	var id int64
	err := h.db.QueryRow(r.Context(), `
		INSERT INTO project_backup_restores (artifact_id,project_id,target_environment,target_namespace,target_bucket,target_prefix,requested_by)
		VALUES ($1,$2,$3,$4,$5,'pending',$6) RETURNING id`,
		body.ArtifactID, p.ID, body.TargetEnvironment, ns, sourceBucket, u.ID).Scan(&id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	prefix := "__restore/" + strconv.FormatInt(id, 10) + "/"
	_, err = h.db.Exec(r.Context(), `UPDATE project_backup_restores SET target_prefix=$1 WHERE id=$2`, prefix, id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	auditAction(r.Context(), h, r, "project.backup.restore.queued", p.Slug, map[string]any{"restore_id": id, "artifact_id": body.ArtifactID, "target_environment": body.TargetEnvironment})
	writeJSON(w, http.StatusAccepted, map[string]any{"restore": map[string]any{"id": id, "status": "queued", "target_prefix": prefix}})
}
