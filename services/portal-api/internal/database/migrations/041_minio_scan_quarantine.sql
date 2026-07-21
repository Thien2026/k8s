-- Phase 2.2: policy antivirus opt-in theo bucket.
-- Mặc định disabled để giữ nguyên S3 direct flow của app hiện có.
ALTER TABLE project_minio_buckets
    ADD COLUMN IF NOT EXISTS scan_mode TEXT NOT NULL DEFAULT 'disabled'
        CHECK (scan_mode IN ('disabled', 'required'));

-- Các bucket nội bộ chỉ được tạo khi scan_mode=required. Chúng không phải bucket
-- tự phục vụ và không được inject vào app-env.
CREATE TABLE IF NOT EXISTS project_minio_scan_profiles (
    bucket_id                   BIGINT PRIMARY KEY REFERENCES project_minio_buckets(id) ON DELETE CASCADE,
    quarantine_bucket           TEXT NOT NULL UNIQUE,
    infected_bucket             TEXT NOT NULL UNIQUE,
    scanner_connection_secret   TEXT NOT NULL UNIQUE,
    uploader_connection_secret  TEXT NOT NULL UNIQUE,
    status                      TEXT NOT NULL DEFAULT 'provisioning'
                                CHECK (status IN ('provisioning', 'ready', 'failed', 'disabling')),
    error_message               TEXT,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Object được stage bằng key bất biến (UUID), tránh race khi client upload cùng
-- final key trong lúc scanner đang chạy. Không có trạng thái fail-open.
CREATE TABLE IF NOT EXISTS project_minio_object_scans (
    id                  UUID PRIMARY KEY,
    project_id          BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    environment         TEXT NOT NULL CHECK (environment IN ('dev', 'prod')),
    bucket_id           BIGINT NOT NULL REFERENCES project_minio_buckets(id) ON DELETE CASCADE,
    quarantine_key      TEXT NOT NULL UNIQUE,
    destination_key     TEXT NOT NULL,
    object_size         BIGINT NOT NULL CHECK (object_size >= 0),
    content_type        TEXT,
    status              TEXT NOT NULL DEFAULT 'queued'
                        CHECK (status IN ('queued', 'scanning', 'clean', 'infected', 'failed', 'expired')),
    attempts            INT NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    scanner_image       TEXT,
    signature_version   TEXT,
    detection_name      TEXT,
    error_message       TEXT,
    requested_by        BIGINT REFERENCES users(id) ON DELETE SET NULL,
    started_at          TIMESTAMPTZ,
    finished_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS project_minio_object_scans_bucket_status_created_idx
    ON project_minio_object_scans (bucket_id, status, created_at DESC);
