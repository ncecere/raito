-- +goose Up

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    name TEXT,
    auth_provider TEXT NOT NULL,
    auth_subject TEXT,
    is_system_admin BOOLEAN NOT NULL DEFAULT FALSE,
    password_hash TEXT,
    password_version INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS tenants (
    id UUID PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    type TEXT NOT NULL, -- e.g. "personal" or "org"
    owner_user_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_tenants_owner_user FOREIGN KEY (owner_user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS tenant_members (
    tenant_id UUID NOT NULL,
    user_id UUID NOT NULL,
    role TEXT NOT NULL, -- "tenant_admin" or "tenant_member"
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, user_id),
    CONSTRAINT fk_tenant_members_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
    CONSTRAINT fk_tenant_members_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS user_id UUID,
    ADD CONSTRAINT fk_api_keys_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;

-- +goose Down

ALTER TABLE api_keys DROP CONSTRAINT IF EXISTS fk_api_keys_user;
ALTER TABLE api_keys DROP COLUMN IF EXISTS user_id;

DROP TABLE IF EXISTS tenant_members;
DROP TABLE IF EXISTS tenants;
DROP TABLE IF EXISTS users;
