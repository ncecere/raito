package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"

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

// CreateCrawlJob inserts a new crawl job row.
func (s *Store) CreateCrawlJob(ctx context.Context, id uuid.UUID, url string, input any) (db.Job, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return db.Job{}, err
	}

	var job db.Job
	err = s.withQueries(ctx, func(ctx context.Context, q *db.Queries) error {
		var err error
		job, err = q.InsertJob(ctx, db.InsertJobParams{
			ID:     id,
			Type:   "crawl",
			Status: "pending",
			Url:    url,
			Input:  payload,
		})
		return err
	})

	return job, err
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
func (s *Store) AddDocument(ctx context.Context, jobID uuid.UUID, url string, markdown, html, rawHTML *string, metadata json.RawMessage, statusCode *int32) error {
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

	return s.withQueries(ctx, func(ctx context.Context, q *db.Queries) error {
		return q.InsertDocument(ctx, db.InsertDocumentParams{
			JobID:      jobID,
			Url:        url,
			Markdown:   m,
			Html:       h,
			RawHtml:    r,
			Metadata:   metadata,
			StatusCode: sc,
		})
	})
}

// GetCrawlJobAndDocuments fetches a job and all associated documents.
func (s *Store) GetCrawlJobAndDocuments(ctx context.Context, id uuid.UUID) (db.Job, []db.Document, error) {
	var job db.Job
	var docs []db.Document

	err := s.withQueries(ctx, func(ctx context.Context, q *db.Queries) error {
		var err error
		job, err = q.GetJobByID(ctx, id)
		if err != nil {
			return err
		}
		docs, err = q.GetDocumentsByJobID(ctx, id)
		return err
	})

	if err != nil {
		return db.Job{}, nil, err
	}
	return job, docs, nil
}

// ListPendingCrawlJobs returns up to `limit` crawl jobs that are still pending.
func (s *Store) ListPendingCrawlJobs(ctx context.Context, limit int32) ([]db.Job, error) {
	var jobs []db.Job

	err := s.withQueries(ctx, func(ctx context.Context, q *db.Queries) error {
		var err error
		jobs, err = q.ListPendingCrawlJobs(ctx, limit)
		return err
	})

	return jobs, err
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
