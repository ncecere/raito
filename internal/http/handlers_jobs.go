package http

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

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
	UpdatedAt   time.Time  `json:"updatedAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

type JobDetailItem struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	Status      string     `json:"status"`
	URL         string     `json:"url"`
	Sync        bool       `json:"sync"`
	Priority    int32      `json:"priority"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	Error       string     `json:"error,omitempty"`
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

	var tenantID *uuid.UUID
	if p.IsSystemAdmin {
		if v := c.Query("tenantId"); v != "" {
			id, err := uuid.Parse(v)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(ListJobsResponse{
					Success: false,
					Code:    "BAD_REQUEST",
					Error:   "invalid tenantId value",
				})
			}
			tenantID = &id
		}
	} else {
		if p.TenantID == nil {
			return c.Status(fiber.StatusBadRequest).JSON(ListJobsResponse{
				Success: false,
				Code:    "BAD_REQUEST",
				Error:   "tenant context is required to list jobs",
			})
		}
		tenantID = p.TenantID
	}

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

	items := make([]JobItem, 0, len(jobs))
	for _, job := range jobs {
		var completedAt *time.Time
		if job.CompletedAt.Valid {
			t := job.CompletedAt.Time
			completedAt = &t
		}
		items = append(items, JobItem{
			ID:          job.ID.String(),
			Type:        job.Type,
			Status:      job.Status,
			URL:         job.Url,
			Sync:        job.Sync,
			Priority:    job.Priority,
			CreatedAt:   job.CreatedAt,
			UpdatedAt:   job.UpdatedAt,
			CompletedAt: completedAt,
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

	// Enforce tenant scoping for non-admin callers when the job has a tenant.
	if !p.IsSystemAdmin && job.TenantID.Valid {
		if p.TenantID == nil || job.TenantID.UUID != *p.TenantID {
			return c.Status(fiber.StatusNotFound).JSON(JobDetailResponse{
				Success: false,
				Code:    "NOT_FOUND",
				Error:   "job not found",
			})
		}
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

	detail := &JobDetailItem{
		ID:          job.ID.String(),
		Type:        job.Type,
		Status:      job.Status,
		URL:         job.Url,
		Sync:        job.Sync,
		Priority:    job.Priority,
		CreatedAt:   job.CreatedAt,
		UpdatedAt:   job.UpdatedAt,
		CompletedAt: completedAt,
		Error:       errMsg,
	}

	return c.Status(fiber.StatusOK).JSON(JobDetailResponse{
		Success: true,
		Job:     detail,
	})
}
