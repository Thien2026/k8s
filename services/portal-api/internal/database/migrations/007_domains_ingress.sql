-- Phase E: Ingress sync metadata cho project domains

ALTER TABLE project_domains ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT 'custom';
ALTER TABLE project_domains ADD COLUMN IF NOT EXISTS ingress_name TEXT NOT NULL DEFAULT '';
ALTER TABLE project_domains ADD COLUMN IF NOT EXISTS sync_status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE project_domains ADD COLUMN IF NOT EXISTS sync_error TEXT NOT NULL DEFAULT '';
ALTER TABLE project_domains ADD COLUMN IF NOT EXISTS cert_status TEXT NOT NULL DEFAULT 'unknown';

ALTER TABLE project_domains DROP CONSTRAINT IF EXISTS project_domains_kind_check;
ALTER TABLE project_domains ADD CONSTRAINT project_domains_kind_check CHECK (kind IN ('auto', 'custom'));

UPDATE project_domains SET ingress_name = 'app-' || id::text WHERE ingress_name = '' OR ingress_name IS NULL;
