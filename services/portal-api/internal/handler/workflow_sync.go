package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
	"github.com/Thien2026/k8s/services/portal-api/internal/platformcontract"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) loadBuildArgs(ctx context.Context, projectID int64, env, projectSlug string) ([]deploy.BuildArg, error) {
	rows, err := h.db.Query(ctx, `
		SELECT key, value, is_secret FROM project_env_vars
		WHERE project_id=$1 AND environment=$2 AND scope='build' ORDER BY key`,
		projectID, env)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []deploy.BuildArg
	for rows.Next() {
		var key, val string
		var isSecret bool
		if err := rows.Scan(&key, &val, &isSecret); err != nil {
			return nil, err
		}
		key = strings.TrimSpace(key)
		if key == "" || strings.TrimSpace(val) == "" {
			continue
		}
		arg := deploy.BuildArg{Key: key, Value: val, IsSecret: isSecret}
		if isSecret {
			arg.SecretName = deploy.BuildArgSecretName(projectSlug, key)
		}
		out = append(out, arg)
	}
	return out, rows.Err()
}

func (h *Handler) syncProjectGitHubWorkflow(ctx context.Context, u auth.User, p projectRow) error {
	ghToken, _, err := h.getGitHubToken(ctx, u.ID)
	if err != nil || ghToken == "" {
		return fmt.Errorf("chưa kết nối GitHub — kết nối tại tab Deploy / Git")
	}
	repo, err := h.getProjectRepo(ctx, p.ID)
	if err != nil || strings.TrimSpace(repo.GitHubOwner) == "" {
		return fmt.Errorf("chưa cấu hình repo GitHub")
	}
	if err := h.resolveBuildMode(ctx, u.ID, p.ID, &repo); err != nil {
		return fmt.Errorf("không quét được repo: %w", err)
	}
	env := strings.ToLower(strings.TrimSpace(repo.DeployEnvironment))
	if env == "" {
		env = "dev"
	}
	contracts := h.loadEnvContracts(ctx, u.ID, repo)
	if contracts.Build == nil {
		contracts.Build = &platformcontract.File{Version: 1, Vars: map[string]platformcontract.VarSpec{}}
	}
	if contracts.Runtime == nil {
		contracts.Runtime = &platformcontract.File{Version: 1, Vars: map[string]platformcontract.VarSpec{}}
	}
	_ = h.persistEnvContracts(ctx, p.ID, contracts)
	if err := h.requireBuildConfigReady(ctx, u.ID, p, env); err != nil {
		return err
	}
	h.enrichProjectRegistry(ctx, &p)
	params := h.buildDeployParams(ctx, p, repo, env, "", true)
	wf := deploy.GitHubWorkflow(params)
	client := h.githubClient()
	if err := client.PutWorkflowFile(ctx, ghToken, repo.GitHubOwner, repo.GitHubRepo, wf.Filename,
		"chore(platform): sync build config for "+p.Slug, wf.Content, repo.Branch); err != nil {
		return fmt.Errorf("không cập nhật workflow: %w", err)
	}
	branch := strings.TrimSpace(repo.Branch)
	if branch == "" {
		branch = "main"
	}
	if err := client.DispatchWorkflow(ctx, ghToken, repo.GitHubOwner, repo.GitHubRepo, wf.Filename, branch); err != nil {
		return fmt.Errorf("không chạy workflow: %w", err)
	}
	buildVars, err := h.listProjectEnvVars(ctx, p.ID, env, "build")
	if err != nil {
		return err
	}
	for _, v := range buildVars {
		if !v.IsSecret {
			continue
		}
		var plain string
		if err := h.db.QueryRow(ctx, `
			SELECT value FROM project_env_vars WHERE id=$1`, v.ID).Scan(&plain); err != nil || plain == "" {
			continue
		}
		secretName := deploy.BuildArgSecretName(p.Slug, v.Key)
		if err := client.SetActionsSecret(ctx, ghToken, repo.GitHubOwner, repo.GitHubRepo, secretName, plain); err != nil {
			return fmt.Errorf("không cập nhật secret %s: %w", secretName, err)
		}
	}
	return nil
}

func (h *Handler) SyncProjectBuildWorkflow(w http.ResponseWriter, r *http.Request) {
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
	if err := h.syncProjectGitHubWorkflow(r.Context(), u, p); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
