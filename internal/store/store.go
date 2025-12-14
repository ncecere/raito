package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/sqlc-dev/pqtype"

	"raito/internal/db"
)

// Store wraps access to the database via sqlc-generated Queries.
type Store struct {
	DB *sql.DB
}

// hashAPIKey hashes a raw API key string using SHA-256 and returns a hex string.
func hashAPIKey(raw string) string {

	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// New creates a new Store that uses a shared *sql.DB with pooling.
func New(database *sql.DB) *Store {
	return &Store{DB: database}
}

// withQueries constructs a sqlc Queries wrapper on the shared *sql.DB and
// executes the callback.
func (s *Store) withQueries(ctx context.Context, fn func(ctx context.Context, q *db.Queries) error) error {
	q := db.New(s.DB)
	return fn(ctx, q)
}

// CreateJob inserts a new job row with the given parameters.
func (s *Store) CreateJob(ctx context.Context, id uuid.UUID, jobType, url string, input any, sync bool, priority int32, tenantID, apiKeyID *uuid.UUID) (db.Job, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return db.Job{}, err
	}

	var job db.Job
	err = s.withQueries(ctx, func(ctx context.Context, q *db.Queries) error {
		var t uuid.NullUUID
		if tenantID != nil {
			t = uuid.NullUUID{UUID: *tenantID, Valid: true}
		}
		var k uuid.NullUUID
		if apiKeyID != nil {
			k = uuid.NullUUID{UUID: *apiKeyID, Valid: true}
		}
		row, err := q.InsertJob(ctx, db.InsertJobParams{
			ID:       id,
			Type:     jobType,
			Status:   "pending",
			Url:      url,
			Input:    payload,
			Sync:     sync,
			Priority: priority,
			TenantID: t,
			ApiKeyID: k,
		})
		if err != nil {
			return err
		}

		job = db.Job{
			ID:          row.ID,
			Type:        row.Type,
			Status:      row.Status,
			Url:         row.Url,
			Input:       row.Input,
			Error:       row.Error,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
			CompletedAt: row.CompletedAt,
			Priority:    row.Priority,
			Sync:        row.Sync,
			Output:      row.Output,
			TenantID:    row.TenantID,
			ApiKeyID:    row.ApiKeyID,
		}
		return nil
	})

	return job, err
}

// CreateCrawlJob inserts a new crawl job row.
func (s *Store) CreateCrawlJob(ctx context.Context, id uuid.UUID, url string, input any, tenantID, apiKeyID *uuid.UUID) (db.Job, error) {
	return s.CreateJob(ctx, id, "crawl", url, input, false, 10, tenantID, apiKeyID)
}

// UpdateCrawlJobStatus updates the status and optional error message for a crawl job.
func (s *Store) UpdateCrawlJobStatus(ctx context.Context, id uuid.UUID, status string, errMsg *string) error {
	var sqlErr sql.NullString
	if errMsg != nil {
		sqlErr = sql.NullString{String: *errMsg, Valid: true}
	}

	return s.withQueries(ctx, func(ctx context.Context, q *db.Queries) error {
		return q.UpdateJobStatus(ctx, db.UpdateJobStatusParams{
			ID:     id,
			Status: status,
			Error:  sqlErr,
		})
	})
}

// AddDocument stores a scraped document row.
func (s *Store) AddDocument(ctx context.Context, jobID uuid.UUID, url string, markdown, html, rawHTML *string, metadata json.RawMessage, statusCode *int32, engine *string) error {
	var m, h, r sql.NullString
	if markdown != nil {
		m = sql.NullString{String: *markdown, Valid: true}
	}
	if html != nil {
		h = sql.NullString{String: *html, Valid: true}
	}
	if rawHTML != nil {
		r = sql.NullString{String: *rawHTML, Valid: true}
	}
	var sc sql.NullInt32
	if statusCode != nil {
		sc = sql.NullInt32{Int32: *statusCode, Valid: true}
	}
	var eng sql.NullString
	if engine != nil {
		eng = sql.NullString{String: *engine, Valid: true}
	}

	return s.withQueries(ctx, func(ctx context.Context, q *db.Queries) error {
		return q.InsertDocument(ctx, db.InsertDocumentParams{
			JobID:      jobID,
			Url:        url,
			Markdown:   m,
			Html:       h,
			RawHtml:    r,
			Metadata:   metadata,
			StatusCode: sc,
			Engine:     eng,
		})
	})
}

