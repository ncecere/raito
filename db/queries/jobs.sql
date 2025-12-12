-- name: InsertJob :one
INSERT INTO jobs (id, type, status, url, input)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateJobStatus :exec
UPDATE jobs
SET status = $2,
    error = $3,
    updated_at = NOW(),
    completed_at = CASE WHEN $2 IN ('completed', 'failed') THEN NOW() ELSE completed_at END
WHERE id = $1;

-- name: GetJobByID :one
SELECT * FROM jobs WHERE id = $1;

-- name: ListPendingCrawlJobs :many
SELECT * FROM jobs
WHERE type = 'crawl' AND status = 'pending'
ORDER BY created_at ASC
LIMIT $1;
