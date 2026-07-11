-- Platform policy (singleton) — trần cứng toàn hệ; đổi cần step-up 2 lớp.

CREATE TABLE IF NOT EXISTS platform_policy (
    id                         INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    redis_max_memory_mb        INT NOT NULL DEFAULT 512
        CHECK (redis_max_memory_mb >= 64 AND redis_max_memory_mb <= 4096),
    redis_max_clients          INT NOT NULL DEFAULT 1000
        CHECK (redis_max_clients >= 10 AND redis_max_clients <= 10000),
    minio_max_storage_gb       INT NOT NULL DEFAULT 100
        CHECK (minio_max_storage_gb >= 1 AND minio_max_storage_gb <= 2000),
    minio_max_memory_mb        INT NOT NULL DEFAULT 1024
        CHECK (minio_max_memory_mb >= 128 AND minio_max_memory_mb <= 8192),
    minio_console_upload_mb    INT NOT NULL DEFAULT 10
        CHECK (minio_console_upload_mb >= 1 AND minio_console_upload_mb <= 512),
    ingress_proxy_body_size    TEXT NOT NULL DEFAULT '32m',
    policy_unlock_hash         TEXT NOT NULL DEFAULT '',
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by                 BIGINT REFERENCES users(id) ON DELETE SET NULL
);

INSERT INTO platform_policy (id) VALUES (1)
ON CONFLICT (id) DO NOTHING;

-- Nới CHECK storage addon theo trần policy (policy vẫn clamp runtime).
ALTER TABLE project_data_addons DROP CONSTRAINT IF EXISTS project_data_addons_storage_gb_check;
ALTER TABLE project_data_addons
    ADD CONSTRAINT project_data_addons_storage_gb_check
    CHECK (storage_gb >= 1 AND storage_gb <= 2000);
