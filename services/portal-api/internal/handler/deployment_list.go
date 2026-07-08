package handler

import (
	"context"
	"strings"
	"time"

	gh "github.com/Thien2026/k8s/services/portal-api/internal/github"
)

const deployHistoryLimit = 50

func (h *Handler) listProjectDeployments(ctx context.Context, projectID int64, env string, limit int) ([]deploymentRow, error) {
	if limit < 1 {
		limit = deployHistoryLimit
	}
	env = strings.ToLower(strings.TrimSpace(env))
	if env == "" {
		env = "dev"
	}
	rows, err := h.db.Query(ctx, `
		SELECT id, environment, image_tag, status,
			build_status, registry_status, deploy_status, runtime_status,
			COALESCE(error_phase,''), COALESCE(error_message,''),
			github_run_id, COALESCE(github_run_url,''), COALESCE(image,''), COALESCE(runtime_detail,''),
			COALESCE(deploy_layout,''), COALESCE(git_branch,''),
			COALESCE(deploy_services,'[]'::jsonb), COALESCE(deploy_images,'{}'::jsonb),
			created_at, updated_at, finished_at
		FROM project_deployments
		WHERE project_id=$1 AND environment=$2
		ORDER BY created_at DESC
		LIMIT $3`, projectID, env, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []deploymentRow
	for rows.Next() {
		var d deploymentRow
		var finished *time.Time
		var created, updated time.Time
		var svcRaw, imgRaw []byte
		var deployLayout, gitBranch string
		if err := rows.Scan(
			&d.ID, &d.Environment, &d.ImageTag, &d.Status,
			&d.BuildStatus, &d.RegistryStatus, &d.DeployStatus, &d.RuntimeStatus,
			&d.ErrorPhase, &d.ErrorMessage,
			&d.GitHubRunID, &d.GitHubRunURL, &d.Image, &d.RuntimeDetail,
			&deployLayout, &gitBranch, &svcRaw, &imgRaw,
			&created, &updated, &finished,
		); err != nil {
			return nil, err
		}
		d.applySnapshotFields(deployLayout, gitBranch, svcRaw, imgRaw)
		d.CreatedAt = created.UTC().Format(time.RFC3339)
		d.UpdatedAt = updated.UTC().Format(time.RFC3339)
		if finished != nil {
			d.FinishedAt = finished.UTC().Format(time.RFC3339)
		}
		normalizeStaleDeploymentRow(&d)
		out = append(out, d)
	}
	return out, rows.Err()
}

func workflowFileForProject(slug string) string {
	return "platform-deploy-" + slug + ".yml"
}

func (h *Handler) fetchGitHubWorkflowRuns(ctx context.Context, userID int64, owner, repo, slug string, limit int) ([]gh.WorkflowRun, error) {
	ghToken, _, err := h.getGitHubToken(ctx, userID)
	if err != nil || ghToken == "" {
		return nil, err
	}
	client := h.githubClient()
	runs, err := client.ListWorkflowRuns(ctx, ghToken, owner, repo, workflowFileForProject(slug), limit)
	if err == nil && len(runs) > 0 {
		return runs, nil
	}
	return client.ListRepoWorkflowRuns(ctx, ghToken, owner, repo, limit)
}

func (h *Handler) attachGitHubRunForDeployment(ctx context.Context, userID int64, p projectRow, repo projectRepoRow, d *deploymentRow) {
	if d == nil || d.GitHubRunID > 0 {
		return
	}
	owner := strings.TrimSpace(repo.GitHubOwner)
	ghRepo := strings.TrimSpace(repo.GitHubRepo)
	if owner == "" || ghRepo == "" {
		return
	}
	runs, err := h.fetchGitHubWorkflowRuns(ctx, userID, owner, ghRepo, p.Slug, 5)
	if err != nil || len(runs) == 0 {
		return
	}
	tag := strings.TrimSpace(d.ImageTag)
	for _, run := range runs {
		if tag != "" && !strings.EqualFold(strings.TrimSpace(run.HeadSHA), tag) {
			continue
		}
		d.GitHubRunID = run.ID
		d.GitHubRunURL = run.HTMLURL
		if d.ImageTag == "" {
			d.ImageTag = run.HeadSHA
		}
		return
	}
	// Chưa có image_tag — gắn run mới nhất đang chạy hoặc vừa tạo.
	run := runs[0]
	if strings.ToLower(run.Status) != "completed" || d.Status == "in_progress" {
		d.GitHubRunID = run.ID
		d.GitHubRunURL = run.HTMLURL
		if d.ImageTag == "" {
			d.ImageTag = run.HeadSHA
		}
	}
}

func (h *Handler) deploymentRowFromRun(ctx context.Context, p projectRow, deployEnv string, run gh.WorkflowRun, withRuntime bool) deploymentRow {
	bs := mapGitHubRunStatus(run.Status, run.Conclusion)
	h.enrichProjectRegistry(ctx, &p)
	repo, _ := h.getProjectRepo(ctx, p.ID)
	params := h.buildDeployParams(ctx, p, repo, deployEnv, run.HeadSHA, false)
	image := deployImageRef(params)
	d := deploymentRow{
		Environment:  deployEnv,
		ImageTag:     run.HeadSHA,
		GitHubRunID:  run.ID,
		GitHubRunURL: run.HTMLURL,
		BuildStatus:  bs,
		Image:        image,
		CreatedAt:    run.CreatedAt,
		UpdatedAt:    run.UpdatedAt,
	}
	if bs == "success" {
		d.RegistryStatus = "success"
		runDone := strings.EqualFold(strings.TrimSpace(run.Status), "completed")
		if !withRuntime {
			if !runDone {
				d.Status = "in_progress"
				d.DeployStatus = "pending"
				d.RuntimeStatus = "pending"
				return d
			}
			d.Status = "success"
			d.DeployStatus = "pending"
			d.RuntimeStatus = "pending"
			return d
		}
		rs, detail, podName, errMsg := h.runtimeStatusForDeploy(ctx, p, deployEnv, run.HeadSHA)
		d.RuntimeDetail = detail
		d.PodName = podName
		if errMsg != "" {
			d.Status = "failed"
			d.DeployStatus = "success"
			d.RuntimeStatus = "failed"
			d.ErrorPhase = "runtime"
			d.ErrorMessage = errMsg
		} else if rs == "success" {
			d.Status = "success"
			d.DeployStatus = "success"
			d.RuntimeStatus = "success"
		} else {
			d.Status = "in_progress"
			d.DeployStatus = "success"
			d.RuntimeStatus = rs
			if rs == "pending" {
				d.DeployStatus = "running"
			}
		}
	} else if bs == "failed" {
		d.Status = "failed"
		d.ErrorPhase = "build"
		if run.Conclusion != "" {
			d.ErrorMessage = "GitHub Actions: " + run.Conclusion
		} else {
			d.ErrorMessage = "Build thất bại trên GitHub Actions"
		}
	} else {
		d.Status = "in_progress"
	}
	return d
}
