package http

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"raito/internal/config"
	"raito/internal/db"
	"raito/internal/store"
)

type JobItem struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	Status      string     `json:"status"`
	URL         string     `json:"url"`
	Sync        bool       `json:"sync"`
	Priority    int32      `json:"priority"`
	CreatedAt   time.Time  `json:"createdAt"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	APIKeyID    string     `json:"apiKeyId,omitempty"`
	APIKeyLabel string     `json:"apiKeyLabel,omitempty"`
}

type JobDetailItem struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	Status      string     `json:"status"`
	URL         string     `json:"url"`
	Formats     []string   `json:"formats,omitempty"`
	Sync        bool       `json:"sync"`
	Priority    int32      `json:"priority"`
	CreatedAt   time.Time  `json:"createdAt"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	Error       string     `json:"error,omitempty"`
	APIKeyID    string     `json:"apiKeyId,omitempty"`
	APIKeyLabel string     `json:"apiKeyLabel,omitempty"`
}

type ListJobsResponse struct {
	Success bool      `json:"success"`
	Code    string    `json:"code,omitempty"`
	Error   string    `json:"error,omitempty"`
	Jobs    []JobItem `json:"jobs,omitempty"`
}

type JobDetailResponse struct {
	Success bool           `json:"success"`
	Code    string         `json:"code,omitempty"`
	Error   string         `json:"error,omitempty"`
	Job     *JobDetailItem `json:"job,omitempty"`
}

// jobsListHandler lists jobs visible to the current principal.
// For non-admin users, results are scoped to the current tenant.
// System admins may optionally filter by tenantId via query param.
func jobsListHandler(c *fiber.Ctx) error {
	var cfg *config.Config
	if val := c.Locals("config"); val != nil {
		cfg, _ = val.(*config.Config)
	}
	st := c.Locals("store").(*store.Store)

	val := c.Locals("principal")
	p, ok := val.(Principal)
	if !ok || p.UserID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ListJobsResponse{
			Success: false,
			Code:    "UNAUTHENTICATED",
			Error:   "User context is not available for this request",
		})
	}

	jobType := c.Query("type")
	status := c.Query("status")

	// /v1/jobs is always scoped to the active tenant (even for system admins).
	// Cross-tenant inspection should use /admin endpoints.
	if p.TenantID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(ListJobsResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "tenant context is required to list jobs",
		})
	}
	tenantID := p.TenantID

	var syncFilter *bool
	if syncStr := c.Query("sync"); syncStr != "" {
		val, err := strconv.ParseBool(syncStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(ListJobsResponse{
				Success: false,
				Code:    "BAD_REQUEST",
				Error:   "invalid sync value; expected true or false",
			})
		}
		syncFilter = &val
	}

	limit := 50
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return c.Status(fiber.StatusBadRequest).JSON(ListJobsResponse{
				Success: false,
				Code:    "BAD_REQUEST",
				Error:   "invalid limit value",
			})
		}
		if n > 500 {
			n = 500
		}
		limit = n
	}

	offset := 0
	if v := c.Query("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return c.Status(fiber.StatusBadRequest).JSON(ListJobsResponse{
				Success: false,
				Code:    "BAD_REQUEST",
				Error:   "invalid offset value",
			})
		}
		offset = n
	}

	jobs, err := st.ListJobs(c.Context(), store.JobListFilter{
		Type:     jobType,
		Status:   status,
		Sync:     syncFilter,
		TenantID: tenantID,
		Limit:    int32(limit),
		Offset:   int32(offset),
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ListJobsResponse{
			Success: false,
			Code:    "JOB_LIST_FAILED",
			Error:   err.Error(),
		})
	}

	apiKeyLabels := map[uuid.UUID]string{}
	{
		ids := make([]uuid.UUID, 0, len(jobs))
		seen := make(map[uuid.UUID]struct{}, len(jobs))
		for _, job := range jobs {
			if job.ApiKeyID.Valid {
				id := job.ApiKeyID.UUID
				if _, ok := seen[id]; ok {
					continue
				}
				seen[id] = struct{}{}
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			q := db.New(st.DB)
			rows, err := q.GetAPIKeyLabelsByIDs(c.Context(), ids)
			if err == nil {
				for _, row := range rows {
					apiKeyLabels[row.ID] = row.Label
				}
			}
		}
	}

	items := make([]JobItem, 0, len(jobs))
	for _, job := range jobs {
		var completedAt *time.Time
		if job.CompletedAt.Valid {
			t := job.CompletedAt.Time
			completedAt = &t
		}
		expiresAt := computeJobExpiresAt(cfg, job.Type, job.CreatedAt)

		var apiKeyLabel string
		var apiKeyID string
		if job.ApiKeyID.Valid {
			apiKeyID = job.ApiKeyID.UUID.String()
			apiKeyLabel = apiKeyLabels[job.ApiKeyID.UUID]
		}
		items = append(items, JobItem{
			ID:          job.ID.String(),
			Type:        job.Type,
			Status:      job.Status,
			URL:         job.Url,
			Sync:        job.Sync,
			Priority:    job.Priority,
			CreatedAt:   job.CreatedAt,
			ExpiresAt:   expiresAt,
			UpdatedAt:   job.UpdatedAt,
			CompletedAt: completedAt,
			APIKeyID:    apiKeyID,
			APIKeyLabel: apiKeyLabel,
		})
	}

	return c.Status(fiber.StatusOK).JSON(ListJobsResponse{
		Success: true,
		Jobs:    items,
	})
}

