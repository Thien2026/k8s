-- Cache contract env từ repo (sync workflow) — validate deploy hook không cần OAuth user.

ALTER TABLE project_repos ADD COLUMN IF NOT EXISTS env_contract_build TEXT NOT NULL DEFAULT '';
ALTER TABLE project_repos ADD COLUMN IF NOT EXISTS env_contract_runtime TEXT NOT NULL DEFAULT '';
