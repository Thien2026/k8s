-- Snapshot layout + fleet lúc sync workflow — validate-config/hook so khớp Console.

ALTER TABLE project_repos ADD COLUMN IF NOT EXISTS workflow_sync_layout TEXT NOT NULL DEFAULT '';
ALTER TABLE project_repos ADD COLUMN IF NOT EXISTS workflow_sync_services TEXT NOT NULL DEFAULT '';
