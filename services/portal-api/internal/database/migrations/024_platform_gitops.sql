-- Cấu hình GitOps platform (repo manifest + PAT push) — quản lý qua Console admin.
CREATE TABLE IF NOT EXISTS platform_gitops_settings (
    id           SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    repo_url     TEXT NOT NULL DEFAULT '',
    repo_branch  TEXT NOT NULL DEFAULT 'main',
    base_path    TEXT NOT NULL DEFAULT 'apps',
    push_token   TEXT NOT NULL DEFAULT '',
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO platform_gitops_settings (id) VALUES (1) ON CONFLICT (id) DO NOTHING;
