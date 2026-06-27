package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/config"
	"github.com/Thien2026/k8s/services/portal-api/internal/harbor"
	"github.com/Thien2026/k8s/services/portal-api/internal/plugins"
	"github.com/Thien2026/k8s/services/portal-api/internal/rancher"
	"github.com/Thien2026/k8s/services/portal-api/internal/registry"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewRouter(db *pgxpool.Pool, cfg config.Config, rancherClient *rancher.Client) http.Handler {
	pluginStore := plugins.NewStore(db)
	_ = pluginStore.ApplyEnvHints(context.Background(), rancherClient.Enabled(), cfg.HarborURL != "" && cfg.HarborPassword != "")

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, middleware.Recoverer)

	origins := cfg.AllowedOrigins
	if len(origins) == 0 {
		origins = []string{cfg.CORSOrigin}
	}
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   origins,
		AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Join-Gate"},
		AllowCredentials: true,
	}))

	tokens := auth.TokenConfig{
		Secret:     []byte(cfg.JWTSecret),
		AccessTTL:  cfg.JWTAccessTTL,
		RefreshTTL: cfg.JWTRefreshTTL,
		Secure:     cfg.CookieSecure,
	}

	harborClient := harbor.NewClient(cfg.HarborURL, "admin", cfg.HarborPassword)
	h := &Handler{
		db:       db,
		cfg:      cfg,
		rancher:  rancherClient,
		harbor:   harborClient,
		auth:     auth.NewStore(db),
		tokens:   tokens,
		plugins:  pluginStore,
		registry: registry.NewService(pluginStore, harborClient, cfg.GHCROrg),
	}

	r.Get("/health", h.Health)
	r.Get("/api/v1/health/db", h.HealthDB)
	r.Get("/api/v1/github/oauth/callback", h.GitHubOAuthCallback)
	r.With(httprate.LimitByIP(60, time.Minute)).Post("/api/v1/hooks/deploy", h.DeployHook)
	r.With(httprate.LimitByIP(60, time.Minute)).Post("/api/v1/hooks/deploy/validate-config", h.DeployValidateConfigHook)
	r.With(httprate.LimitByIP(60, time.Minute)).Post("/api/v1/hooks/deploy/event", h.DeployEventHook)

	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Get("/quick-login", h.AuthQuickLogin)
		r.With(httprate.LimitByIP(10, time.Minute)).Post("/login", h.AuthLogin)
		r.Post("/refresh", h.AuthRefresh)
		r.Group(func(r chi.Router) {
			r.Use(h.requireMutatingOrigin)
			r.Use(h.requireAuth)
			r.Get("/me", h.AuthMe)
			r.Post("/logout", h.AuthLogout)
		})
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(h.requireMutatingOrigin)
		r.Use(h.requireAuth)

		r.Get("/dashboard", h.Dashboard)
		r.Get("/infra/links", h.GetInfraLinks)
		r.Get("/cluster", h.ClusterSummary)
		r.Get("/projects", h.ListProjects)
		r.Get("/projects/{slug}", h.GetProject)
		r.Get("/projects/{slug}/overview", h.ProjectOverview)
		r.Get("/projects/{slug}/members", h.ListProjectMembers)
		r.Get("/projects/{slug}/repo", h.GetProjectRepo)
		r.Get("/projects/{slug}/deploy/plan", h.GetProjectDeployPlan)
		r.Get("/projects/{slug}/deploy/activity", h.GetProjectDeployActivity)
		r.Post("/projects/{slug}/deploy/apply", h.ApplyProjectDeploy)
		r.Post("/projects/{slug}/deploy/promote", h.PromoteProjectDeploy)
		r.Get("/projects/{slug}/deploy/promote-readiness", h.GetPromoteReadiness)
		r.Post("/projects/{slug}/deploy/rollback", h.RollbackProjectDeploy)
		r.Patch("/projects/{slug}/repo/auto-deploy", h.PatchProjectAutoDeploy)
		r.Post("/projects/{slug}/github/setup", h.ProjectGitHubSetup)
		r.Get("/github/status", h.GitHubStatus)
		r.Get("/github/oauth/start", h.GitHubOAuthStart)
		r.Get("/github/repos", h.GitHubListRepos)
		r.Get("/github/repos/{owner}/{repo}/branches", h.GitHubListBranches)
		r.Delete("/github/disconnect", h.GitHubDisconnect)
		r.Get("/projects/{slug}/domains", h.ListProjectDomains)
		r.Post("/projects/{slug}/domains/{domainId}/sync", h.SyncProjectDomain)
		r.Get("/projects/{slug}/env", h.ListProjectEnvVars)
		r.Get("/projects/{slug}/env/readiness", h.GetProjectEnvReadiness)
		r.Get("/projects/{slug}/env/sync-status", h.GetProjectEnvSyncStatus)
		r.Post("/projects/{slug}/env", h.CreateProjectEnvVar)
		r.Post("/projects/{slug}/env/sync", h.SyncProjectEnvVars)
		r.Post("/projects/{slug}/env/sync-workflow", h.SyncProjectBuildWorkflow)
		r.Patch("/projects/{slug}/env/{envVarId}", h.PatchProjectEnvVar)
		r.Delete("/projects/{slug}/env/{envVarId}", h.DeleteProjectEnvVar)
		r.Patch("/projects/{slug}", h.PatchProject)
		r.Patch("/projects/{slug}/repo", h.PatchProjectRepo)
		r.Post("/projects/{slug}/members", h.AddProjectMember)
		r.Delete("/projects/{slug}/members/{userId}", h.RemoveProjectMember)
		r.Post("/projects/{slug}/domains", h.AddProjectDomain)
		r.Delete("/projects/{slug}/domains/{domainId}", h.DeleteProjectDomain)

		r.Group(func(r chi.Router) {
			r.Use(h.requireRole(auth.RoleAdmin, auth.RoleTechLead))
			r.Post("/projects", h.CreateProject)
			r.Delete("/projects/{slug}", h.DeleteProject)
			r.Get("/team/users", h.TeamUserPicker)
		})
		r.Get("/explorer/menu", h.ExplorerMenu)
		r.Get("/registry/providers", h.ListRegistryProviders)
		r.Get("/rancher/namespaces", h.ListNamespaces)
		r.Get("/rancher/cluster/dashboard", h.ClusterDashboard)
		r.Get("/rancher/clusters", h.ListRancherClusters)
		r.Get("/rancher/projects", h.ListRancherProjects)
		r.Patch("/k8s/deployments/{name}/scale", h.ScaleDeployment)
		r.Get("/k8s/pods/{name}/logs", h.GetPodLogs)
		r.Get("/k8s/{resource}/{name}/yaml", h.GetK8sYAML)
		r.Delete("/k8s/{resource}/{name}", h.DeleteK8sResource)
		r.Get("/k8s/{resource}/{name}", h.GetK8sDetail)
		r.Get("/k8s/{resource}", h.ListK8sResource)

		r.Group(func(r chi.Router) {
			r.Use(h.requireRole(auth.RoleAdmin, auth.RoleTechLead))
			r.Get("/cluster/join-info", h.JoinInfo)
			r.Post("/cluster/join-script", h.JoinScript)
			r.Get("/admin/audit", h.AdminAuditLog)
		})

		r.Group(func(r chi.Router) {
			r.Use(h.requireRole(auth.RoleAdmin, auth.RoleTechLead))
			r.Get("/admin/plugins", h.AdminListPlugins)
		})

		r.Group(func(r chi.Router) {
			r.Use(h.requireRole(auth.RoleAdmin))
			r.Patch("/admin/plugins/{name}", h.AdminPatchPlugin)
			r.Get("/admin/users", h.AdminListUsers)
			r.Post("/admin/users", h.AdminCreateUser)
			r.Patch("/admin/users/{id}", h.AdminUpdateUser)
		})
	})

	return r
}

