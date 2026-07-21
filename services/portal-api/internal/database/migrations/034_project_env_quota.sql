-- B1: Trần tài nguyên mỗi (project × env) — tách dev/prod, thực thi bằng K8s ResourceQuota.
-- Lớp admin: bảo vệ project không lấn tài nguyên lẫn nhau (giống hosting: dung lượng, RAM, CPU).
-- Default toàn hệ nằm trong platform_policy; override riêng từng project nằm ở project_env_quota.

-- 1) Default quota cho project mới (admin chỉnh 1 lần, áp mọi project chưa override).
ALTER TABLE platform_policy
    ADD COLUMN IF NOT EXISTS default_quota_storage_gb_dev  INT NOT NULL DEFAULT 20
        CHECK (default_quota_storage_gb_dev  >= 1 AND default_quota_storage_gb_dev  <= 2000),
    ADD COLUMN IF NOT EXISTS default_quota_storage_gb_prod INT NOT NULL DEFAULT 50
        CHECK (default_quota_storage_gb_prod >= 1 AND default_quota_storage_gb_prod <= 2000),
    ADD COLUMN IF NOT EXISTS default_quota_memory_mb_dev   INT NOT NULL DEFAULT 2048
        CHECK (default_quota_memory_mb_dev  >= 128 AND default_quota_memory_mb_dev  <= 65536),
    ADD COLUMN IF NOT EXISTS default_quota_memory_mb_prod  INT NOT NULL DEFAULT 4096
        CHECK (default_quota_memory_mb_prod >= 128 AND default_quota_memory_mb_prod <= 65536),
    ADD COLUMN IF NOT EXISTS default_quota_cpu_m_dev       INT NOT NULL DEFAULT 2000
        CHECK (default_quota_cpu_m_dev  >= 100 AND default_quota_cpu_m_dev  <= 64000),
    ADD COLUMN IF NOT EXISTS default_quota_cpu_m_prod      INT NOT NULL DEFAULT 4000
        CHECK (default_quota_cpu_m_prod >= 100 AND default_quota_cpu_m_prod <= 64000),
    ADD COLUMN IF NOT EXISTS default_quota_max_pods        INT NOT NULL DEFAULT 50
        CHECK (default_quota_max_pods >= 1 AND default_quota_max_pods <= 500),
    ADD COLUMN IF NOT EXISTS default_quota_max_pvcs        INT NOT NULL DEFAULT 20
        CHECK (default_quota_max_pvcs >= 1 AND default_quota_max_pvcs <= 200);

-- 2) Quota thực tế mỗi (project × env). is_override=true khi admin nâng riêng (dự án trọng điểm).
CREATE TABLE IF NOT EXISTS project_env_quota (
    project_id   BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    environment  TEXT   NOT NULL CHECK (environment IN ('dev','prod')),
    storage_gb   INT    NOT NULL CHECK (storage_gb >= 1  AND storage_gb <= 2000),
    memory_mb    INT    NOT NULL CHECK (memory_mb  >= 128 AND memory_mb <= 65536),
    cpu_m        INT    NOT NULL CHECK (cpu_m      >= 100 AND cpu_m     <= 64000),
    max_pods     INT    NOT NULL DEFAULT 50 CHECK (max_pods >= 1 AND max_pods <= 500),
    max_pvcs     INT    NOT NULL DEFAULT 20 CHECK (max_pvcs >= 1 AND max_pvcs <= 200),
    is_override  BOOLEAN NOT NULL DEFAULT false,
    updated_by   BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, environment)
);

-- 3) Backfill: sinh quota mặc định cho mọi project hiện có (dev + prod) từ default policy.
INSERT INTO project_env_quota (project_id, environment, storage_gb, memory_mb, cpu_m, max_pods, max_pvcs)
SELECT p.id, e.env,
       CASE WHEN e.env = 'prod' THEN pol.default_quota_storage_gb_prod ELSE pol.default_quota_storage_gb_dev END,
       CASE WHEN e.env = 'prod' THEN pol.default_quota_memory_mb_prod  ELSE pol.default_quota_memory_mb_dev  END,
       CASE WHEN e.env = 'prod' THEN pol.default_quota_cpu_m_prod      ELSE pol.default_quota_cpu_m_dev      END,
       pol.default_quota_max_pods, pol.default_quota_max_pvcs
FROM projects p
CROSS JOIN (VALUES ('dev'), ('prod')) AS e(env)
CROSS JOIN platform_policy pol
WHERE pol.id = 1
ON CONFLICT (project_id, environment) DO NOTHING;
