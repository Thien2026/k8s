-- MinIO app addon: engine + topology (standalone | distributed) + storage quota.

ALTER TABLE project_data_addons DROP CONSTRAINT IF EXISTS project_data_addons_engine_check;
ALTER TABLE project_data_addons
    ADD CONSTRAINT project_data_addons_engine_check
    CHECK (engine IN ('redis', 'postgres', 'minio'));

ALTER TABLE project_data_addons
    ADD COLUMN IF NOT EXISTS topology TEXT NOT NULL DEFAULT 'standalone';

ALTER TABLE project_data_addons
    ADD COLUMN IF NOT EXISTS storage_gb INT NOT NULL DEFAULT 5;

ALTER TABLE project_data_addons DROP CONSTRAINT IF EXISTS project_data_addons_topology_check;
ALTER TABLE project_data_addons
    ADD CONSTRAINT project_data_addons_topology_check
    CHECK (topology IN ('standalone', 'distributed'));

ALTER TABLE project_data_addons DROP CONSTRAINT IF EXISTS project_data_addons_storage_gb_check;
ALTER TABLE project_data_addons
    ADD CONSTRAINT project_data_addons_storage_gb_check
    CHECK (storage_gb >= 1 AND storage_gb <= 100);
