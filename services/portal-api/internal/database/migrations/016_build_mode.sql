-- dockerfile | buildpack — platform tự gán sau khi quét repo GitHub
ALTER TABLE project_repos ADD COLUMN IF NOT EXISTS build_mode TEXT NOT NULL DEFAULT 'dockerfile';
