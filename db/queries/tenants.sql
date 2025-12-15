-- name: CreateTenant :one
INSERT INTO tenants (id, slug, name, type, owner_user_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetTenantByID :one
SELECT * FROM tenants WHERE id = $1;

-- name: GetTenantBySlug :one
SELECT * FROM tenants WHERE slug = $1;

-- name: ListTenants :many
SELECT * FROM tenants
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: AdminCountTenants :one
SELECT COUNT(*) FROM tenants
WHERE ($1 = '' OR slug ILIKE '%' || $1 || '%' OR name ILIKE '%' || $1 || '%')
  AND ($2 OR type <> 'personal');

-- name: AdminListTenants :many
SELECT * FROM tenants
WHERE ($1 = '' OR slug ILIKE '%' || $1 || '%' OR name ILIKE '%' || $1 || '%')
  AND ($2 OR type <> 'personal')
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;

-- name: ListPersonalTenantsForUser :many
SELECT * FROM tenants
WHERE owner_user_id = $1 AND type = 'personal'
ORDER BY created_at ASC;

-- name: ListTenantsForUser :many
SELECT tenants.* FROM tenants
JOIN tenant_members ON tenant_members.tenant_id = tenants.id
WHERE tenant_members.user_id = $1
ORDER BY tenants.created_at DESC;

-- name: AdminListTenantMembers :many
SELECT
    tenant_members.tenant_id,
    tenant_members.user_id,
    tenant_members.role,
    tenant_members.created_at,
    tenant_members.updated_at,
    users.email,
    users.name
FROM tenant_members
JOIN users ON users.id = tenant_members.user_id
WHERE tenant_members.tenant_id = $1
ORDER BY tenant_members.created_at DESC
LIMIT $2 OFFSET $3;

-- name: AdminSetTenantDefaultAPIKeyRateLimit :one
UPDATE tenants
SET default_api_key_rate_limit_per_minute = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;
