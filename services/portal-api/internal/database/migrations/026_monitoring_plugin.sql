-- Phase 8 — Monitoring plugin (Prometheus + Grafana)

INSERT INTO platform_plugins (name, label, description, category, enabled, core, bootstrap) VALUES
    ('monitoring', 'Prometheus & Grafana', 'Metrics, dashboards, Alertmanager — bootstrap/addons/install-monitoring.sh', 'observability', false, false, 'addons/install-monitoring.sh')
ON CONFLICT (name) DO NOTHING;
