-- +goose Up
ALTER TABLE jobs
  ADD COLUMN IF NOT EXISTS priority INT NOT NULL DEFAULT 10,
  ADD COLUMN IF NOT EXISTS sync BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_jobs_status_priority_created
  ON jobs (status, priority DESC, created_at ASC);

-- +goose Down
DROP INDEX IF EXISTS idx_jobs_status_priority_created;
ALTER TABLE jobs DROP COLUMN IF EXISTS sync;
ALTER TABLE jobs DROP COLUMN IF EXISTS priority;
