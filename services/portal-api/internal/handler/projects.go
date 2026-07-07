package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
	"github.com/Thien2026/k8s/services/portal-api/internal/domains"
	"github.com/Thien2026/k8s/services/portal-api/internal/plugins"
	"github.com/Thien2026/k8s/services/portal-api/internal/registry"
	"github.com/go-chi/chi/v5"
)

type projectRow struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Slug          string `json:"slug"`
	Layout        string `json:"layout"`
	Description   string `json:"description,omitempty"`
	NamespaceDev  string `json:"namespace_dev"`
	NamespaceProd string `json:"namespace_prod"`
	HarborProject      string `json:"harbor_project,omitempty"`
	RegistryProvider   string `json:"registry_provider"`
	Registry           registry.ProjectRegistry `json:"registry,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
}

type projectMemberRow struct {
	UserID      int64  `json:"user_id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name,omitempty"`
	Role        string `json:"role"`
}

type projectRepoRow struct {
	GitURL             string `json:"git_url"`
	Branch             string `json:"branch"`
	BuildMode             string `json:"build_mode"`
	BuildModeDetectedPath string `json:"build_mode_detected_path,omitempty"`
	DockerfilePath        string `json:"dockerfile_path"`
	BuildContext       string `json:"build_context"`
	GitHubOwner       string `json:"github_owner,omitempty"`
	GitHubRepo        string `json:"github_repo,omitempty"`
	DeployEnvironment string `json:"deploy_environment,omitempty"`
	WorkflowSyncedAt     string `json:"workflow_synced_at,omitempty"`
	WorkflowSyncLayout   string `json:"workflow_sync_layout,omitempty"`
	WorkflowSyncServices string `json:"workflow_sync_services,omitempty"`
	WorkflowStale        bool   `json:"workflow_stale,omitempty"`
	WorkflowStaleReason  string `json:"workflow_stale_reason,omitempty"`
	AutoDeployEnabled    bool   `json:"auto_deploy_enabled"`
	GitSubmodules        string `json:"git_submodules,omitempty"`
	EnvContractBuild  string `json:"-"`
	EnvContractRuntime string `json:"-"`
	UpdatedAt         string `json:"updated_at,omitempty"`
}

type projectDomainRow struct {
	ID          int64              `json:"id"`
	Hostname    string             `json:"hostname"`
	Environment string             `json:"environment"`
	TLSEnabled  bool               `json:"tls_enabled"`
	Kind        string             `json:"kind"`
	IngressName string             `json:"ingress_name,omitempty"`
	SyncStatus  string             `json:"sync_status"`
	SyncError   string             `json:"sync_error,omitempty"`
	CertStatus  string             `json:"cert_status"`
	URL         string             `json:"url,omitempty"`
	DNS         domains.DNSHint    `json:"dns,omitempty"`
	CreatedAt   string             `json:"created_at"`
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 48 {
		s = s[:48]
	}
	return s
}

func (h *Handler) getProjectBySlug(ctx context.Context, slug string) (projectRow, error) {
	var p projectRow
	var created time.Time
	err := h.db.QueryRow(ctx, `
		SELECT id, name, slug, COALESCE(layout,'single'), COALESCE(description,''), namespace_dev, namespace_prod,
		       COALESCE(harbor_project,''), COALESCE(registry_provider,'ghcr'), created_at
		FROM projects WHERE slug = $1`, slug).Scan(
		&p.ID, &p.Name, &p.Slug, &p.Layout, &p.Description, &p.NamespaceDev, &p.NamespaceProd,
		&p.HarborProject, &p.RegistryProvider, &created,
	)
	if err == nil {
		p.CreatedAt = created.UTC().Format(time.RFC3339)
	}
	return p, err
}

func (h *Handler) enrichProjectRegistry(ctx context.Context, p *projectRow) {
	if h.registry == nil {
		return
	}
	p.Registry = h.registry.ProjectRegistry(ctx, p.RegistryProvider, p.Slug, p.HarborProject)
	if p.RegistryProvider != registry.Harbor {
		if strings.TrimSpace(h.cfg.GHCROrg) == "" {
			p.Registry.Ready = false
			p.Registry.ReadyHint = "Thiếu GHCR_ORG trên VPS"
			return
		}
		if !h.ghcrPullConfigured(ctx, p.ID) {
			p.Registry.Ready = false
			p.Registry.ReadyHint = "Thiếu GHCR_PULL_TOKEN (PAT read:packages) trên VPS"
			return
		}
		p.Registry.Ready = true
		p.Registry.ReadyHint = "GHCR_ORG + pull token OK"
	}
}

func (h *Handler) userCanAccessProject(ctx context.Context, u auth.User, projectID int64) bool {
	if auth.CanViewAllProjects(u.Role) {
		return true
	}
	var exists bool
	_ = h.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND user_id=$2)`,
		projectID, u.ID).Scan(&exists)
	return exists
}

