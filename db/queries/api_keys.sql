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
