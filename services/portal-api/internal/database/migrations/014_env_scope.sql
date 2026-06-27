-- Runtime vs build (Dockerfile ARG) cho cấu hình app.

ALTER TABLE project_env_vars ADD COLUMN IF NOT EXISTS scope TEXT NOT NULL DEFAULT 'runtime';

ALTER TABLE project_env_vars DROP CONSTRAINT IF EXISTS project_env_vars_scope_check;
ALTER TABLE project_env_vars ADD CONSTRAINT project_env_vars_scope_check
    CHECK (scope IN ('runtime', 'build'));

ALTER TABLE project_env_vars DROP CONSTRAINT IF EXISTS project_env_vars_project_id_environment_key_key;
ALTER TABLE project_env_vars DROP CONSTRAINT IF EXISTS project_env_vars_project_env_key_scope_key;
ALTER TABLE project_env_vars ADD CONSTRAINT project_env_vars_project_env_key_scope_key
    UNIQUE (project_id, environment, key, scope);

CREATE INDEX IF NOT EXISTS idx_project_env_vars_scope
    ON project_env_vars (project_id, environment, scope);
