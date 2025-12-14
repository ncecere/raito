-- +goose Up
ALTER TABLE jobs ADD COLUMN tenant_id UUID;

-- +goose Down
ALTER TABLE jobs DROP COLUMN tenant_id;
