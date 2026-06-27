-- Phase C: registry provider per project (ghcr default, harbor optional)

ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS registry_provider TEXT NOT NULL DEFAULT 'ghcr';

UPDATE projects
SET registry_provider = 'harbor'
WHERE registry_provider = 'ghcr'
  AND COALESCE(harbor_project, '') <> '';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'projects_registry_provider_check'
    ) THEN
        ALTER TABLE projects
            ADD CONSTRAINT projects_registry_provider_check
            CHECK (registry_provider IN ('ghcr', 'harbor'));
    END IF;
END $$;
