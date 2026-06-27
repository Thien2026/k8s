-- Deployment activity timeline (build → registry → k8s → runtime)

CREATE TABLE IF NOT EXISTS project_deployments (
    id              BIGSERIAL PRIMARY KEY,
    project_id      BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    environment     TEXT NOT NULL DEFAULT 'dev',
    image_tag       TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'in_progress',
    build_status    TEXT NOT NULL DEFAULT 'pending',
    registry_status TEXT NOT NULL DEFAULT 'pending',
    deploy_status   TEXT NOT NULL DEFAULT 'pending',
    runtime_status  TEXT NOT NULL DEFAULT 'pending',
    error_phase     TEXT NOT NULL DEFAULT '',
    error_message   TEXT NOT NULL DEFAULT '',
    github_run_id   BIGINT NOT NULL DEFAULT 0,
    github_run_url  TEXT NOT NULL DEFAULT '',
    image           TEXT NOT NULL DEFAULT '',
    runtime_detail  TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_project_deployments_project_created
    ON project_deployments (project_id, created_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS idx_project_deployments_project_tag_env
    ON project_deployments (project_id, environment, image_tag)
    WHERE image_tag <> '';
