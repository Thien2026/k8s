-- Project hub: slug, git repo, domains

ALTER TABLE projects ADD COLUMN IF NOT EXISTS slug TEXT;

UPDATE projects SET slug = lower(regexp_replace(trim(name), '[^a-zA-Z0-9]+', '-', 'g'))
WHERE slug IS NULL OR slug = '';

UPDATE projects SET slug = 'project-' || id::text WHERE slug IS NULL OR slug = '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_projects_slug ON projects(slug);

CREATE TABLE IF NOT EXISTS project_repos (
    project_id       BIGINT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    git_url          TEXT NOT NULL DEFAULT '',
    branch           TEXT NOT NULL DEFAULT 'main',
    dockerfile_path  TEXT NOT NULL DEFAULT 'Dockerfile',
    build_context    TEXT NOT NULL DEFAULT '.',
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS project_domains (
    id           BIGSERIAL PRIMARY KEY,
    project_id   BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    hostname     TEXT NOT NULL,
    environment  TEXT NOT NULL DEFAULT 'dev' CHECK (environment IN ('dev', 'prod')),
    tls_enabled  BOOLEAN NOT NULL DEFAULT true,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, hostname)
);

CREATE INDEX IF NOT EXISTS idx_project_domains_project ON project_domains(project_id);
