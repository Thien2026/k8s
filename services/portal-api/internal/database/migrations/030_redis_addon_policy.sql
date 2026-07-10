-- Redis addon: eviction policy + gợi ý TTL mặc định cho app (REDIS_KEY_TTL_SECONDS).

ALTER TABLE project_data_addons
    ADD COLUMN IF NOT EXISTS maxmemory_policy TEXT NOT NULL DEFAULT 'allkeys-lru';

ALTER TABLE project_data_addons
    ADD COLUMN IF NOT EXISTS default_key_ttl_sec INT NOT NULL DEFAULT 86400;

ALTER TABLE project_data_addons DROP CONSTRAINT IF EXISTS project_data_addons_maxmemory_policy_check;
ALTER TABLE project_data_addons
    ADD CONSTRAINT project_data_addons_maxmemory_policy_check
    CHECK (maxmemory_policy IN (
        'allkeys-lru', 'volatile-lru', 'allkeys-lfu', 'volatile-lfu',
        'allkeys-random', 'volatile-random', 'volatile-ttl', 'noeviction'
    ));

ALTER TABLE project_data_addons DROP CONSTRAINT IF EXISTS project_data_addons_default_key_ttl_check;
ALTER TABLE project_data_addons
    ADD CONSTRAINT project_data_addons_default_key_ttl_check
    CHECK (default_key_ttl_sec >= 0 AND default_key_ttl_sec <= 2592000);
