package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/domains"
	"github.com/Thien2026/k8s/services/portal-api/internal/gitops"
	"github.com/go-chi/chi/v5"
)

type gitOpsConfig struct {
	RepoURL    string `json:"repo_url"`
	RepoBranch string `json:"repo_branch"`
	BasePath   string `json:"base_path"`
	PushToken  string `json:"-"`
	Configured bool   `json:"configured"`
	FromDB     bool   `json:"from_db,omitempty"`
}

func (h *Handler) loadGitOpsConfig(ctx context.Context) gitOpsConfig {
	cfg := gitOpsConfig{
		RepoURL:    strings.TrimSpace(h.cfg.GitOpsRepoURL),
		RepoBranch: strings.TrimSpace(h.cfg.GitOpsRepoBranch),
		BasePath:   strings.TrimSpace(h.cfg.GitOpsBasePath),
		PushToken:  strings.TrimSpace(h.cfg.GitOpsPushToken),
	}
	if cfg.RepoBranch == "" {
		cfg.RepoBranch = "main"
	}
	if cfg.BasePath == "" {
		cfg.BasePath = "apps"
	}
	var dbURL, dbBranch, dbBase, dbToken string
	err := h.db.QueryRow(ctx, `
		SELECT repo_url, repo_branch, base_path, push_token
		FROM platform_gitops_settings WHERE id=1`).Scan(&dbURL, &dbBranch, &dbBase, &dbToken)
	if err == nil {
		cfg.FromDB = true
		if strings.TrimSpace(dbURL) != "" {
			cfg.RepoURL = strings.TrimSpace(dbURL)
		}
		if strings.TrimSpace(dbBranch) != "" {
			cfg.RepoBranch = strings.TrimSpace(dbBranch)
		}
		if strings.TrimSpace(dbBase) != "" {
			cfg.BasePath = strings.TrimSpace(dbBase)
		}
		if strings.TrimSpace(dbToken) != "" {
			cfg.PushToken = strings.TrimSpace(dbToken)
		}
	}
	cfg.Configured = cfg.RepoURL != "" && cfg.PushToken != ""
	return cfg
}

func (h *Handler) gitOpsPublicPayload(ctx context.Context) map[string]any {
	g := h.loadGitOpsConfig(ctx)
	argoOn := h.argoEnabledCtx(ctx)
	out := map[string]any{
		"enabled":           g.RepoURL != "",
		"configured":        g.Configured,
		"repo_url":          g.RepoURL,
		"repo_branch":       g.RepoBranch,
		"base_path":         g.BasePath,
		"token_configured":  g.PushToken != "",
		"argocd_enabled":    argoOn,
		"argocd_url":        strings.TrimRight(strings.TrimSpace(h.cfg.ArgoCDURL), "/"),
		"argocd_namespace":  strings.TrimSpace(h.cfg.ArgoCDNamespace),
	}
	return out
}

func (h *Handler) GetGitOpsPublic(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.gitOpsPublicPayload(r.Context()))
}

func (h *Handler) GetAdminGitOps(w http.ResponseWriter, r *http.Request) {
	if u, _ := auth.UserFromContext(r.Context()); u.Role != auth.RoleAdmin {
		writeAccessDenied(w)
		return
	}
	g := h.loadGitOpsConfig(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"repo_url":         g.RepoURL,
		"repo_branch":      g.RepoBranch,
		"base_path":        g.BasePath,
		"token_configured": g.PushToken != "",
		"configured":       g.Configured,
		"from_db":          g.FromDB,
		"argocd_url":       strings.TrimRight(strings.TrimSpace(h.cfg.ArgoCDURL), "/"),
	})
}

