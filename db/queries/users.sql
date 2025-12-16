-- name: CreateUser :one
INSERT INTO users (id, email, name, auth_provider, auth_subject, is_system_admin, password_hash, password_version)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: GetUserByProviderSubject :one
SELECT * FROM users WHERE auth_provider = $1 AND auth_subject = $2;

-- name: UpdateUserProfile :one
UPDATE users
SET
    name = $2,
    theme_preference = $3,
    default_tenant_id = $4,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: AdminCountUsers :one
SELECT COUNT(*) FROM users
WHERE ($1 = '' OR email ILIKE '%' || $1 || '%' OR name ILIKE '%' || $1 || '%');

-- name: AdminListUsers :many
SELECT * FROM users
WHERE ($1 = '' OR email ILIKE '%' || $1 || '%' OR name ILIKE '%' || $1 || '%')
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: AdminUpdateUser :one
UPDATE users
SET
    name = $2,
    is_system_admin = $3,
    is_disabled = $4,
    disabled_at = $5,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: AdminSetUserPassword :one
UPDATE users
SET
    password_hash = $2,
    password_version = $3,
    updated_at = NOW()
WHERE id = $1
RETURNING *;
