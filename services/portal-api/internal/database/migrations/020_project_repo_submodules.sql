-- L4C: Git submodule checkout mode (true | recursive) cho CI

ALTER TABLE project_repos ADD COLUMN IF NOT EXISTS git_submodules TEXT NOT NULL DEFAULT '';
