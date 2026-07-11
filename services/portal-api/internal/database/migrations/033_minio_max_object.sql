-- Max object size: trần platform + quota từng project MinIO.
-- Console enforce cứng; app nhận S3_MAX_OBJECT_MB (gợi ý — hard S3 path ở Phase 2).

ALTER TABLE platform_policy
    ADD COLUMN IF NOT EXISTS minio_max_object_mb INT NOT NULL DEFAULT 5120
        CHECK (minio_max_object_mb >= 1 AND minio_max_object_mb <= 51200);

ALTER TABLE project_data_addons
    ADD COLUMN IF NOT EXISTS max_object_mb INT NOT NULL DEFAULT 100
        CHECK (max_object_mb >= 1 AND max_object_mb <= 51200);
