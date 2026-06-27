package handler

import (
	"context"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
)

// resolveBuildMode quét repo GitHub: có Dockerfile → dockerfile, không → buildpack.
func (h *Handler) resolveBuildMode(ctx context.Context, userID, projectID int64, repo *projectRepoRow) error {
	owner := strings.TrimSpace(repo.GitHubOwner)
	ghRepo := strings.TrimSpace(repo.GitHubRepo)
	if owner == "" || ghRepo == "" {
		return nil
	}
	ghToken, _, err := h.getGitHubToken(ctx, userID)
	if err != nil || ghToken == "" {
		return nil
	}
	branch := strings.TrimSpace(repo.Branch)
	if branch == "" {
		branch = "main"
	}
	client := h.githubClient()
	mode, detectedPath, err := deploy.DetectBuildMode(ctx, func(ctx context.Context, path string) (bool, error) {
		_, found, err := client.GetFileContent(ctx, ghToken, owner, ghRepo, path, branch)
		return found, err
	}, repo.DockerfilePath)
	if err != nil {
		return err
	}
	repo.BuildMode = mode
	repo.BuildModeDetectedPath = detectedPath
	if _, err := h.db.Exec(ctx, `
		UPDATE project_repos SET build_mode=$1, updated_at=now() WHERE project_id=$2`,
		mode, projectID); err != nil {
		return err
	}
	return nil
}
