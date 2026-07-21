-- Một Platform run mirror từng bucket độc lập. Schema 037 chỉ unique theo project/env
-- nên không thể ghi app + media trong cùng run.
DO $$
DECLARE
    constraint_name TEXT;
BEGIN
    SELECT conname INTO constraint_name
    FROM pg_constraint
    WHERE conrelid = 'project_backup_artifacts'::regclass
      AND contype = 'u'
      AND pg_get_constraintdef(oid) LIKE '%backup_run_id, project_id, environment%';
    IF constraint_name IS NOT NULL THEN
        EXECUTE format('ALTER TABLE project_backup_artifacts DROP CONSTRAINT %I', constraint_name);
    END IF;
END $$;

ALTER TABLE project_backup_artifacts
    ADD CONSTRAINT project_backup_artifacts_run_project_env_bucket_key
    UNIQUE (backup_run_id, project_id, environment, source_bucket);
