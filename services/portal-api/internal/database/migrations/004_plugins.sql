-- Platform plugins (addon registry — WordPress-style)

CREATE TABLE IF NOT EXISTS platform_plugins (
    name          TEXT PRIMARY KEY,
    label         TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    category      TEXT NOT NULL DEFAULT 'addon',
    enabled       BOOLEAN NOT NULL DEFAULT false,
    core          BOOLEAN NOT NULL DEFAULT false,
    version       TEXT NOT NULL DEFAULT '',
    bootstrap     TEXT NOT NULL DEFAULT '',
    installed_at  TIMESTAMPTZ,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO platform_plugins (name, label, description, category, enabled, core, bootstrap) VALUES
    ('console', 'Platform Console', 'Core — auth, projects, project hub', 'core', true, true, ''),
    ('ghcr', 'GitHub Container Registry', 'Registry mặc định — push image qua GitHub Actions, không cài thêm trên VPS', 'registry', true, false, ''),
    ('rancher', 'Rancher / Kubernetes', 'Explorer cluster, K8s API, join worker — cài bootstrap/addons/install-rancher.sh', 'cluster', false, false, 'addons/install-rancher.sh'),
    ('harbor', 'Harbor Registry', 'Registry on-prem — scan image, robot CI — cài bootstrap/addons/install-harbor.sh', 'registry', false, false, 'addons/install-harbor.sh')
ON CONFLICT (name) DO NOTHING;
