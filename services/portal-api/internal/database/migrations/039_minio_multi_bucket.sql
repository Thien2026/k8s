-- Phase 2.1: một MinIO instance / project / environment có nhiều bucket.
-- `app` là bucket tương thích ngược: app hiện hữu vẫn dùng S3_* không đổi.
CREATE TABLE IF NOT EXISTS project_minio_buckets (
    id                  BIGSERIAL PRIMARY KEY,
    project_id          BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    environment         TEXT NOT NULL CHECK (environment IN ('dev', 'prod')),
    name                TEXT NOT NULL CHECK (name ~ '^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$'),
    storage_gb          INT NOT NULL CHECK (storage_gb >= 1 AND storage_gb <= 2000),
    max_object_mb       INT NOT NULL DEFAULT 100 CHECK (max_object_mb >= 1 AND max_object_mb <= 51200),
    connection_secret   TEXT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'provisioning'
                        CHECK (status IN ('provisioning', 'running', 'failed', 'deleting')),
    quota_verified_at   TIMESTAMPTZ,
    quota_verify_error  TEXT,
    is_default          BOOLEAN NOT NULL DEFAULT false,
    created_by          BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, environment, name),
    UNIQUE (connection_secret)
);

CREATE UNIQUE INDEX IF NOT EXISTS project_minio_buckets_default_idx
    ON project_minio_buckets (project_id, environment) WHERE is_default;

-- Backfill legacy bucket; secret/runtime S3_* vẫn giữ nguyên để không làm app cũ gián đoạn.
INSERT INTO project_minio_buckets
    (project_id, environment, name, storage_gb, max_object_mb, connection_secret, status,
     quota_verified_at, quota_verify_error, is_default)
SELECT project_id, environment, 'app', storage_gb, max_object_mb, connection_secret, status,
       minio_quota_verified_at, minio_quota_verify_error, true
FROM project_data_addons
WHERE engine='minio' AND connection_secret <> ''
ON CONFLICT (project_id, environment, name) DO NOTHING;
