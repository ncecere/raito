-- name: InsertDocument :exec
INSERT INTO documents (job_id, url, markdown, html, raw_html, metadata, status_code, engine)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: GetDocumentsByJobID :many
SELECT id, job_id, url, markdown, html, raw_html, metadata, status_code, created_at, engine FROM documents
WHERE job_id = $1
ORDER BY id ASC;
