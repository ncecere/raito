-- +goose Up

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS is_disabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS disabled_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_users_is_disabled ON users(is_disabled);

-- +goose Down

DROP INDEX IF EXISTS idx_users_is_disabled;
ALTER TABLE users DROP COLUMN IF EXISTS disabled_at;
ALTER TABLE users DROP COLUMN IF EXISTS is_disabled;

