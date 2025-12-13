-- +goose Up
ALTER TABLE documents ADD COLUMN IF NOT EXISTS engine TEXT;

-- +goose Down
ALTER TABLE documents DROP COLUMN IF EXISTS engine;
