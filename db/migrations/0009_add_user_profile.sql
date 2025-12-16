-- +goose Up

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS default_tenant_id UUID;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS theme_preference TEXT NOT NULL DEFAULT 'system';

-- +goose StatementBegin
DO $$
BEGIN
    BEGIN
        ALTER TABLE users
            ADD CONSTRAINT fk_users_default_tenant
                FOREIGN KEY (default_tenant_id) REFERENCES tenants(id) ON DELETE SET NULL;
    EXCEPTION
        WHEN duplicate_object THEN
            NULL;
    END;
END
$$;
-- +goose StatementEnd

-- +goose Down

ALTER TABLE users DROP CONSTRAINT IF EXISTS fk_users_default_tenant;
ALTER TABLE users DROP COLUMN IF EXISTS default_tenant_id;
ALTER TABLE users DROP COLUMN IF EXISTS theme_preference;
