-- Service có thể internal-only (ClusterIP, không Ingress).

ALTER TABLE project_services ADD COLUMN IF NOT EXISTS expose_ingress BOOLEAN NOT NULL DEFAULT true;
