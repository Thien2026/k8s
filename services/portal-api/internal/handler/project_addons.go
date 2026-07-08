package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type addonCatalogItem struct {
	Engine      string `json:"engine"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Available   bool   `json:"available"`
}

type projectAddonView struct {
	Engine        string `json:"engine"`
	Environment   string `json:"environment"`
	Status        string `json:"status"`
	K8sRelease    string `json:"k8s_release,omitempty"`
	MaxMemoryMB   int    `json:"max_memory_mb"`
	MaxClients    int    `json:"max_clients"`
	HasConnection bool   `json:"has_connection"`
	CreatedAt     string `json:"created_at,omitempty"`
	UpdatedAt     string `json:"updated_at,omitempty"`
}

var addonCatalog = []addonCatalogItem{
	{
		Engine:      "redis",
		Label:       "Redis",
		Description: "Cache, session, queue — mỗi project một instance riêng",
		Icon:        "redis",
		Available:   true,
	},
	{
		Engine:      "postgres",
		Label:       "Postgres",
		Description: "Database qua CNPG — Phase 10b",
		Icon:        "postgres",
		Available:   false,
	},
}

func validAddonEngine(engine string) bool {
	for _, item := range addonCatalog {
		if item.Engine == engine && item.Available {
			return true
		}
	}
	return false
}

func validAddonEnv(env string) bool {
	return env == "dev" || env == "prod"
}

func (h *Handler) listProjectAddons(ctx context.Context, projectID int64) ([]projectAddonView, error) {
	rows, err := h.db.Query(ctx, `
		SELECT engine, environment, status, k8s_release, max_memory_mb, max_clients,
		       (connection_secret <> ''), created_at::text, updated_at::text
		FROM project_data_addons
		WHERE project_id = $1
		ORDER BY engine, environment`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]projectAddonView, 0, 4)
	for rows.Next() {
		var v projectAddonView
		if err := rows.Scan(
			&v.Engine, &v.Environment, &v.Status, &v.K8sRelease,
			&v.MaxMemoryMB, &v.MaxClients, &v.HasConnection,
			&v.CreatedAt, &v.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (h *Handler) getProjectAddon(ctx context.Context, projectID int64, engine, env string) (*projectAddonView, error) {
	var v projectAddonView
	err := h.db.QueryRow(ctx, `
		SELECT engine, environment, status, k8s_release, max_memory_mb, max_clients,
		       (connection_secret <> ''), created_at::text, updated_at::text
		FROM project_data_addons
		WHERE project_id = $1 AND engine = $2 AND environment = $3`,
		projectID, engine, env).Scan(
		&v.Engine, &v.Environment, &v.Status, &v.K8sRelease,
		&v.MaxMemoryMB, &v.MaxClients, &v.HasConnection,
		&v.CreatedAt, &v.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// ListProjectAddons GET /projects/{slug}/addons
func (h *Handler) ListProjectAddons(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	items, err := h.listProjectAddons(r.Context(), p.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"catalog":     addonCatalog,
		"items":       items,
		"can_manage":  h.canManageProject(r.Context(), u, p.ID),
		"project":     map[string]string{"slug": p.Slug, "name": p.Name},
	})
}

// GetProjectAddon GET /projects/{slug}/addons/{engine}?environment=dev
func (h *Handler) GetProjectAddon(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	engine := strings.TrimSpace(chi.URLParam(r, "engine"))
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	env := strings.TrimSpace(r.URL.Query().Get("environment"))
	if env == "" {
		env = "dev"
	}
	if !validAddonEnv(env) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment phải là dev hoặc prod"})
		return
	}
	catalogItem, catalogOK := addonByEngine(engine)
	if !catalogOK {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "addon không tồn tại"})
		return
	}

	item, err := h.getProjectAddon(r.Context(), p.ID, engine, env)
	u, _ := auth.UserFromContext(r.Context())
	canManage := h.canManageProject(r.Context(), u, p.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusOK, map[string]any{
				"catalog":     catalogItem,
				"installed":   false,
				"can_manage":  canManage,
				"environment": env,
				"namespace":   h.projectNamespace(p, env),
			})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"catalog":     catalogItem,
		"installed":   true,
		"addon":       item,
		"can_manage":  canManage,
		"environment": env,
		"namespace":   h.projectNamespace(p, env),
	})
}

func addonByEngine(engine string) (addonCatalogItem, bool) {
	for _, item := range addonCatalog {
		if item.Engine == engine {
			return item, true
		}
	}
	return addonCatalogItem{}, false
}

// CreateProjectAddon POST /projects/{slug}/addons/{engine}
func (h *Handler) CreateProjectAddon(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	engine := strings.TrimSpace(chi.URLParam(r, "engine"))
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	if !h.canManageProject(r.Context(), u, p.ID) {
		writeAccessDenied(w)
		return
	}
	if !validAddonEngine(engine) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "addon chưa hỗ trợ hoặc chưa bật"})
		return
	}

	var body struct {
		Environment string `json:"environment"`
		MaxMemoryMB int    `json:"max_memory_mb"`
		MaxClients  int    `json:"max_clients"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	env := strings.TrimSpace(body.Environment)
	if env == "" {
		env = "dev"
	}
	if !validAddonEnv(env) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment phải là dev hoặc prod"})
		return
	}
	if env == "prod" && !auth.CanWriteProd(u.Role) {
		writeAccessDenied(w)
		return
	}
	maxMem := body.MaxMemoryMB
	if maxMem < 64 || maxMem > 512 {
		maxMem = 128
	}
	maxClients := body.MaxClients
	if maxClients < 10 || maxClients > 1000 {
		maxClients = 100
	}

	ns := h.projectNamespace(p, env)
	if ns == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "namespace chưa cấu hình"})
		return
	}

	release := p.Slug + "-" + engine + "-" + env
	_, err := h.db.Exec(r.Context(), `
		INSERT INTO project_data_addons (project_id, engine, environment, status, k8s_release, max_memory_mb, max_clients)
		VALUES ($1, $2, $3, 'pending', $4, $5, $6)
		ON CONFLICT (project_id, engine, environment) DO UPDATE SET
			status = CASE WHEN project_data_addons.status IN ('stopped', 'failed') THEN 'pending' ELSE project_data_addons.status END,
			max_memory_mb = EXCLUDED.max_memory_mb,
			max_clients = EXCLUDED.max_clients,
			updated_at = now()`,
		p.ID, engine, env, release, maxMem, maxClients)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	auditAction(r.Context(), h, r, "addon.create", slug, map[string]any{
		"engine": engine, "environment": env, "by": u.Email,
	})

	// Phase 10a: provision K8s sẽ gọi ở bước tiếp theo.
	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":  "pending",
		"message": "Đã ghi nhận addon — provision Redis trên cluster sẽ có ở bước kế tiếp",
		"engine":  engine,
		"environment": env,
		"k8s_release": release,
	})
}
