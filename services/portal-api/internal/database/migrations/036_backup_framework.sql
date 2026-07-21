-- Backup framework Phase 1: target trung lập (S3-compatible trước), run history và manifest.
-- Credential không lưu DB; target chỉ tham chiếu Kubernetes Secret ở namespace platform.

CREATE TABLE IF NOT EXISTS backup_targets (
    id                  BIGSERIAL PRIMARY KEY,
    name                TEXT NOT NULL UNIQUE,
    provider            TEXT NOT NULL CHECK (provider IN ('s3')),
    endpoint            TEXT NOT NULL,
    region              TEXT NOT NULL DEFAULT 'us-east-1',
    bucket              TEXT NOT NULL,
    prefix              TEXT NOT NULL DEFAULT 'platform-backups',
    credentials_secret  TEXT NOT NULL,
    schedule_cron       TEXT NOT NULL DEFAULT '0 3 * * *',
    retention_days      INT NOT NULL DEFAULT 14 CHECK (retention_days BETWEEN 1 AND 3650),
    encryption_enabled  BOOLEAN NOT NULL DEFAULT true,
    enabled             BOOLEAN NOT NULL DEFAULT false,
    last_tested_at      TIMESTAMPTZ,
    last_test_error     TEXT,
    created_by          BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS backup_runs (
    id                  BIGSERIAL PRIMARY KEY,
    target_id           BIGINT NOT NULL REFERENCES backup_targets(id) ON DELETE RESTRICT,
    run_kind            TEXT NOT NULL CHECK (run_kind IN ('manual', 'scheduled', 'restore_drill')),
    status              TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'success', 'failed')),
    run_prefix          TEXT NOT NULL,
    manifest_key        TEXT,
    artifact_count      INT NOT NULL DEFAULT 0,
    total_bytes         BIGINT NOT NULL DEFAULT 0,
    checksum_sha256     TEXT,
    error_message       TEXT,
    started_by          BIGINT REFERENCES users(id) ON DELETE SET NULL,
    started_at          TIMESTAMPTZ,
    finished_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS backup_runs_target_created_idx
    ON backup_runs (target_id, created_at DESC);
