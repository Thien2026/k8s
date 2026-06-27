-- GHCR pull credentials for K8s imagePullSecret (platform-managed)

ALTER TABLE projects ADD COLUMN IF NOT EXISTS ghcr_pull_user TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN IF NOT EXISTS ghcr_pull_token TEXT NOT NULL DEFAULT '';
