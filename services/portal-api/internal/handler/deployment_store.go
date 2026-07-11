package handler

import (
	"context"
	"strings"
)

func (h *Handler) upsertDeployment(ctx context.Context, projectID int64, env, imageTag string) (int64, error) {
	env = strings.ToLower(strings.TrimSpace(env))
	if env == "" {
		env = "dev"
	}
	tag := strings.TrimSpace(imageTag)
	var id int64
	err := h.db.QueryRow(ctx, `
		SELECT id FROM project_deployments
		WHERE project_id=$1 AND environment=$2 AND image_tag=$3
		ORDER BY id DESC LIMIT 1`, projectID, env, tag).Scan(&id)
	if err == nil {
		_, _ = h.db.Exec(ctx, `
			UPDATE project_deployments SET build_status='running', status='in_progress', updated_at=now()
			WHERE id=$1`, id)
		return id, nil
	}
	err = h.db.QueryRow(ctx, `
		INSERT INTO project_deployments (project_id, environment, image_tag, build_status, status)
		VALUES ($1, $2, $3, 'running', 'in_progress')
		RETURNING id`, projectID, env, tag).Scan(&id)
	return id, err
}

func (h *Handler) markDeploymentBuildStarted(ctx context.Context, projectID int64, env, imageTag string) {
	id, err := h.upsertDeployment(ctx, projectID, env, imageTag)
	if err != nil || id <= 0 {
		return
	}
	// Ghi snapshot Console ngay khi build bắt đầu — tránh badge mặc định "single · app"
	// khi deploy_layout còn trống (trước đây chỉ ghi lúc apply cluster).
	p, err := h.getProjectByID(ctx, projectID)
	if err != nil {
		return
	}
	repo, _ := h.getProjectRepo(ctx, p.ID)
	params := h.buildDeployParams(ctx, p, repo, env, imageTag, false)
	h.saveDeploymentSnapshot(ctx, id, h.buildDeploySnapshot(p, repo, params))
}

// markDeploymentDeploySkipped — CI đã build image nhưng hook không apply cluster (auto-deploy tắt).
func (h *Handler) markDeploymentDeploySkipped(ctx context.Context, projectID int64, env, imageTag, reason string) {
	id, err := h.getOrCreateDeploymentID(ctx, projectID, env, imageTag)
	if err != nil || id <= 0 {
		return
	}
	_, _ = h.db.Exec(ctx, `
		UPDATE project_deployments SET
			status='failed', build_status='success', registry_status='success',
			deploy_status='skipped', runtime_status='skipped',
			error_phase='deploy', error_message=$1,
			runtime_detail=$1, updated_at=now(), finished_at=now()
		WHERE id=$2`, reason, id)
}

func (h *Handler) markDeploymentFailed(ctx context.Context, id int64, phase, msg string) {
	_, _ = h.db.Exec(ctx, `
		UPDATE project_deployments SET
			status='failed', error_phase=$1, error_message=$2,
			build_status=CASE WHEN $1='build' THEN 'failed' ELSE build_status END,
			registry_status=CASE WHEN $1='registry' THEN 'failed' ELSE registry_status END,
			deploy_status=CASE WHEN $1='deploy' THEN 'failed' ELSE deploy_status END,
			runtime_status=CASE WHEN $1='runtime' THEN 'failed' ELSE runtime_status END,
			updated_at=now(), finished_at=now()
		WHERE id=$3`, phase, msg, id)
}

func (h *Handler) clearFalseBuildFailure(ctx context.Context, id int64) {
	_, _ = h.db.Exec(ctx, `
		UPDATE project_deployments SET
			build_status='success',
			error_phase=CASE WHEN error_phase='build' THEN '' ELSE error_phase END,
			error_message=CASE WHEN error_phase='build' OR error_message ILIKE 'GitHub Actions:%' THEN '' ELSE error_message END,
			status=CASE WHEN status='failed' AND deploy_status='success' AND runtime_status IN ('success','pending','running') THEN 'in_progress' ELSE status END,
			finished_at=CASE WHEN status='failed' AND deploy_status='success' THEN NULL ELSE finished_at END,
			updated_at=now()
		WHERE id=$1`, id)
}

func (h *Handler) markDeploymentDeployPhase(ctx context.Context, id int64, image string) {
	_, _ = h.db.Exec(ctx, `
		UPDATE project_deployments SET
			build_status='success', registry_status='success',
			deploy_status='running', runtime_status='pending',
			image=$1, updated_at=now()
		WHERE id=$2`, image, id)
}

func (h *Handler) markDeploymentSuccess(ctx context.Context, id int64, runtimeDetail string) {
	_, _ = h.db.Exec(ctx, `
		UPDATE project_deployments SET
			status='success',
			build_status='success', registry_status='success',
			deploy_status='success', runtime_status='success',
			runtime_detail=$1, error_phase='', error_message='',
			updated_at=now(), finished_at=now()
		WHERE id=$2`, runtimeDetail, id)
}

func (h *Handler) getOrCreateDeploymentID(ctx context.Context, projectID int64, env, imageTag string) (int64, error) {
	var id int64
	err := h.db.QueryRow(ctx, `
		SELECT id FROM project_deployments
		WHERE project_id=$1 AND environment=$2 AND image_tag=$3
		ORDER BY id DESC LIMIT 1`, projectID, env, imageTag).Scan(&id)
	if err == nil {
		return id, nil
	}
	return h.upsertDeployment(ctx, projectID, env, imageTag)
}

// beginRollbackDeployment — mỗi lần rollback tạo row mới (không reuse bản cũ) để tracking/UI đúng.
func (h *Handler) beginRollbackDeployment(ctx context.Context, projectID int64, env, imageTag string) (int64, error) {
	env = strings.ToLower(strings.TrimSpace(env))
	if env == "" {
		env = "dev"
	}
	tag := strings.TrimSpace(imageTag)
	var id int64
	err := h.db.QueryRow(ctx, `
		INSERT INTO project_deployments (project_id, environment, image_tag, build_status, registry_status, deploy_status, status)
		VALUES ($1, $2, $3, 'success', 'success', 'running', 'in_progress')
		RETURNING id`, projectID, env, tag).Scan(&id)
	if err != nil {
		return 0, err
	}
	h.sealStaleInProgressDeployments(ctx, projectID, env, id)
	return id, nil
}

func (h *Handler) sealStaleInProgressDeployments(ctx context.Context, projectID int64, env string, exceptID int64) {
	env = strings.ToLower(strings.TrimSpace(env))
	if env == "" {
		env = "dev"
	}
	_, _ = h.db.Exec(ctx, `
		UPDATE project_deployments SET
			status='success', runtime_status='success',
			runtime_detail=CASE WHEN COALESCE(runtime_detail,'')='' THEN 'Đã thay bởi deploy mới hơn' ELSE runtime_detail END,
			error_phase='', error_message='', updated_at=now(),
			finished_at=COALESCE(finished_at, now())
		WHERE project_id=$1 AND environment=$2 AND status='in_progress' AND id <> $3`,
		projectID, env, exceptID)
}

func (h *Handler) hasSuccessfulDeployForTag(ctx context.Context, projectID int64, env, imageTag string) bool {
	env = strings.ToLower(strings.TrimSpace(env))
	if env == "" {
		env = "dev"
	}
	tag := strings.TrimSpace(imageTag)
	if tag == "" {
		return false
	}
	var ok bool
	_ = h.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM project_deployments
			WHERE project_id=$1 AND environment=$2 AND image_tag=$3 AND status='success'
		)`, projectID, env, tag).Scan(&ok)
	return ok
}
