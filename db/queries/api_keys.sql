-- name: InsertAPIKey :one
INSERT INTO api_keys (id, key_hash, label, is_admin, rate_limit_per_minute, tenant_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetAPIKeyByHash :one
SELECT * FROM api_keys
WHERE key_hash = $1 AND revoked_at IS NULL
LIMIT 1;

-- name: ListAPIKeysByTenant :many
SELECT * FROM api_keys
WHERE tenant_id = $1 AND revoked_at IS NULL
ORDER BY created_at DESC;

-- name: RevokeAPIKey :exec
UPDATE api_keys
SET revoked_at = NOW()
WHERE id = $1 AND revoked_at IS NULL;

-- name: GetAPIKeyLabelsByIDs :many
SELECT id, label
FROM api_keys
WHERE id IN (sqlc.slice('ids'));

-- name: AdminCountAPIKeys :one
SELECT COUNT(*)
FROM api_keys k
LEFT JOIN tenants t ON t.id::text = k.tenant_id
LEFT JOIN users u ON u.id = k.user_id
WHERE
  ($1 = '' OR
    k.label ILIKE '%' || $1 || '%' OR
    COALESCE(t.name, '') ILIKE '%' || $1 || '%' OR
    COALESCE(t.slug, '') ILIKE '%' || $1 || '%' OR
    COALESCE(u.email, '') ILIKE '%' || $1 || '%' OR
    COALESCE(u.name, '') ILIKE '%' || $1 || '%'
  )
  AND ($2::boolean OR k.revoked_at IS NULL);

-- name: AdminListAPIKeys :many
SELECT
  k.id,
  k.label,
  k.is_admin,
  k.rate_limit_per_minute,
  k.tenant_id,
  k.user_id,
  k.created_at,
  k.revoked_at,
  t.name AS tenant_name,
  t.slug AS tenant_slug,
  t.type AS tenant_type,
  u.email AS user_email,
  u.name AS user_name
FROM api_keys k
LEFT JOIN tenants t ON t.id::text = k.tenant_id
LEFT JOIN users u ON u.id = k.user_id
WHERE
  ($1 = '' OR
    k.label ILIKE '%' || $1 || '%' OR
    COALESCE(t.name, '') ILIKE '%' || $1 || '%' OR
    COALESCE(t.slug, '') ILIKE '%' || $1 || '%' OR
    COALESCE(u.email, '') ILIKE '%' || $1 || '%' OR
    COALESCE(u.name, '') ILIKE '%' || $1 || '%'
  )
  AND ($2::boolean OR k.revoked_at IS NULL)
ORDER BY k.created_at DESC
LIMIT $3 OFFSET $4;

-- name: AdminRevokeAPIKey :one
UPDATE api_keys
SET revoked_at = NOW()
WHERE id = $1 AND revoked_at IS NULL
RETURNING id, revoked_at;
