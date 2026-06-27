-- Phase G: GitHub OAuth + auto-deploy hook

ALTER TABLE project_repos ADD COLUMN IF NOT EXISTS github_owner TEXT NOT NULL DEFAULT '';
ALTER TABLE project_repos ADD COLUMN IF NOT EXISTS github_repo TEXT NOT NULL DEFAULT '';
ALTER TABLE project_repos ADD COLUMN IF NOT EXISTS deploy_environment TEXT NOT NULL DEFAULT 'dev';
ALTER TABLE project_repos ADD COLUMN IF NOT EXISTS deploy_hook_token TEXT NOT NULL DEFAULT '';
ALTER TABLE project_repos ADD COLUMN IF NOT EXISTS workflow_synced_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS user_github_tokens (
    user_id        BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    github_user_id BIGINT NOT NULL DEFAULT 0,
    github_login   TEXT NOT NULL DEFAULT '',
    access_token   TEXT NOT NULL,
    token_scope    TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
