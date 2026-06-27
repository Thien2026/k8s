package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
	"github.com/Thien2026/k8s/services/portal-api/internal/platformcontract"
	"github.com/go-chi/chi/v5"
)

type projectEnvVarRow struct {
	ID          int64  `json:"id"`
	Key         string `json:"key"`
	Value       string `json:"value"`
	IsSecret    bool   `json:"is_secret"`
	Environment string `json:"environment"`
	Scope       string `json:"scope"`
	HasValue    bool   `json:"has_value,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

func validEnvScope(scope string) (string, bool) {
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope == "" {
		scope = "runtime"
	}
	if scope != "runtime" && scope != "build" {
		return "", false
	}
	return scope, true
}

// optionalEnvScope — chỉ dùng khi lọc list: rỗng = trả mọi scope (runtime + build).
func optionalEnvScope(scope string) (string, bool) {
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope == "" {
		return "", true
	}
	if scope != "runtime" && scope != "build" {
		return "", false
	}
	return scope, true
}

func validProjectEnv(env string) (string, bool) {
	env = strings.ToLower(strings.TrimSpace(env))
	if env == "" {
		env = "dev"
	}
	if env != "dev" && env != "prod" {
		return "", false
	}
	return env, true
}

func normalizeEnvKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("key bắt buộc")
	}
	if strings.Contains(key, " ") {
		return "", fmt.Errorf("key không được chứa khoảng trắng")
	}
	return key, nil
}

func (h *Handler) canEditProjectEnv(u auth.User, env string) bool {
	if env == "prod" && !auth.CanWriteProd(u.Role) {
		return false
	}
	return auth.CanWriteK8s(u.Role)
}

func (h *Handler) listProjectEnvVars(ctx context.Context, projectID int64, env, scope string) ([]projectEnvVarRow, error) {
	q := `
		SELECT id, key, value, is_secret, environment, scope, created_at, updated_at
		FROM project_env_vars WHERE project_id = $1`
	args := []any{projectID}
	n := 2
	if env != "" {
		q += fmt.Sprintf(` AND environment = $%d`, n)
		args = append(args, env)
		n++
	}
	if scope != "" {
		q += fmt.Sprintf(` AND scope = $%d`, n)
		args = append(args, scope)
		n++
	}
	q += ` ORDER BY scope, environment, key`
	rows, err := h.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []projectEnvVarRow
	for rows.Next() {
		var row projectEnvVarRow
		var created, updated time.Time
		if err := rows.Scan(&row.ID, &row.Key, &row.Value, &row.IsSecret, &row.Environment, &row.Scope, &created, &updated); err != nil {
			return nil, err
		}
		row.HasValue = row.Value != ""
		if row.IsSecret {
			row.Value = maskSecretValue(row.Value)
		}
		row.CreatedAt = created.UTC().Format(time.RFC3339)
		row.UpdatedAt = updated.UTC().Format(time.RFC3339)
		list = append(list, row)
	}
	if list == nil {
		list = []projectEnvVarRow{}
	}
	return list, rows.Err()
}

func maskSecretValue(v string) string {
	if v == "" {
		return ""
	}
	return "••••••••"
}

func (h *Handler) envVarsMap(ctx context.Context, projectID int64, env string) (map[string]string, error) {
	return h.envVarsMapScoped(ctx, projectID, env, "runtime")
}

func (h *Handler) envVarsMapScoped(ctx context.Context, projectID int64, env, scope string) (map[string]string, error) {
	rows, err := h.db.Query(ctx, `
		SELECT key, value FROM project_env_vars
		WHERE project_id = $1 AND environment = $2 AND scope = $3 ORDER BY key`,
		projectID, env, scope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// syncAppEnvSecret apply Secret app-env lên cluster; restart deployment nếu có.
func (h *Handler) syncAppEnvSecret(ctx context.Context, p projectRow, env, clusterID string, restart bool) error {
	if h.rancher == nil || !h.rancher.Enabled() {
		return fmt.Errorf("Rancher chưa sẵn sàng")
	}
	ns := h.projectNamespace(p, env)
	if err := h.rancher.EnsureNamespace(ctx, clusterID, ns); err != nil {
		return err
	}
	vars, err := h.envVarsMap(ctx, p.ID, env)
	if err != nil {
		return err
	}
	secretJSON, err := deploy.AppEnvSecret(ns, vars)
	if err != nil {
		return err
	}
	if secretJSON != nil {
		if err := h.rancher.ApplyNamespacedObject(ctx, clusterID, "/api/v1/secrets", ns, secretJSON); err != nil {
			return err
		}
	} else {
		_ = h.rancher.DeleteNamespacedObject(ctx, clusterID, "/api/v1/secrets", ns, deploy.AppEnvSecretName)
	}
	if restart {
		_ = h.rancher.RolloutRestartDeployment(ctx, clusterID, ns, "app")
	}
	return nil
}

func (h *Handler) attachAppEnvToDeploy(ctx context.Context, p projectRow, env, clusterID string) error {
	vars, err := h.envVarsMap(ctx, p.ID, env)
	if err != nil {
		return err
	}
	if len(vars) == 0 {
		return nil
	}
	return h.syncAppEnvSecret(ctx, p, env, clusterID, false)
}

func (h *Handler) ListProjectEnvVars(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	env, _ := validProjectEnv(r.URL.Query().Get("environment"))
	scope, ok := optionalEnvScope(r.URL.Query().Get("scope"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "scope phải là runtime hoặc build"})
		return
	}
	list, err := h.listProjectEnvVars(r.Context(), p.ID, env, scope)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list, "environment": env})
}

func (h *Handler) CreateProjectEnvVar(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	var body struct {
		Key         string `json:"key"`
		Value       string `json:"value"`
		IsSecret    bool   `json:"is_secret"`
		Environment string `json:"environment"`
		Scope       string `json:"scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload không hợp lệ"})
		return
	}
	env, ok := validProjectEnv(body.Environment)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment phải là dev hoặc prod"})
		return
	}
	scope, ok := validEnvScope(body.Scope)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "scope phải là runtime hoặc build"})
		return
	}
	if !h.canEditProjectEnv(u, env) {
		writeAccessDenied(w)
		return
	}
	key, err := normalizeEnvKey(body.Key)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	contract := h.contractForSave(r.Context(), u.ID, p, scope)
	if err := platformcontract.ValidateSaveValue(scope, key, body.Value, contract); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	var id int64
	err = h.db.QueryRow(r.Context(), `
		INSERT INTO project_env_vars (project_id, environment, key, value, is_secret, scope)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		p.ID, env, key, body.Value, body.IsSecret, scope).Scan(&id)
	if err != nil {
		if strings.Contains(err.Error(), "unique") {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "key đã tồn tại trong môi trường này"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	cid := r.URL.Query().Get("cluster_id")
	var syncErr error
	if scope == "runtime" {
		syncErr = h.syncAppEnvSecret(r.Context(), p, env, cid, true)
	} else {
		syncErr = h.syncProjectGitHubWorkflow(r.Context(), u, p)
	}
	row := projectEnvVarRow{
		ID: id, Key: key, Environment: env, Scope: scope, IsSecret: body.IsSecret, HasValue: body.Value != "",
	}
	if body.IsSecret {
		row.Value = maskSecretValue(body.Value)
	} else {
		row.Value = body.Value
	}
	resp := map[string]any{"item": row}
	if syncErr != nil {
		resp["sync_warning"] = syncErr.Error()
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (h *Handler) PatchProjectEnvVar(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	var envVarID int64
	if _, err := fmt.Sscanf(chi.URLParam(r, "envVarId"), "%d", &envVarID); err != nil || envVarID < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id không hợp lệ"})
		return
	}
	var body struct {
		Value    *string `json:"value"`
		IsSecret *bool   `json:"is_secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload không hợp lệ"})
		return
	}
	var curKey, curEnv, curValue, curScope string
	var curSecret bool
	err := h.db.QueryRow(r.Context(), `
		SELECT key, value, is_secret, environment, scope FROM project_env_vars
		WHERE id = $1 AND project_id = $2`, envVarID, p.ID).Scan(&curKey, &curValue, &curSecret, &curEnv, &curScope)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "biến không tồn tại"})
		return
	}
	if !h.canEditProjectEnv(u, curEnv) {
		writeAccessDenied(w)
		return
	}
	newValue := curValue
	if body.Value != nil {
		newValue = *body.Value
	}
	newSecret := curSecret
	if body.IsSecret != nil {
		newSecret = *body.IsSecret
	}
	contract := h.contractForSave(r.Context(), u.ID, p, curScope)
	if err := platformcontract.ValidateSaveValue(curScope, curKey, newValue, contract); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	_, err = h.db.Exec(r.Context(), `
		UPDATE project_env_vars SET value=$1, is_secret=$2, updated_at=now()
		WHERE id=$3 AND project_id=$4`, newValue, newSecret, envVarID, p.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	cid := r.URL.Query().Get("cluster_id")
	var syncErr error
	if curScope == "build" {
		syncErr = h.syncProjectGitHubWorkflow(r.Context(), u, p)
	} else {
		syncErr = h.syncAppEnvSecret(r.Context(), p, curEnv, cid, true)
	}
	row := projectEnvVarRow{
		ID: envVarID, Key: curKey, Environment: curEnv, Scope: curScope, IsSecret: newSecret, HasValue: newValue != "",
	}
	if newSecret {
		row.Value = maskSecretValue(newValue)
	} else {
		row.Value = newValue
	}
	resp := map[string]any{"item": row}
	if syncErr != nil {
		resp["sync_warning"] = syncErr.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) DeleteProjectEnvVar(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	var envVarID int64
	if _, err := fmt.Sscanf(chi.URLParam(r, "envVarId"), "%d", &envVarID); err != nil || envVarID < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id không hợp lệ"})
		return
	}
	var curEnv, curScope string
	err := h.db.QueryRow(r.Context(), `
		SELECT environment, scope FROM project_env_vars WHERE id=$1 AND project_id=$2`, envVarID, p.ID).Scan(&curEnv, &curScope)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "biến không tồn tại"})
		return
	}
	if !h.canEditProjectEnv(u, curEnv) {
		writeAccessDenied(w)
		return
	}
	_, _ = h.db.Exec(r.Context(), `DELETE FROM project_env_vars WHERE id=$1 AND project_id=$2`, envVarID, p.ID)
	cid := r.URL.Query().Get("cluster_id")
	var syncErr error
	if curScope == "build" {
		syncErr = h.syncProjectGitHubWorkflow(r.Context(), u, p)
	} else {
		syncErr = h.syncAppEnvSecret(r.Context(), p, curEnv, cid, true)
	}
	if syncErr != nil {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "sync_warning": syncErr.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) SyncProjectEnvVars(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	env, ok := validProjectEnv(r.URL.Query().Get("environment"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment phải là dev hoặc prod"})
		return
	}
	if !h.canEditProjectEnv(u, env) {
		writeAccessDenied(w)
		return
	}
	cid := r.URL.Query().Get("cluster_id")
	if err := h.requireRuntimeConfigReady(r.Context(), u.ID, p, env); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := h.syncAppEnvSecret(r.Context(), p, env, cid, true); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "namespace": h.projectNamespace(p, env)})
}
