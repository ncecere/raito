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

-- name: ListPersonalTenantsForUser :many
SELECT * FROM tenants
WHERE owner_user_id = $1 AND type = 'personal'
ORDER BY created_at ASC;

-- name: ListTenantsForUser :many
SELECT tenants.* FROM tenants
JOIN tenant_members ON tenant_members.tenant_id = tenants.id
WHERE tenant_members.user_id = $1
ORDER BY tenants.created_at DESC;