type Handler struct {
	db       *pgxpool.Pool
	cfg      config.Config
	rancher  *rancher.Client
	harbor   *harbor.Client
	auth     *auth.Store
	tokens   auth.TokenConfig
	plugins  *plugins.Store
	registry *registry.Service
}

func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) HealthDB(w http.ResponseWriter, r *http.Request) {
	if err := h.db.Ping(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "database": "connected"})
}

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	health := map[string]string{"status": "ok"}
	if err := h.db.Ping(r.Context()); err != nil {
		health = map[string]string{"status": "error", "error": err.Error()}
	} else {
		health["database"] = "connected"
	}

	projects, err := h.listProjectsForUser(r, u)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	cluster := map[string]any{"connected": false}
	rancherOn, _ := h.plugins.Enabled(r.Context(), plugins.Rancher)
	if rancherOn && h.rancher != nil && h.rancher.Enabled() {
		if sum, err := h.rancher.ClusterSummary(r.Context()); err != nil {
			cluster = map[string]any{"connected": false, "error": err.Error()}
		} else {
			cluster = map[string]any{
				"connected": sum.Connected,
				"total":     sum.Total,
				"ready":     sum.Ready,
				"not_ready": sum.NotReady,
				"nodes":     sum.Nodes,
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"health":   health,
		"cluster":  cluster,
		"projects": projects,
		"user":     u,
	})
}

func (h *Handler) ListProjects(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	list, err := h.listProjectsForUser(r, u)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) listProjectsForUser(r *http.Request, u auth.User) ([]projectRow, error) {
	var (
		rows interface {
			Next() bool
			Scan(dest ...any) error
			Close()
			Err() error
		}
		err error
	)
	if auth.CanViewAllProjects(u.Role) {
		rows, err = h.db.Query(r.Context(), `
			SELECT id, name, slug, COALESCE(description,''), namespace_dev, namespace_prod,
			       COALESCE(harbor_project,''), COALESCE(registry_provider,'ghcr')
			FROM projects ORDER BY id`)
	} else {
		rows, err = h.db.Query(r.Context(), `
			SELECT p.id, p.name, p.slug, COALESCE(p.description,''), p.namespace_dev, p.namespace_prod,
			       COALESCE(p.harbor_project,''), COALESCE(p.registry_provider,'ghcr')
			FROM projects p
			INNER JOIN project_members pm ON pm.project_id = p.id
			WHERE pm.user_id = $1
			ORDER BY p.id`, u.ID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []projectRow
	for rows.Next() {
		var p projectRow
		if err := rows.Scan(&p.ID, &p.Name, &p.Slug, &p.Description, &p.NamespaceDev, &p.NamespaceProd, &p.HarborProject, &p.RegistryProvider); err != nil {
			return nil, err
		}
		h.enrichProjectRegistry(r.Context(), &p)
		list = append(list, p)
	}
	if list == nil {
		list = []projectRow{}
	}
	return list, rows.Err()
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
