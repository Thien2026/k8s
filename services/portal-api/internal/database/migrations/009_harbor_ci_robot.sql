-- Harbor CI robot credentials (platform-managed, không đưa cho dev)

ALTER TABLE projects ADD COLUMN IF NOT EXISTS harbor_robot_name TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN IF NOT EXISTS harbor_robot_secret TEXT NOT NULL DEFAULT '';
