-- Toggle auto-deploy per project; không ghi đè secret khi nhiều project / 1 repo.

ALTER TABLE project_repos ADD COLUMN IF NOT EXISTS auto_deploy_enabled BOOLEAN NOT NULL DEFAULT true;

UPDATE project_repos SET auto_deploy_enabled = false WHERE workflow_synced_at IS NULL;
