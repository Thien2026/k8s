package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/domains"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) AddProjectDomain(w http.ResponseWriter, r *http.Request) {
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
		Hostname    string `json:"hostname"`
		Environment string `json:"environment"`
		TLSEnabled  *bool  `json:"tls_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload không hợp lệ"})
		return
	}
	host := strings.TrimSpace(strings.ToLower(body.Hostname))
	if host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "hostname bắt buộc"})
		return
	}
	env := body.Environment
	if env == "" {
		env = "dev"
	}
	if env != "dev" && env != "prod" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment phải là dev hoặc prod"})
		return
	}
	tls := true
	if body.TLSEnabled != nil {
		tls = *body.TLSEnabled
	}
	if ownerID, ownerSlug, taken := h.domainHostnameOwner(r.Context(), host); taken {
		if ownerID == p.ID {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "hostname đã tồn tại trong project"})
		} else {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": fmt.Sprintf("hostname %s đã được project «%s» sử dụng — mỗi domain chỉ gắn một project", host, ownerSlug),
			})
		}
		return
	}
	var id int64
	err := h.db.QueryRow(r.Context(), `
		INSERT INTO project_domains (project_id, hostname, environment, tls_enabled, kind, ingress_name)
		VALUES ($1, $2, $3, $4, 'custom', '') RETURNING id`,
		p.ID, host, env, tls).Scan(&id)
	if err != nil {
		if strings.Contains(err.Error(), "unique") {
			if ownerID, ownerSlug, taken := h.domainHostnameOwner(r.Context(), host); taken && ownerID != p.ID {
				writeJSON(w, http.StatusConflict, map[string]string{
					"error": fmt.Sprintf("hostname %s đã được project «%s» sử dụng", host, ownerSlug),
				})
				return
			}
			writeJSON(w, http.StatusConflict, map[string]string{"error": "hostname đã tồn tại trong project"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	ingressName := domains.IngressName(id)
	_, _ = h.db.Exec(r.Context(), `UPDATE project_domains SET ingress_name=$1 WHERE id=$2`, ingressName, id)

	d := projectDomainRow{
		ID: id, Hostname: host, Environment: env, TLSEnabled: tls,
		Kind: "custom", IngressName: ingressName, SyncStatus: "pending", CertStatus: "unknown",
	}
	cid := r.URL.Query().Get("cluster_id")
	h.syncProjectDomain(r.Context(), p, &d, cid, nil)
	h.enrichProjectDomain(r.Context(), p, &d, cid)
	writeJSON(w, http.StatusCreated, d)
}

func (h *Handler) DeleteProjectDomain(w http.ResponseWriter, r *http.Request) {
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
	var domainID int64
	if _, err := fmt.Sscanf(chi.URLParam(r, "domainId"), "%d", &domainID); err != nil || domainID < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id không hợp lệ"})
		return
	}
	d, err := h.getProjectDomainByID(r.Context(), p.ID, domainID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "domain không tồn tại"})
		return
	}
	cid := r.URL.Query().Get("cluster_id")
	ns := h.projectNamespace(p, d.Environment)
	_ = h.domainSyncer().DeleteIngress(r.Context(), cid, ns, d.ID)
	_, _ = h.db.Exec(r.Context(), `DELETE FROM project_domains WHERE id=$1 AND project_id=$2`, domainID, p.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) TeamUserPicker(w http.ResponseWriter, r *http.Request) {
	list, err := h.auth.ListUsers(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	type pick struct {
		ID          int64  `json:"id"`
		Email       string `json:"email"`
		DisplayName string `json:"display_name,omitempty"`
		Role        string `json:"role"`
	}
	items := make([]pick, 0, len(list))
	for _, u := range list {
		if !u.Active {
			continue
		}
		items = append(items, pick{ID: u.ID, Email: u.Email, DisplayName: u.DisplayName, Role: u.Role})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) listProjectMembers(ctx context.Context, projectID int64) ([]projectMemberRow, error) {
	rows, err := h.db.Query(ctx, `
		SELECT u.id, u.email, COALESCE(u.display_name,''), pm.role
		FROM project_members pm
		INNER JOIN users u ON u.id = pm.user_id
		WHERE pm.project_id = $1
		ORDER BY pm.role, u.email`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []projectMemberRow
	for rows.Next() {
		var m projectMemberRow
		if err := rows.Scan(&m.UserID, &m.Email, &m.DisplayName, &m.Role); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	if list == nil {
		list = []projectMemberRow{}
	}
	return list, rows.Err()
}

func (h *Handler) getProjectRepo(ctx context.Context, projectID int64) (projectRepoRow, error) {
	var repo projectRepoRow
	var updated time.Time
	var synced *time.Time
	err := h.db.QueryRow(ctx, `
		SELECT git_url, branch, COALESCE(build_mode,'dockerfile'), dockerfile_path, build_context,
		       COALESCE(github_owner,''), COALESCE(github_repo,''),
		       COALESCE(deploy_environment,'dev'), workflow_synced_at, auto_deploy_enabled,
		       COALESCE(git_submodules,''), COALESCE(env_contract_build,''), COALESCE(env_contract_runtime,''),
		       COALESCE(workflow_sync_layout,''), COALESCE(workflow_sync_services,''), updated_at
		FROM project_repos WHERE project_id = $1`, projectID).Scan(
		&repo.GitURL, &repo.Branch, &repo.BuildMode, &repo.DockerfilePath, &repo.BuildContext,
		&repo.GitHubOwner, &repo.GitHubRepo, &repo.DeployEnvironment, &synced, &repo.AutoDeployEnabled,
		&repo.GitSubmodules, &repo.EnvContractBuild, &repo.EnvContractRuntime,
		&repo.WorkflowSyncLayout, &repo.WorkflowSyncServices, &updated)
	if err != nil {
		return projectRepoRow{Branch: "main", BuildMode: "dockerfile", DockerfilePath: "Dockerfile", BuildContext: ".", DeployEnvironment: "dev"}, nil
	}
	repo.UpdatedAt = updated.UTC().Format(time.RFC3339)
	if synced != nil {
		repo.WorkflowSyncedAt = synced.UTC().Format(time.RFC3339)
	} else {
		repo.AutoDeployEnabled = false
	}
	return repo, nil
}

func (h *Handler) listProjectDomains(ctx context.Context, projectID int64) ([]projectDomainRow, error) {
	rows, err := h.db.Query(ctx, `
		SELECT id, hostname, environment, tls_enabled,
		       COALESCE(kind,'custom'), COALESCE(ingress_name,''),
		       COALESCE(sync_status,'pending'), COALESCE(sync_error,''),
		       COALESCE(cert_status,'unknown'), created_at
		FROM project_domains WHERE project_id = $1 ORDER BY kind DESC, environment, hostname`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []projectDomainRow
	for rows.Next() {
		var d projectDomainRow
		var created time.Time
		if err := rows.Scan(&d.ID, &d.Hostname, &d.Environment, &d.TLSEnabled,
			&d.Kind, &d.IngressName, &d.SyncStatus, &d.SyncError, &d.CertStatus, &created); err != nil {
			return nil, err
		}
		d.CreatedAt = created.UTC().Format(time.RFC3339)
		list = append(list, d)
	}
	if list == nil {
		list = []projectDomainRow{}
	}
	return list, rows.Err()
}
