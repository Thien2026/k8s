package handler

import (
	"encoding/json"
	"net/http"

	"github.com/Thien2026/k8s/services/portal-api/internal/config"
	"github.com/Thien2026/k8s/services/portal-api/internal/rancher"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewRouter(db *pgxpool.Pool, cfg config.Config, rancherClient *rancher.Client) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, middleware.Recoverer)

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{cfg.CORSOrigin},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
	}))

	h := &Handler{db: db, rancher: rancherClient}

	r.Get("/health", h.Health)
	r.Get("/api/v1/health/db", h.HealthDB)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/dashboard", h.Dashboard)
		r.Get("/cluster", h.ClusterSummary)
		r.Get("/projects", h.ListProjects)
		r.Get("/explorer/menu", h.ExplorerMenu)
		r.Get("/rancher/clusters", h.ListRancherClusters)
		r.Get("/rancher/projects", h.ListRancherProjects)
		r.Get("/k8s/{resource}", h.ListK8sResource)
	})

	return r
}

type Handler struct {
	db      *pgxpool.Pool
	rancher *rancher.Client
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
	health := map[string]string{"status": "ok"}
	if err := h.db.Ping(r.Context()); err != nil {
		health = map[string]string{"status": "error", "error": err.Error()}
	} else {
		health["database"] = "connected"
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT id, name, namespace_dev, namespace_prod FROM projects ORDER BY id`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	type project struct {
		ID            int64  `json:"id"`
		Name          string `json:"name"`
		NamespaceDev  string `json:"namespace_dev"`
		NamespaceProd string `json:"namespace_prod"`
	}
	var projects []project
	for rows.Next() {
		var p project
		if err := rows.Scan(&p.ID, &p.Name, &p.NamespaceDev, &p.NamespaceProd); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		projects = append(projects, p)
	}
	if projects == nil {
		projects = []project{}
	}

	cluster := map[string]any{"connected": false}
	if h.rancher != nil && h.rancher.Enabled() {
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
	})
}

func (h *Handler) ListProjects(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(r.Context(),
		`SELECT id, name, namespace_dev, namespace_prod FROM projects ORDER BY id`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	type project struct {
		ID            int64  `json:"id"`
		Name          string `json:"name"`
		NamespaceDev  string `json:"namespace_dev"`
		NamespaceProd string `json:"namespace_prod"`
	}

	var list []project
	for rows.Next() {
		var p project
		if err := rows.Scan(&p.ID, &p.Name, &p.NamespaceDev, &p.NamespaceProd); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		list = append(list, p)
	}
	if list == nil {
		list = []project{}
	}
	writeJSON(w, http.StatusOK, list)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
