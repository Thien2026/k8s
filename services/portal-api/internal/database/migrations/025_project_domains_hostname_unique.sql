-- Một hostname chỉ thuộc một project (tránh Ingress trùng host trên cluster).
CREATE UNIQUE INDEX IF NOT EXISTS idx_project_domains_hostname_lower
    ON project_domains (LOWER(hostname));
