-- name: InsertAPIKey :one
INSERT INTO api_keys (id, key_hash, label, is_admin, rate_limit_per_minute, tenant_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetAPIKeyByHash :one
SELECT * FROM api_keys
WHERE key_hash = $1 AND revoked_at IS NULL
LIMIT 1;
