package http

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"raito/internal/config"
	"raito/internal/db"
	"raito/internal/jobs"
	"raito/internal/store"
)

type createAPIKeyRequest struct {
	Label              string `json:"label"`
	RateLimitPerMinute *int   `json:"rateLimitPerMinute,omitempty"`
}

type createAPIKeyResponse struct {
	Success bool   `json:"success"`
	Key     string `json:"key"`
}

// AdminJob describes a single job row for admin inspection.
type AdminJob struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Status      string          `json:"status"`
	URL         string          `json:"url"`
	Sync        bool            `json:"sync"`
	Priority    int32           `json:"priority"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
	CompletedAt *time.Time      `json:"completedAt,omitempty"`
	TenantID    string          `json:"tenantId,omitempty"`
	Error       string          `json:"error,omitempty"`
	Output      json.RawMessage `json:"output,omitempty"`
}

type adminJobResponse struct {
	Success bool     `json:"success"`
	Job     AdminJob `json:"job"`
}

type adminJobsResponse struct {
	Success bool       `json:"success"`
	Jobs    []AdminJob `json:"jobs"`
}

type adminRetentionResponse struct {
	Success          bool             `json:"success"`
	JobsDeleted      map[string]int64 `json:"jobsDeleted"`
	DocumentsDeleted int64            `json:"documentsDeleted"`
}

// registerAdminRoutes registers admin-only endpoints under /admin.
func registerAdminRoutes(group fiber.Router) {
	group.Post("/api-keys", adminCreateAPIKeyHandler)
	group.Get("/api-keys", adminListAPIKeysHandler)
	group.Delete("/api-keys/:id", adminRevokeAPIKeyHandler)
	group.Get("/usage", adminUsageHandler)

	group.Post("/users", adminCreateUserHandler)
	group.Get("/users", adminListUsersHandler)
	group.Get("/users/:id", adminGetUserHandler)
	group.Patch("/users/:id", adminUpdateUserHandler)
	group.Post("/users/:id/reset-password", adminResetUserPasswordHandler)

	group.Post("/tenants", adminCreateTenantHandler)
	group.Get("/tenants", adminListTenantsHandler)
	group.Get("/tenants/:id", adminGetTenantHandler)
	group.Patch("/tenants/:id", adminUpdateTenantHandler)
	group.Get("/tenants/:id/members", adminListTenantMembersHandler)
	group.Post("/tenants/:id/members", adminAddTenantMemberHandler)
	group.Patch("/tenants/:id/members/:userID", adminUpdateTenantMemberHandler)
	group.Delete("/tenants/:id/members/:userID", adminRemoveTenantMemberHandler)

	group.Get("/jobs/:id", adminGetJobHandler)
	group.Get("/jobs", adminListJobsHandler)
	group.Post("/retention/cleanup", adminRetentionCleanupHandler)
}

// adminCreateAPIKeyHandler creates a new user API key and returns the raw key once.
func adminCreateAPIKeyHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)

	var req createAPIKeyRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST_INVALID_JSON",
			Error:   "Bad request, malformed JSON",
		})
	}

	if req.Label == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "label is required",
		})
	}

	// For now, always create non-admin keys via this endpoint.
	rawKey, _, err := st.CreateRandomAPIKey(c.Context(), req.Label, false, req.RateLimitPerMinute, nil)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "API_KEY_CREATE_FAILED",
			Error:   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(createAPIKeyResponse{
		Success: true,
		Key:     rawKey,
	})
}

// adminRetentionCleanupHandler triggers a TTL cleanup run and returns
// the number of jobs/documents deleted in this run.
func adminRetentionCleanupHandler(c *fiber.Ctx) error {
	cfg := c.Locals("config").(*config.Config)
	st := c.Locals("store").(*store.Store)

	if !cfg.Retention.Enabled {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "RETENTION_DISABLED",
			Error:   "retention is disabled in server configuration",
		})
	}

	stats := jobs.CleanupExpiredData(c.Context(), cfg, st)

	return c.Status(fiber.StatusOK).JSON(adminRetentionResponse{
		Success:          true,
		JobsDeleted:      stats.JobsDeleted,
		DocumentsDeleted: stats.DocumentsDeleted,
	})
}

// adminGetJobHandler returns details for a single job by ID.
func adminGetJobHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)

	rawID := c.Params("id")
	jobID, err := uuid.Parse(rawID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "invalid job id",
		})
	}

	job, err := st.GetJobByID(c.Context(), jobID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Success: false,
				Code:    "NOT_FOUND",
				Error:   "job not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "JOB_LOOKUP_FAILED",
			Error:   err.Error(),
		})
	}

	respJob := marshalAdminJob(job, true)

	return c.Status(fiber.StatusOK).JSON(adminJobResponse{
		Success: true,
		Job:     respJob,
	})
}

// adminListJobsHandler lists recent jobs with optional filters.
func adminListJobsHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)

	jobType := c.Query("type")
	status := c.Query("status")

	var tenantID *uuid.UUID
	if v := c.Query("tenantId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Success: false,
				Code:    "BAD_REQUEST",
				Error:   "invalid tenantId value",
			})
		}
		tenantID = &id
	}

	var syncFilter *bool
	if syncStr := c.Query("sync"); syncStr != "" {
		val, err := strconv.ParseBool(syncStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
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
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
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
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
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
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "JOB_LIST_FAILED",
			Error:   err.Error(),
		})
	}

	out := make([]AdminJob, 0, len(jobs))
	for _, job := range jobs {
		out = append(out, marshalAdminJob(job, false))
	}

	return c.Status(fiber.StatusOK).JSON(adminJobsResponse{
		Success: true,
		Jobs:    out,
	})
}

// marshalAdminJob converts a db.Job into an AdminJob. When includeOutput is false,
// the output field is omitted to keep list responses lightweight.
func marshalAdminJob(job db.Job, includeOutput bool) AdminJob {
	var completedAt *time.Time
	if job.CompletedAt.Valid {
		t := job.CompletedAt.Time
		completedAt = &t
	}

	var output json.RawMessage
	if includeOutput && job.Output.Valid && len(job.Output.RawMessage) > 0 {
		output = job.Output.RawMessage
	}

	var errMsg string
	if job.Error.Valid {
		errMsg = job.Error.String
	}

	var tenantID string
	if job.TenantID.Valid {
		tenantID = job.TenantID.UUID.String()
	}

	return AdminJob{
		ID:          job.ID.String(),
		Type:        job.Type,
		Status:      job.Status,
		URL:         job.Url,
		Sync:        job.Sync,
		Priority:    job.Priority,
		CreatedAt:   job.CreatedAt,
		UpdatedAt:   job.UpdatedAt,
		CompletedAt: completedAt,
		TenantID:    tenantID,
		Error:       errMsg,
		Output:      output,
	}
}
