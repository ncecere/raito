-- +goose Up

ALTER TABLE tenants
    ADD COLUMN IF NOT EXISTS default_api_key_rate_limit_per_minute INT;

-- +goose Down

ALTER TABLE tenants
    DROP COLUMN IF EXISTS default_api_key_rate_limit_per_minute;

