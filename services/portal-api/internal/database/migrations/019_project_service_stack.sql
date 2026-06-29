-- L4B polyglot: stack hint per service (python, node, go, dotnet) for buildpack builder

ALTER TABLE project_services ADD COLUMN IF NOT EXISTS stack TEXT NOT NULL DEFAULT '';
