-- Retention theo số run thành công, phù hợp backup full MinIO:
-- khi run mới success, chỉ giữ N run mới nhất của target.
ALTER TABLE backup_targets
    ADD COLUMN IF NOT EXISTS retention_count INT NOT NULL DEFAULT 3
    CHECK (retention_count BETWEEN 1 AND 1000);
