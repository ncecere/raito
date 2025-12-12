-- +goose Up
CREATE TABLE IF NOT EXISTS api_keys (
    id UUID PRIMARY KEY,
    key_hash TEXT NOT NULL UNIQUE,
    label TEXT NOT NULL,
    is_admin BOOLEAN NOT NULL DEFAULT FALSE,
    rate_limit_per_minute INT,
    tenant_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ
);

-- +goose Down
DROP TABLE IF EXISTS api_keys;
