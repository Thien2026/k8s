package handler

import (
	"context"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
)

func (h *Handler) previewBuildMode(ctx context.Context, userID int64, owner, ghRepo, branch, dockerfilePath string) (mode, detectedPath string, err error) {
	owner = strings.TrimSpace(owner)
	ghRepo = strings.TrimSpace(ghRepo)
	if owner == "" || ghRepo == "" {
		return "", "", nil
	}
	ghToken, _, err := h.getGitHubToken(ctx, userID)
	if err != nil || ghToken == "" {
		return "", "", nil
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "main"
	}
	client := h.githubClient()
	return deploy.DetectBuildMode(ctx, func(ctx context.Context, path string) (bool, error) {
		_, found, err := client.GetFileContent(ctx, ghToken, owner, ghRepo, path, branch)
		return found, err
	}, dockerfilePath)
}

// resolveBuildMode quét repo GitHub: có Dockerfile → dockerfile, không → buildpack.
func (h *Handler) resolveBuildMode(ctx context.Context, userID, projectID int64, repo *projectRepoRow) error {
	mode, detectedPath, err := h.previewBuildMode(ctx, userID, repo.GitHubOwner, repo.GitHubRepo, repo.Branch, repo.DockerfilePath)
	if err != nil {
		return err
	}
	if mode == "" {
		return nil
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
