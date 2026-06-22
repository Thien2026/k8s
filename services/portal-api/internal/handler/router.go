package handler

import (
	"encoding/json"
	"net/http"

	"github.com/Thien2026/k8s/services/portal-api/internal/config"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewRouter(db *pgxpool.Pool, cfg config.Config) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, middleware.Recoverer)

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{cfg.CORSOrigin},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
	}))

	h := &Handler{db: db}

	r.Get("/health", h.Health)
	r.Get("/api/v1/health/db", h.HealthDB)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/projects", h.ListProjects)
	})

	return r
}

type Handler struct {
	db *pgxpool.Pool
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
