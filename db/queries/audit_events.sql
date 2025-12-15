-- name: InsertAuditEvent :one
INSERT INTO audit_events (
  action,
  actor_user_id,
  actor_api_key_id,
  tenant_id,
  resource_type,
  resource_id,
  ip,
  user_agent,
  metadata
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: AdminCountAuditEvents :one
SELECT COUNT(*)
FROM audit_events e
LEFT JOIN users u ON u.id = e.actor_user_id
LEFT JOIN api_keys k ON k.id = e.actor_api_key_id
LEFT JOIN tenants t ON t.id = e.tenant_id
WHERE
  ($1 = '' OR
    e.action ILIKE '%' || $1 || '%' OR
    COALESCE(e.resource_type, '') ILIKE '%' || $1 || '%' OR
    COALESCE(e.resource_id, '') ILIKE '%' || $1 || '%' OR
    COALESCE(u.email, '') ILIKE '%' || $1 || '%' OR
    COALESCE(u.name, '') ILIKE '%' || $1 || '%' OR
    COALESCE(k.label, '') ILIKE '%' || $1 || '%' OR
    COALESCE(t.name, '') ILIKE '%' || $1 || '%' OR
    COALESCE(t.slug, '') ILIKE '%' || $1 || '%'
  )
  AND ($2 = '' OR e.action = $2)
  AND (NOT $3 OR e.tenant_id = $4)
  AND (NOT $5 OR e.actor_user_id = $6)
  AND (NOT $7 OR e.actor_api_key_id = $8)
  AND (NOT $9 OR e.created_at >= $10)
  AND (
    $11 = '' OR
    ($11 = 'api_key' AND e.actor_api_key_id IS NOT NULL) OR
    ($11 = 'session' AND e.actor_api_key_id IS NULL AND e.actor_user_id IS NOT NULL)
  );

-- name: AdminListAuditEvents :many
SELECT
  e.id,
  e.created_at,
  e.action,
  e.actor_user_id,
  e.actor_api_key_id,
  e.tenant_id,
  e.resource_type,
  e.resource_id,
  e.ip,
  e.user_agent,
  e.metadata,
  u.email AS actor_user_email,
  u.name AS actor_user_name,
  k.label AS actor_api_key_label,
  t.name AS tenant_name,
  t.slug AS tenant_slug,
  t.type AS tenant_type
FROM audit_events e
LEFT JOIN users u ON u.id = e.actor_user_id
LEFT JOIN api_keys k ON k.id = e.actor_api_key_id
LEFT JOIN tenants t ON t.id = e.tenant_id
WHERE
  ($1 = '' OR
    e.action ILIKE '%' || $1 || '%' OR
    COALESCE(e.resource_type, '') ILIKE '%' || $1 || '%' OR
    COALESCE(e.resource_id, '') ILIKE '%' || $1 || '%' OR
    COALESCE(u.email, '') ILIKE '%' || $1 || '%' OR
    COALESCE(u.name, '') ILIKE '%' || $1 || '%' OR
    COALESCE(k.label, '') ILIKE '%' || $1 || '%' OR
    COALESCE(t.name, '') ILIKE '%' || $1 || '%' OR
    COALESCE(t.slug, '') ILIKE '%' || $1 || '%'
  )
  AND ($2 = '' OR e.action = $2)
  AND (NOT $3 OR e.tenant_id = $4)
  AND (NOT $5 OR e.actor_user_id = $6)
  AND (NOT $7 OR e.actor_api_key_id = $8)
  AND (NOT $9 OR e.created_at >= $10)
  AND (
    $11 = '' OR
    ($11 = 'api_key' AND e.actor_api_key_id IS NOT NULL) OR
    ($11 = 'session' AND e.actor_api_key_id IS NULL AND e.actor_user_id IS NOT NULL)
  )
ORDER BY e.created_at DESC
LIMIT $12 OFFSET $13;
