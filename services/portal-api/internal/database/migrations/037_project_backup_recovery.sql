-- Recovery theo project: artifact được ghi chỉ sau khi MinIO mirror thành công.
-- Restore luôn đi vào prefix sandbox, không có đường ghi đè production trong Phase 1.

CREATE TABLE IF NOT EXISTS project_backup_artifacts (
    id                BIGSERIAL PRIMARY KEY,
    backup_run_id     BIGINT NOT NULL REFERENCES backup_runs(id) ON DELETE CASCADE,
    project_id        BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    environment       TEXT NOT NULL CHECK (environment IN ('dev', 'prod')),
    source_namespace  TEXT NOT NULL,
    source_release    TEXT NOT NULL,
    source_bucket     TEXT NOT NULL DEFAULT 'app',
    object_prefix     TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'success' CHECK (status IN ('success', 'failed')),
    object_count      BIGINT,
    total_bytes       BIGINT,
    error_message     TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (backup_run_id, project_id, environment)
);

CREATE INDEX IF NOT EXISTS project_backup_artifacts_project_env_created_idx
    ON project_backup_artifacts (project_id, environment, created_at DESC);

CREATE TABLE IF NOT EXISTS project_backup_restores (
    id                  BIGSERIAL PRIMARY KEY,
    artifact_id         BIGINT NOT NULL REFERENCES project_backup_artifacts(id) ON DELETE RESTRICT,
    project_id          BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    target_environment  TEXT NOT NULL CHECK (target_environment IN ('dev', 'prod')),
    target_namespace    TEXT NOT NULL,
    target_bucket       TEXT NOT NULL DEFAULT 'app',
    target_prefix       TEXT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'queued'
                        CHECK (status IN ('queued', 'running', 'success', 'failed', 'cancelled')),
    requested_by        BIGINT REFERENCES users(id) ON DELETE SET NULL,
    started_at          TIMESTAMPTZ,
    finished_at         TIMESTAMPTZ,
    error_message       TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (target_namespace, target_bucket, target_prefix)
);

CREATE INDEX IF NOT EXISTS project_backup_restores_project_created_idx
    ON project_backup_restores (project_id, created_at DESC);