// jobDetailHandler returns details for a single job visible to the current principal.
// Non-admin users are limited to jobs in their current tenant. Admins can see all jobs.
func jobDetailHandler(c *fiber.Ctx) error {
	var cfg *config.Config
	if val := c.Locals("config"); val != nil {
		cfg, _ = val.(*config.Config)
	}
	st := c.Locals("store").(*store.Store)

	val := c.Locals("principal")
	p, ok := val.(Principal)
	if !ok || p.UserID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(JobDetailResponse{
			Success: false,
			Code:    "UNAUTHENTICATED",
			Error:   "User context is not available for this request",
		})
	}

	// /v1/jobs/:id is always scoped to the active tenant (even for system admins).
	// Cross-tenant inspection should use /admin endpoints.
	if p.TenantID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(JobDetailResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "tenant context is required to view jobs",
		})
	}

	rawID := c.Params("id")
	jobID, err := uuid.Parse(rawID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(JobDetailResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "invalid job id",
		})
	}

	job, err := st.GetJobByID(c.Context(), jobID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(JobDetailResponse{
			Success: false,
			Code:    "NOT_FOUND",
			Error:   "job not found",
		})
	}

	// Enforce active-tenant scoping for all callers.
	if !job.TenantID.Valid || job.TenantID.UUID != *p.TenantID {
		return c.Status(fiber.StatusNotFound).JSON(JobDetailResponse{
			Success: false,
			Code:    "NOT_FOUND",
			Error:   "job not found",
		})
	}

	var completedAt *time.Time
	if job.CompletedAt.Valid {
		t := job.CompletedAt.Time
		completedAt = &t
	}

	var errMsg string
	if job.Error.Valid {
		errMsg = job.Error.String
	}

	var apiKeyLabel string
	var apiKeyID string
	if job.ApiKeyID.Valid {
		apiKeyID = job.ApiKeyID.UUID.String()
		q := db.New(st.DB)
		rows, err := q.GetAPIKeyLabelsByIDs(c.Context(), []uuid.UUID{job.ApiKeyID.UUID})
		if err == nil && len(rows) > 0 {
			apiKeyLabel = rows[0].Label
		}
	}

	expiresAt := computeJobExpiresAt(cfg, job.Type, job.CreatedAt)
	formats := formatsFromJobInput(job.Type, job.Input)

	detail := &JobDetailItem{
		ID:          job.ID.String(),
		Type:        job.Type,
		Status:      job.Status,
		URL:         job.Url,
		Formats:     formats,
		Sync:        job.Sync,
		Priority:    job.Priority,
		CreatedAt:   job.CreatedAt,
		ExpiresAt:   expiresAt,
		UpdatedAt:   job.UpdatedAt,
		CompletedAt: completedAt,
		Error:       errMsg,
		APIKeyID:    apiKeyID,
		APIKeyLabel: apiKeyLabel,
	}

	return c.Status(fiber.StatusOK).JSON(JobDetailResponse{
		Success: true,
		Job:     detail,
	})
}

func formatsFromJobInput(jobType string, input []byte) []string {
	switch jobType {
	case "scrape":
		var req ScrapeRequest
		if err := json.Unmarshal(input, &req); err != nil {
			return nil
		}
		formats := scrapeFormatNames(req.Formats)
		if len(formats) == 0 {
			return []string{"markdown"}
		}
		return formats
	case "crawl":
		var req CrawlRequest
		if err := json.Unmarshal(input, &req); err != nil {
			return nil
		}
		formats := scrapeFormatNames(req.Formats)
		if len(formats) == 0 {
			return []string{"markdown"}
		}
		return formats
	case "batch_scrape", "batch":
		var req BatchScrapeRequest
		if err := json.Unmarshal(input, &req); err != nil {
			return nil
		}
		formats := scrapeFormatNames(req.Formats)
		if len(formats) == 0 {
			return []string{"markdown"}
		}
		return formats
	case "extract":
		var req ExtractRequest
		if err := json.Unmarshal(input, &req); err != nil {
			return nil
		}
		if req.ScrapeOptions == nil {
			return nil
		}
		formats := scrapeFormatNames(req.ScrapeOptions.Formats)
		return formats
	default:
		return nil
	}
}

func computeJobExpiresAt(cfg *config.Config, jobType string, createdAt time.Time) *time.Time {
	if cfg == nil {
		return nil
	}

	ttl := cfg.Retention.Jobs
	days := ttl.DefaultDays
	switch jobType {
	case "scrape":
		if ttl.ScrapeDays > 0 {
			days = ttl.ScrapeDays
		}
	case "map":
		if ttl.MapDays > 0 {
			days = ttl.MapDays
		}
	case "extract":
		if ttl.ExtractDays > 0 {
			days = ttl.ExtractDays
		}
	case "crawl":
		if ttl.CrawlDays > 0 {
			days = ttl.CrawlDays
		}
	}

	if days <= 0 {
		return nil
	}

	expiresAt := createdAt.AddDate(0, 0, days)
	return &expiresAt
}