func (h *Handler) canManageProject(ctx context.Context, u auth.User, projectID int64) bool {
	if auth.CanViewAllProjects(u.Role) {
		return true
	}
	var role string
	err := h.db.QueryRow(ctx,
		`SELECT role FROM project_members WHERE project_id=$1 AND user_id=$2`,
		projectID, u.ID).Scan(&role)
	return err == nil && role == "owner"
}

func (h *Handler) requireProjectAccess(w http.ResponseWriter, r *http.Request, slug string) (projectRow, bool) {
	u, _ := auth.UserFromContext(r.Context())
	p, err := h.getProjectBySlug(r.Context(), slug)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project không tồn tại"})
		return projectRow{}, false
	}
	if !h.userCanAccessProject(r.Context(), u, p.ID) {
		writeAccessDenied(w)
		return projectRow{}, false
	}
	return p, true
}

func (h *Handler) GetProject(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	members, _ := h.listProjectMembers(r.Context(), p.ID)
	repo, _ := h.getProjectRepo(r.Context(), p.ID)
	h.enrichRepoWorkflowStatus(r.Context(), p.ID, &repo)
	services, _ := h.listProjectServices(r.Context(), p.ID)
	if p.Layout == "" {
		p.Layout = h.getProjectLayout(r.Context(), p.ID)
	}
	u, _ := auth.UserFromContext(r.Context())
	_ = h.resolveBuildMode(r.Context(), u.ID, p.ID, &repo)
	_ = h.ensureAutoDomains(r.Context(), p)
	domainsList, _ := h.listProjectDomainsEnriched(r.Context(), p, r.URL.Query().Get("cluster_id"))
	h.enrichProjectRegistry(r.Context(), &p)
	writeJSON(w, http.StatusOK, map[string]any{
		"project":  p,
		"members":  members,
		"repo":     repo,
		"domains":  domainsList,
		"services": map[string]any{"layout": p.Layout, "items": services},
	})
}

