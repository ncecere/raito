-- name: InsertJob :one
INSERT INTO jobs (id, type, status, url, input, sync, priority, tenant_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, type, status, url, input, error, created_at, updated_at, completed_at, sync, priority, output, tenant_id;

-- name: UpdateJobStatus :exec
UPDATE jobs
SET status = $2,
    error = $3,
    updated_at = NOW(),
    completed_at = CASE WHEN $2 IN ('completed', 'failed') THEN NOW() ELSE completed_at END
WHERE id = $1;

-- name: GetJobByID :one
SELECT id, type, status, url, input, error, created_at, updated_at, completed_at, sync, priority, output, tenant_id
FROM jobs
WHERE id = $1;

-- name: ListPendingJobs :many
SELECT id, type, status, url, input, error, created_at, updated_at, completed_at, sync, priority, output, tenant_id
FROM jobs
WHERE status = 'pending'
ORDER BY priority DESC, created_at ASC
LIMIT $1;

-- name: UpdateJobOutput :exec
UPDATE jobs
SET output = $2,
    updated_at = NOW()
WHERE id = $1;
