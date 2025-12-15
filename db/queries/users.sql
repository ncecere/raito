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
