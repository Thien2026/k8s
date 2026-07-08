package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/go-chi/chi/v5"
)

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
