-- name: AddTenantMember :one
INSERT INTO tenant_members (tenant_id, user_id, role)
VALUES ($1, $2, $3)
ON CONFLICT (tenant_id, user_id) DO UPDATE
SET role = tenant_members.role, updated_at = NOW()
RETURNING *;

-- name: GetTenantMember :one
SELECT * FROM tenant_members
WHERE tenant_id = $1 AND user_id = $2;

-- name: ListTenantMembers :many
SELECT * FROM tenant_members
WHERE tenant_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: UpdateTenantMemberRole :one
UPDATE tenant_members
SET role = $3, updated_at = NOW()
WHERE tenant_id = $1 AND user_id = $2
RETURNING *;

-- name: RemoveTenantMember :exec
DELETE FROM tenant_members
WHERE tenant_id = $1 AND user_id = $2;
