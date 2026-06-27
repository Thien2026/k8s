-- Biến môi trường runtime cho app (thay .env không commit Git).

CREATE TABLE IF NOT EXISTS project_env_vars (
    id           BIGSERIAL PRIMARY KEY,
    project_id   BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    environment  TEXT NOT NULL DEFAULT 'dev',
    key          TEXT NOT NULL,
    value        TEXT NOT NULL DEFAULT '',
    is_secret    BOOLEAN NOT NULL DEFAULT false,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, environment, key)
);

ALTER TABLE project_env_vars DROP CONSTRAINT IF EXISTS project_env_vars_environment_check;
ALTER TABLE project_env_vars ADD CONSTRAINT project_env_vars_environment_check
    CHECK (environment IN ('dev', 'prod'));

CREATE INDEX IF NOT EXISTS idx_project_env_vars_project_env
    ON project_env_vars (project_id, environment);
