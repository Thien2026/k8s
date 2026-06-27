-- Multi-service layout: 1 repo → api + web (2 image, 2 Deployment, 1 Ingress)

ALTER TABLE projects ADD COLUMN IF NOT EXISTS layout TEXT NOT NULL DEFAULT 'single';

ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_layout_check;
ALTER TABLE projects ADD CONSTRAINT projects_layout_check CHECK (layout IN ('single', 'multi'));

CREATE TABLE IF NOT EXISTS project_services (
    id              BIGSERIAL PRIMARY KEY,
    project_id      BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    display_name    TEXT NOT NULL DEFAULT '',
    build_context   TEXT NOT NULL DEFAULT '.',
    build_mode      TEXT NOT NULL DEFAULT 'dockerfile',
    dockerfile_path TEXT NOT NULL DEFAULT 'Dockerfile',
    container_port  INT NOT NULL DEFAULT 8080,
    health_path     TEXT NOT NULL DEFAULT '/health',
    ingress_path    TEXT NOT NULL DEFAULT '/',
    sort_order      INT NOT NULL DEFAULT 0,
    enabled         BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, name)
);

CREATE INDEX IF NOT EXISTS idx_project_services_project ON project_services(project_id, sort_order);
