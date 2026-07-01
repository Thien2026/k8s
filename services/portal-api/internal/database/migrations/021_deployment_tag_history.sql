-- Cho phép nhiều lần deploy/rollback cùng image tag (lịch sử đầy đủ).
DROP INDEX IF EXISTS idx_project_deployments_project_tag_env;

CREATE INDEX IF NOT EXISTS idx_project_deployments_project_tag_env
    ON project_deployments (project_id, environment, image_tag, id DESC);