func (h *Handler) PatchAdminGitOps(w http.ResponseWriter, r *http.Request) {
	if u, _ := auth.UserFromContext(r.Context()); u.Role != auth.RoleAdmin {
		writeAccessDenied(w)
		return
	}
	var body struct {
		RepoURL    *string `json:"repo_url"`
		RepoBranch *string `json:"repo_branch"`
		BasePath   *string `json:"base_path"`
		PushToken  *string `json:"push_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload không hợp lệ"})
		return
	}
	cur := h.loadGitOpsConfig(r.Context())
	if body.RepoURL != nil {
		cur.RepoURL = strings.TrimSpace(*body.RepoURL)
	}
	if body.RepoBranch != nil {
		cur.RepoBranch = strings.TrimSpace(*body.RepoBranch)
		if cur.RepoBranch == "" {
			cur.RepoBranch = "main"
		}
	}
	if body.BasePath != nil {
		cur.BasePath = strings.Trim(strings.TrimSpace(*body.BasePath), "/")
		if cur.BasePath == "" {
			cur.BasePath = "apps"
		}
	}
	if body.PushToken != nil {
		tok := strings.TrimSpace(*body.PushToken)
		if tok != "" {
			cur.PushToken = tok
		}
	}
	if cur.RepoURL != "" {
		if _, err := gitops.ParseRepoURL(cur.RepoURL); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}
	_, err := h.db.Exec(r.Context(), `
		INSERT INTO platform_gitops_settings (id, repo_url, repo_branch, base_path, push_token, updated_at)
		VALUES (1, $1, $2, $3, $4, now())
		ON CONFLICT (id) DO UPDATE SET
			repo_url=EXCLUDED.repo_url,
			repo_branch=EXCLUDED.repo_branch,
			base_path=EXCLUDED.base_path,
			push_token=CASE WHEN EXCLUDED.push_token <> '' THEN EXCLUDED.push_token ELSE platform_gitops_settings.push_token END,
			updated_at=now()`,
		cur.RepoURL, cur.RepoBranch, cur.BasePath, cur.PushToken)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":           "ok",
		"configured":       cur.RepoURL != "" && cur.PushToken != "",
		"token_configured": cur.PushToken != "",
	})
}

func (h *Handler) PostAdminGitOpsTest(w http.ResponseWriter, r *http.Request) {
	if u, _ := auth.UserFromContext(r.Context()); u.Role != auth.RoleAdmin {
		writeAccessDenied(w)
		return
	}
	g := h.loadGitOpsConfig(r.Context())
	if g.RepoURL == "" || g.PushToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Chưa cấu hình repo URL và PAT"})
		return
	}
	ref, err := gitops.ParseRepoURL(g.RepoURL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	client := gitops.NewClient()
	if err := client.RepoReachable(r.Context(), g.PushToken, ref); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"owner":   ref.Owner,
		"repo":    ref.Name,
		"branch":  g.RepoBranch,
		"message": "Kết nối repo GitOps OK",
	})
}

func (h *Handler) GetProjectGitOpsStatus(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	g := h.loadGitOpsConfig(r.Context())
	resp := map[string]any{
		"platform_configured": g.Configured,
		"repo_url":            g.RepoURL,
		"argocd_enabled":      h.argoEnabledCtx(r.Context()),
		"dev_scaffolded":      false,
		"prod_scaffolded":     false,
	}
	if !g.Configured {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	ref, err := gitops.ParseRepoURL(g.RepoURL)
	if err != nil {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	client := gitops.NewClient()
	devPath := gitops.OverlayPath(g.BasePath, p.Slug, "dev")
	prodPath := gitops.OverlayPath(g.BasePath, p.Slug, "prod")
	if ok, _ := client.FileExists(r.Context(), g.PushToken, ref, devPath, g.RepoBranch); ok {
		resp["dev_scaffolded"] = true
	}
	if ok, _ := client.FileExists(r.Context(), g.PushToken, ref, prodPath, g.RepoBranch); ok {
		resp["prod_scaffolded"] = true
	}
	if h.argoEnabledCtx(r.Context()) {
		for _, env := range []string{"dev", "prod"} {
			appName := h.argoAppName(p.Slug, env)
			if st, err := h.argoApplicationStatus(r.Context(), appName); err == nil {
				resp["argocd_"+env] = map[string]string{
					"app":    appName,
					"sync":   st.SyncStatus,
					"health": st.HealthStatus,
					"url":    h.argoDashboardURL(appName),
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) PostProjectGitOpsScaffold(w http.ResponseWriter, r *http.Request) {
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
	g := h.loadGitOpsConfig(r.Context())
	if !g.Configured {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
			"error": "Platform chưa cấu hình GitOps — Admin → GitOps",
		})
		return
	}
	ref, err := gitops.ParseRepoURL(g.RepoURL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	h.enrichProjectRegistry(r.Context(), &p)
	repo, _ := h.getProjectRepo(r.Context(), p.ID)
	devParams := h.buildDeployParams(r.Context(), p, repo, "dev", "latest", false)
	prodParams := h.buildDeployParams(r.Context(), p, repo, "prod", "latest", false)
	plat := domains.Platform{Domain: h.cfg.PlatformDomain}
	devHost := h.primaryDomainHost(r.Context(), p.ID, "dev")
	if devHost == "" {
		devHost = plat.AutoHostname(p.Slug, "dev")
	}
	prodHost := h.primaryDomainHost(r.Context(), p.ID, "prod")
	if prodHost == "" {
		prodHost = plat.AutoHostname(p.Slug, "prod")
	}
	files, err := gitops.BuildFiles(gitops.ScaffoldInput{
		Slug:         p.Slug,
		BasePath:     g.BasePath,
		PlatformHost: h.cfg.PlatformDomain,
		DevParams:    devParams,
		ProdParams:   prodParams,
		DevRoutes:    h.ingressRoutesForProject(r.Context(), p.ID, repo),
		ProdRoutes:   h.ingressRoutesForProject(r.Context(), p.ID, repo),
		DevHost:      devHost,
		ProdHost:     prodHost,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	client := gitops.NewClient()
	pushed := []string{}
	for path, content := range files {
		msg := fmt.Sprintf("chore(gitops): scaffold %s (%s)", p.Slug, path)
		if err := client.PutFile(r.Context(), g.PushToken, ref, path, g.RepoBranch, msg, content); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{
				"error": "push " + path + ": " + err.Error(),
			})
			return
		}
		pushed = append(pushed, path)
	}
	if h.argoEnabledCtx(r.Context()) {
		for _, env := range []string{"dev", "prod"} {
			_, _, _ = h.ensureArgoApplication(r.Context(), p, env, "latest")
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "ok",
		"files":        pushed,
		"repo_url":     g.RepoURL,
		"branch":       g.RepoBranch,
		"overlay_dev":  gitops.OverlayPath(g.BasePath, p.Slug, "dev"),
		"overlay_prod": gitops.OverlayPath(g.BasePath, p.Slug, "prod"),
	})
}