// GetCrawlJobAndDocuments fetches a job and all associated documents.
func (s *Store) GetCrawlJobAndDocuments(ctx context.Context, id uuid.UUID) (db.Job, []db.Document, error) {
	var job db.Job
	var docs []db.Document

	err := s.withQueries(ctx, func(ctx context.Context, q *db.Queries) error {
		row, err := q.GetJobByID(ctx, id)
		if err != nil {
			return err
		}

		job = db.Job{
			ID:          row.ID,
			Type:        row.Type,
			Status:      row.Status,
			Url:         row.Url,
			Input:       row.Input,
			Error:       row.Error,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
			CompletedAt: row.CompletedAt,
			Priority:    row.Priority,
			Sync:        row.Sync,
			Output:      row.Output,
			TenantID:    row.TenantID,
			ApiKeyID:    row.ApiKeyID,
		}

		docs, err = q.GetDocumentsByJobID(ctx, id)
		return err
	})

	if err != nil {
		return db.Job{}, nil, err
	}
	return job, docs, nil
}

// ListPendingJobs returns up to `limit` jobs that are still pending,
// ordered by priority (desc) and created_at (asc).
func (s *Store) ListPendingJobs(ctx context.Context, limit int32) ([]db.Job, error) {
	var jobs []db.Job

	err := s.withQueries(ctx, func(ctx context.Context, q *db.Queries) error {
		rows, err := q.ListPendingJobs(ctx, limit)
		if err != nil {
			return err
		}

		jobs = make([]db.Job, 0, len(rows))
		for _, row := range rows {
			jobs = append(jobs, db.Job{
				ID:          row.ID,
				Type:        row.Type,
				Status:      row.Status,
				Url:         row.Url,
				Input:       row.Input,
				Error:       row.Error,
				CreatedAt:   row.CreatedAt,
				UpdatedAt:   row.UpdatedAt,
				CompletedAt: row.CompletedAt,
				Priority:    row.Priority,
				Sync:        row.Sync,
				Output:      row.Output,
				TenantID:    row.TenantID,
				ApiKeyID:    row.ApiKeyID,
			})
		}
		return nil
	})

	return jobs, err
}

// JobListFilter describes optional filters for listing jobs in admin APIs.
type JobListFilter struct {
	Type     string
	Status   string
	Sync     *bool
	TenantID *uuid.UUID
	Limit    int32
	Offset   int32
}

// ListJobs returns jobs matching the given filter, ordered by created_at desc.
func (s *Store) ListJobs(ctx context.Context, filter JobListFilter) ([]db.Job, error) {
	baseQuery := "SELECT id FROM jobs"
	var conditions []string
	var args []any
	argPos := 1

	if filter.Type != "" {
		conditions = append(conditions, fmt.Sprintf("type = $%d", argPos))
		args = append(args, filter.Type)
		argPos++
	}
	if filter.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argPos))
		args = append(args, filter.Status)
		argPos++
	}
	if filter.Sync != nil {
		conditions = append(conditions, fmt.Sprintf("sync = $%d", argPos))
		args = append(args, *filter.Sync)
		argPos++
	}
	if filter.TenantID != nil {
		conditions = append(conditions, fmt.Sprintf("tenant_id = $%d", argPos))
		args = append(args, *filter.TenantID)
		argPos++
	}

	if len(conditions) > 0 {
		baseQuery = baseQuery + " WHERE " + strings.Join(conditions, " AND ")
	}

	baseQuery = baseQuery + " ORDER BY created_at DESC"

	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	baseQuery = baseQuery + fmt.Sprintf(" LIMIT $%d", argPos)
	args = append(args, limit)
	argPos++

	if filter.Offset > 0 {
		baseQuery = baseQuery + fmt.Sprintf(" OFFSET $%d", argPos)
		args = append(args, filter.Offset)
	}

	rows, err := s.DB.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	jobs := make([]db.Job, 0, len(ids))
	for _, id := range ids {
		job, err := s.GetJobByID(ctx, id)
		if err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return nil, err
		}
		jobs = append(jobs, job)
	}

	return jobs, nil
}

