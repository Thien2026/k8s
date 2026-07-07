-- Phase 8 — bật plugin monitoring sau khi stack đã cài (Grafana URL trong portal-api-env).
UPDATE platform_plugins
SET enabled = true,
    installed_at = COALESCE(installed_at, now()),
    updated_at = now()
WHERE name = 'monitoring';
