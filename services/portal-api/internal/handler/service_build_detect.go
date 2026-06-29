package handler

import (
	"context"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
)

// enrichProjectServiceBuildModes quét GitHub — build_mode + stack từng service (L4B polyglot).
func (h *Handler) enrichProjectServiceBuildModes(ctx context.Context, userID int64, projectID int64, repo projectRepoRow, branch string) error {
	owner := strings.TrimSpace(repo.GitHubOwner)
	ghRepo := strings.TrimSpace(repo.GitHubRepo)
	if owner == "" || ghRepo == "" {
		return nil
	}
	token, _, err := h.getGitHubToken(ctx, userID)
	if err != nil || token == "" {
		return nil
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = strings.TrimSpace(repo.Branch)
	}
	if branch == "" {
		branch = "main"
	}
	rows, err := h.listProjectServices(ctx, projectID)
	if err != nil || len(rows) == 0 {
		return err
	}
	client := h.githubClient()
	exists := func(ctx context.Context, repoPath string) (bool, error) {
		_, found, err := client.GetFileContent(ctx, token, owner, ghRepo, repoPath, branch)
		return found, err
	}
	for _, s := range rows {
		mode, df, stack, err := deploy.DetectServiceBuild(ctx, exists, s.BuildContext, s.DockerfilePath, s.Stack)
		if err != nil {
			return err
		}
		if m := strings.TrimSpace(s.BuildMode); m == "buildpack" || m == "dockerfile" {
			mode = deploy.NormalizeBuildMode(m)
		}
		if sh := deploy.NormalizeStack(s.Stack); sh != "" {
			stack = sh
		}
		if df == "" {
			df = s.DockerfilePath
		}
		_, err = h.db.Exec(ctx, `
			UPDATE project_services
			SET build_mode=$1, dockerfile_path=$2, stack=$3, updated_at=now()
			WHERE project_id=$4 AND name=$5`,
			mode, df, stack, projectID, s.Name)
		if err != nil {
			return err
		}
	}
	return nil
}