// GetJobByID fetches a single job row by its ID.
func (s *Store) GetJobByID(ctx context.Context, id uuid.UUID) (db.Job, error) {
	var job db.Job

	err := s.withQueries(ctx, func(ctx context.Context, q *db.Queries) error {
		row, err := q.GetJobByID(ctx, id)
		if err != nil {
			return err
		}

		job = db.Job{
			ID:          row.ID,
			Type:        row.Type,
			Status:      row.Status,
			Url:         row.Url,
			Input:       row.Input,
			Error:       row.Error,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
			CompletedAt: row.CompletedAt,
			Priority:    row.Priority,
			Sync:        row.Sync,
			Output:      row.Output,
			TenantID:    row.TenantID,
			ApiKeyID:    row.ApiKeyID,
		}
		return nil
	})

	return job, err
}

// SetJobOutput updates the output JSON for a job.
func (s *Store) SetJobOutput(ctx context.Context, id uuid.UUID, output json.RawMessage) error {
	return s.withQueries(ctx, func(ctx context.Context, q *db.Queries) error {
		return q.UpdateJobOutput(ctx, db.UpdateJobOutputParams{
			ID: id,
			Output: pqtype.NullRawMessage{
				RawMessage: output,
				Valid:      len(output) > 0,
			},
		})
	})
}

// DeleteExpiredDocuments deletes documents older than the given cutoff timestamp.
func (s *Store) DeleteExpiredDocuments(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM documents WHERE created_at < $1`, cutoff)
	if err != nil {
		return 0, err
	}
	rows, _ := res.RowsAffected()
	return rows, nil
}

// DeleteExpiredJobsByType deletes jobs of the given type older than the cutoff.
func (s *Store) DeleteExpiredJobsByType(ctx context.Context, jobType string, cutoff time.Time) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM jobs WHERE type = $1 AND created_at < $2`, jobType, cutoff)
	if err != nil {
		return 0, err
	}
	rows, _ := res.RowsAffected()
	return rows, nil
}

// GetAPIKeyByRawKey looks up an API key by its raw value.
func (s *Store) GetAPIKeyByRawKey(ctx context.Context, rawKey string) (db.ApiKey, error) {
	hash := hashAPIKey(rawKey)
	var key db.ApiKey

	err := s.withQueries(ctx, func(ctx context.Context, q *db.Queries) error {
		var err error
		key, err = q.GetAPIKeyByHash(ctx, hash)
		return err
	})

	return key, err
}

// EnsureAdminAPIKey ensures that there is an admin API key for the given raw key and label.
// If it already exists, it is returned; otherwise, it is created.
func (s *Store) EnsureAdminAPIKey(ctx context.Context, rawKey, label string) (db.ApiKey, error) {
	hash := hashAPIKey(rawKey)
	var out db.ApiKey

	err := s.withQueries(ctx, func(ctx context.Context, q *db.Queries) error {
		// Try existing
		key, err := q.GetAPIKeyByHash(ctx, hash)
		if err == nil {
			out = key
			return nil
		}
		if err != nil && err != sql.ErrNoRows {
			return err
		}

		// Create new admin key
		id := uuid.New()
		key, err = q.InsertAPIKey(ctx, db.InsertAPIKeyParams{
			ID:                 id,
			KeyHash:            hash,
			Label:              label,
			IsAdmin:            true,
			RateLimitPerMinute: sql.NullInt32{},
			TenantID:           sql.NullString{},
		})
		if err != nil {
			return err
		}
		out = key
		return nil
	})

	return out, err
}

// CreateRandomAPIKey creates a new random API key (with raito_ prefix).
// It returns the raw key plus the stored record.
func (s *Store) CreateRandomAPIKey(ctx context.Context, label string, isAdmin bool, rateLimitPerMinute *int, tenantID *string) (string, db.ApiKey, error) {
	// Generate raw key
	raw := "raito_" + uuid.New().String()
	hash := hashAPIKey(raw)
	var out db.ApiKey

	err := s.withQueries(ctx, func(ctx context.Context, q *db.Queries) error {
		var rl sql.NullInt32
		if rateLimitPerMinute != nil && *rateLimitPerMinute > 0 {
			rl = sql.NullInt32{Int32: int32(*rateLimitPerMinute), Valid: true}
		}
		var tenant sql.NullString
		if tenantID != nil && *tenantID != "" {
			tenant = sql.NullString{String: *tenantID, Valid: true}
		}

		id := uuid.New()
		key, err := q.InsertAPIKey(ctx, db.InsertAPIKeyParams{
			ID:                 id,
			KeyHash:            hash,
			Label:              label,
			IsAdmin:            isAdmin,
			RateLimitPerMinute: rl,
			TenantID:           tenant,
		})
		if err != nil {
			return err
		}
		out = key
		return nil
	})

	return raw, out, err
}
