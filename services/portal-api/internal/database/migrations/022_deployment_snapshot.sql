-- Snapshot profile lúc deploy (layout, services, branch, images) — rollback & lịch sử.

ALTER TABLE project_deployments ADD COLUMN IF NOT EXISTS deploy_layout TEXT NOT NULL DEFAULT '';
ALTER TABLE project_deployments ADD COLUMN IF NOT EXISTS git_branch TEXT NOT NULL DEFAULT '';
ALTER TABLE project_deployments ADD COLUMN IF NOT EXISTS deploy_services JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE project_deployments ADD COLUMN IF NOT EXISTS deploy_images JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS idx_project_deployments_profile
    ON project_deployments (project_id, environment, image_tag, id DESC)
    WHERE deploy_layout <> '';
