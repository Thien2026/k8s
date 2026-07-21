-- Trạng thái kiểm chứng native hard quota của MinIO.
-- Chỉ được ghi thành công sau khi init Job hoàn thành và `mc quota info` đọc lại được quota.
ALTER TABLE project_data_addons
    ADD COLUMN IF NOT EXISTS minio_quota_verified_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS minio_quota_verify_error TEXT;
