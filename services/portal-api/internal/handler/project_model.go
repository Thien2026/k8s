package handler

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/domains"
	"github.com/Thien2026/k8s/services/portal-api/internal/registry"
)

type projectRow struct {
	ID               int64                    `json:"id"`
	Name             string                   `json:"name"`
	Slug             string                   `json:"slug"`
	Layout           string                   `json:"layout"`
	Description      string                   `json:"description,omitempty"`
	NamespaceDev     string                   `json:"namespace_dev"`
	NamespaceProd    string                   `json:"namespace_prod"`
	HarborProject    string                   `json:"harbor_project,omitempty"`
	RegistryProvider string                   `json:"registry_provider"`
	Registry         registry.ProjectRegistry `json:"registry,omitempty"`
	CreatedAt        string                   `json:"created_at,omitempty"`
}

type projectMemberRow struct {
	UserID      int64  `json:"user_id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name,omitempty"`
	Role        string `json:"role"`
}

type projectRepoRow struct {
	GitURL                string `json:"git_url"`
	Branch                string `json:"branch"`
	BuildMode             string `json:"build_mode"`
	BuildModeDetectedPath string `json:"build_mode_detected_path,omitempty"`
	DockerfilePath        string `json:"dockerfile_path"`
	BuildContext          string `json:"build_context"`
	GitHubOwner           string `json:"github_owner,omitempty"`
	GitHubRepo            string `json:"github_repo,omitempty"`
	DeployEnvironment     string `json:"deploy_environment,omitempty"`
	WorkflowSyncedAt      string `json:"workflow_synced_at,omitempty"`
	WorkflowSyncLayout    string `json:"workflow_sync_layout,omitempty"`
	WorkflowSyncServices  string `json:"workflow_sync_services,omitempty"`
	WorkflowStale         bool   `json:"workflow_stale,omitempty"`
	WorkflowStaleReason   string `json:"workflow_stale_reason,omitempty"`
	AutoDeployEnabled     bool   `json:"auto_deploy_enabled"`
	GitSubmodules         string `json:"git_submodules,omitempty"`
	EnvContractBuild      string `json:"-"`
	EnvContractRuntime    string `json:"-"`
	UpdatedAt             string `json:"updated_at,omitempty"`
}

type projectDomainRow struct {
	ID          int64           `json:"id"`
	Hostname    string          `json:"hostname"`
	Environment string          `json:"environment"`
	TLSEnabled  bool            `json:"tls_enabled"`
	Kind        string          `json:"kind"`
	IngressName string          `json:"ingress_name,omitempty"`
	SyncStatus  string          `json:"sync_status"`
	SyncError   string          `json:"sync_error,omitempty"`
	CertStatus  string          `json:"cert_status"`
	URL         string          `json:"url,omitempty"`
	DNS         domains.DNSHint `json:"dns,omitempty"`
	CreatedAt   string          `json:"created_at"`
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
