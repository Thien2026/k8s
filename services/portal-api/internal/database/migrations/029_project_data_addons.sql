-- Phase 10a: data addons per project (Redis first, Postgres later).

CREATE TABLE IF NOT EXISTS project_data_addons (
    id                BIGSERIAL PRIMARY KEY,
    project_id        BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    engine            TEXT NOT NULL,
    environment       TEXT NOT NULL DEFAULT 'dev',
    status            TEXT NOT NULL DEFAULT 'pending',
    k8s_release       TEXT NOT NULL DEFAULT '',
    connection_secret TEXT NOT NULL DEFAULT '',
    max_memory_mb     INT NOT NULL DEFAULT 128,
    max_clients       INT NOT NULL DEFAULT 100,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, engine, environment)
);

CREATE INDEX IF NOT EXISTS idx_project_data_addons_project ON project_data_addons(project_id);

ALTER TABLE project_data_addons DROP CONSTRAINT IF EXISTS project_data_addons_engine_check;
ALTER TABLE project_data_addons
    ADD CONSTRAINT project_data_addons_engine_check
    CHECK (engine IN ('redis', 'postgres'));

ALTER TABLE project_data_addons DROP CONSTRAINT IF EXISTS project_data_addons_environment_check;
ALTER TABLE project_data_addons
    ADD CONSTRAINT project_data_addons_environment_check
    CHECK (environment IN ('dev', 'prod'));

ALTER TABLE project_data_addons DROP CONSTRAINT IF EXISTS project_data_addons_status_check;
ALTER TABLE project_data_addons
    ADD CONSTRAINT project_data_addons_status_check
    CHECK (status IN ('pending', 'provisioning', 'running', 'failed', 'stopped'));
