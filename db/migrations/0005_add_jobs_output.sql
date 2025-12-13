-- +goose Up
ALTER TABLE jobs
  ADD COLUMN IF NOT EXISTS output JSONB;

-- +goose Down
ALTER TABLE jobs
  DROP COLUMN IF EXISTS output;