func (h *Handler) ProjectOverview(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	out := map[string]any{
		"project": p,
		"dev":     map[string]any{"namespace": p.NamespaceDev},
		"prod":    map[string]any{"namespace": p.NamespaceProd},
	}
	if h.monitoringConfigured() {
		out["monitoring"] = map[string]any{
			"grafana_url": trimURL(h.cfg.GrafanaURL),
			"dev_dashboard_url":  grafanaNamespaceDashboardURL(h.cfg.GrafanaURL, p.NamespaceDev),
			"prod_dashboard_url": grafanaNamespaceDashboardURL(h.cfg.GrafanaURL, p.NamespaceProd),
		}
	}
	if h.rancher != nil && h.rancher.Enabled() {
		cid := r.URL.Query().Get("cluster_id")
		if pods, err := h.rancher.ListK8s(r.Context(), cid, "pods", p.NamespaceDev, 1, 500); err == nil {
			out["dev"].(map[string]any)["pods"] = len(pods.Items)
		}
		if dep, err := h.rancher.ListK8s(r.Context(), cid, "deployments", p.NamespaceDev, 1, 500); err == nil {
			out["dev"].(map[string]any)["deployments"] = len(dep.Items)
		}
		if pods, err := h.rancher.ListK8s(r.Context(), cid, "pods", p.NamespaceProd, 1, 500); err == nil {
			out["prod"].(map[string]any)["pods"] = len(pods.Items)
		}
		if dep, err := h.rancher.ListK8s(r.Context(), cid, "deployments", p.NamespaceProd, 1, 500); err == nil {
			out["prod"].(map[string]any)["deployments"] = len(dep.Items)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) CreateProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name             string  `json:"name"`
		Slug             string  `json:"slug"`
		Description      string  `json:"description"`
		NamespaceDev     string  `json:"namespace_dev"`
		NamespaceProd    string  `json:"namespace_prod"`
		RegistryProvider string  `json:"registry_provider"`
		Layout           string  `json:"layout"`
		MemberIDs        []int64 `json:"member_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload không hợp lệ"})
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tên project bắt buộc"})
		return
	}
	slug := slugify(body.Slug)
	if slug == "" {
		slug = slugify(name)
	}
	if slug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "slug không hợp lệ"})
		return
	}
	nsDev := strings.TrimSpace(body.NamespaceDev)
	nsProd := strings.TrimSpace(body.NamespaceProd)
	if nsDev == "" {
		nsDev = slug + "-dev"
	}
	if nsProd == "" {
		nsProd = slug + "-prod"
	}
	provider := strings.TrimSpace(body.RegistryProvider)
	if provider == "" {
		provider = registry.GHCR
	}
	if provider != registry.GHCR && provider != registry.Harbor {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "registry_provider phải là ghcr hoặc harbor"})
		return
	}
	if err := h.registry.ValidateProvider(r.Context(), provider); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	actor := auth.MustUser(r.Context())
	prov, err := h.registry.Provision(r.Context(), provider, slug)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	harborName := prov.HarborProject
	warnings := []string{}
	// Kiểu chạy chốt khi gắn GitHub (Deploy / Git), không lúc tạo project.
	layout := deploy.LayoutSingle

	var id int64
	err = h.db.QueryRow(r.Context(), `
		INSERT INTO projects (name, slug, description, namespace_dev, namespace_prod, harbor_project, registry_provider, owner_id, layout)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`,
		name, slug, strings.TrimSpace(body.Description), nsDev, nsProd, harborName, provider, actor.ID, layout,
	).Scan(&id)
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "tên hoặc slug đã tồn tại"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	_, _ = h.db.Exec(r.Context(), `INSERT INTO project_repos (project_id) VALUES ($1)`, id)
	_, _ = h.db.Exec(r.Context(),
		`INSERT INTO project_members (project_id, user_id, role) VALUES ($1, $2, 'owner') ON CONFLICT DO NOTHING`,
		id, actor.ID)

	for _, uid := range body.MemberIDs {
		if uid == actor.ID {
			continue
		}
		_, _ = h.db.Exec(r.Context(),
			`INSERT INTO project_members (project_id, user_id, role) VALUES ($1, $2, 'dev') ON CONFLICT DO NOTHING`,
			id, uid)
	}

	rancherOn, _ := h.plugins.Enabled(r.Context(), plugins.Rancher)
	if rancherOn {
		if h.rancher != nil && h.rancher.Enabled() {
			cid := r.URL.Query().Get("cluster_id")
			for _, ns := range []string{nsDev, nsProd} {
				if err := h.rancher.EnsureNamespace(r.Context(), cid, ns); err != nil {
					warnings = append(warnings, fmt.Sprintf("Namespace %s: %s", ns, err.Error()))
				}
			}
		} else {
			warnings = append(warnings, "Rancher addon đã bật nhưng chưa cấu hình — chạy bootstrap/addons/install-rancher.sh")
		}
	} else {
		warnings = append(warnings, "Rancher addon chưa bật — bỏ qua tạo namespace (bật trong Addons)")
	}

	auditAction(r.Context(), h, r, "project.create", slug, map[string]any{"name": name, "by": actor.Email})

	created := projectRow{
		ID: id, Slug: slug, Name: name,
		NamespaceDev: nsDev, NamespaceProd: nsProd,
	}
	_ = h.ensureAutoDomains(r.Context(), created)
	cid := r.URL.Query().Get("cluster_id")
	if rancherOn && h.rancher != nil && h.rancher.Enabled() {
		domList, _ := h.listProjectDomains(r.Context(), id)
		for i := range domList {
			h.syncProjectDomain(r.Context(), created, &domList[i], cid, nil)
		}
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id": id, "slug": slug, "name": name,
		"layout": layout,
		"namespace_dev": nsDev, "namespace_prod": nsProd,
		"registry_provider": provider,
		"harbor_project": harborName, "warnings": warnings,
	})
}

func (h *Handler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	u, _ := auth.UserFromContext(r.Context())
	if !auth.CanDeleteProject(u.Role) {
		writeAccessDenied(w)
		return
	}
	p, err := h.getProjectBySlug(r.Context(), slug)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project không tồn tại"})
		return
	}

	var body struct {
		PurgeK8s bool `json:"purge_k8s"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	purgeK8s := true
	if r.ContentLength > 0 {
		purgeK8s = body.PurgeK8s
	}
	// Luôn dọn tài nguyên trên VPS — chỉ giữ lại dữ liệu bên thứ 3 (GitHub/GHCR cloud).
	purgeK8s = true

	cid := r.URL.Query().Get("cluster_id")
	warnings := []string{}
	purged := []string{}

	warnings = append(warnings, h.deleteArgoApplications(r.Context(), cid, p.Slug)...)

	domList, _ := h.listProjectDomains(r.Context(), p.ID)
	syncer := h.domainSyncer()
	for _, d := range domList {
		ns := h.projectNamespace(p, d.Environment)
		if err := syncer.DeleteIngress(r.Context(), cid, ns, d.ID); err != nil {
			warnings = append(warnings, fmt.Sprintf("Ingress %s: %s", d.Hostname, err.Error()))
		}
	}

	if purgeK8s && h.rancher != nil && h.rancher.Enabled() {
		for _, ns := range []string{p.NamespaceDev, p.NamespaceProd} {
			if strings.TrimSpace(ns) == "" {
				continue
			}
			if err := h.rancher.PurgeNamespace(r.Context(), cid, ns); err != nil {
				warnings = append(warnings, fmt.Sprintf("Namespace %s: %s", ns, err.Error()))
				continue
			}
			if err := h.rancher.WaitNamespaceDeleted(r.Context(), cid, ns, 90*time.Second); err != nil {
				warnings = append(warnings, err.Error())
			} else {
				purged = append(purged, "k8s:"+ns)
			}
		}
	}

	harborName := strings.TrimSpace(p.HarborProject)
	if harborName == "" {
		harborName = p.Slug
	}
	if strings.EqualFold(strings.TrimSpace(p.RegistryProvider), "harbor") && h.registry != nil {
		if err := h.registry.Deprovision(r.Context(), p.RegistryProvider, p.Slug, harborName); err != nil {
			warnings = append(warnings, fmt.Sprintf("Harbor project %s: %s", harborName, err.Error()))
		} else {
			purged = append(purged, "harbor:"+harborName)
		}
	}

	_, err = h.db.Exec(r.Context(), `DELETE FROM projects WHERE id=$1`, p.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	auditAction(r.Context(), h, r, "project.delete", slug, map[string]any{
		"name": p.Name, "by": u.Email, "purge_k8s": purgeK8s, "purged": purged,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"slug":     slug,
		"purged":   purged,
		"warnings": warnings,
		"note":     "Đã xóa metadata DB, namespace K8s, Harbor và ArgoCD app (nếu có). GitHub Actions / GHCR cloud không xóa.",
	})
}

func (h *Handler) PatchProject(w http.ResponseWriter, r *http.Request) {
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
		Description      string `json:"description"`
		RegistryProvider string `json:"registry_provider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload không hợp lệ"})
		return
	}
	desc := strings.TrimSpace(body.Description)
	provider := strings.TrimSpace(body.RegistryProvider)
	if provider != "" && provider != p.RegistryProvider {
		if provider != registry.GHCR && provider != registry.Harbor {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "registry_provider phải là ghcr hoặc harbor"})
			return
		}
		if err := h.registry.ValidateProvider(r.Context(), provider); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		prov, err := h.registry.Provision(r.Context(), provider, p.Slug)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		harborName := prov.HarborProject
		if provider == registry.GHCR {
			harborName = p.HarborProject
		}
		_, err = h.db.Exec(r.Context(), `
			UPDATE projects SET registry_provider=$1, harbor_project=$2, description=COALESCE(NULLIF($3,''), description), updated_at=now()
			WHERE id=$4`,
			provider, harborName, desc, p.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		auditAction(r.Context(), h, r, "project.registry", slug, map[string]any{"provider": provider, "by": u.Email})
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	_, err := h.db.Exec(r.Context(), `UPDATE projects SET description=$1, updated_at=now() WHERE id=$2`, desc, p.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

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

func (h *Handler) GetProjectRepo(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	repo, err := h.getProjectRepo(r.Context(), p.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	h.enrichRepoWorkflowStatus(r.Context(), p.ID, &repo)
	writeJSON(w, http.StatusOK, repo)
}

func (h *Handler) PatchProjectRepo(w http.ResponseWriter, r *http.Request) {
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
		GitURL         string `json:"git_url"`
		Branch         string `json:"branch"`
		DockerfilePath string `json:"dockerfile_path"`
		BuildContext   string `json:"build_context"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload không hợp lệ"})
		return
	}
	branch := strings.TrimSpace(body.Branch)
	if branch == "" {
		branch = "main"
	}
	df := strings.TrimSpace(body.DockerfilePath)
	if df == "" {
		df = "Dockerfile"
	}
	ctx := strings.TrimSpace(body.BuildContext)
	if ctx == "" {
		ctx = "."
	}
	_, err := h.db.Exec(r.Context(), `
		INSERT INTO project_repos (project_id, git_url, branch, dockerfile_path, build_context, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (project_id) DO UPDATE SET
			git_url = EXCLUDED.git_url,
			branch = EXCLUDED.branch,
			dockerfile_path = EXCLUDED.dockerfile_path,
			build_context = EXCLUDED.build_context,
			updated_at = now()`,
		p.ID, strings.TrimSpace(body.GitURL), branch, df, ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	repo, _ := h.getProjectRepo(r.Context(), p.ID)
	_ = h.resolveBuildMode(r.Context(), u.ID, p.ID, &repo)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

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
