package http

import (
	"database/sql"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"raito/internal/store"
)

type adminUsageResponse struct {
	Success         bool             `json:"success"`
	Code            string           `json:"code,omitempty"`
	Error           string           `json:"error,omitempty"`
	ScopeType       string           `json:"scopeType,omitempty"` // "all" | "tenant" | "user"
	ScopeTenantID   *string          `json:"scopeTenantId,omitempty"`
	ScopeUserID     *string          `json:"scopeUserId,omitempty"`
	Jobs            int64            `json:"jobs"`
	Documents       int64            `json:"documents"`
	Users           int64            `json:"users"`
	Tenants         int64            `json:"tenants"`
	TenantsByType   map[string]int64 `json:"tenantsByType,omitempty"`
	JobsByType      map[string]int64 `json:"jobsByType,omitempty"`
	DocumentsByType map[string]int64 `json:"documentsByType,omitempty"`
}

func adminUsageHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)

	ctx := c.Context()

	// Optional since/window filter for time-bounded stats.
	var sinceParam any
	if s := c.Query("since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			sinceParam = t
		}
	} else if w := c.Query("window"); w != "" {
		now := time.Now().UTC()
		switch w {
		case "24h":
			sinceParam = now.Add(-24 * time.Hour)
		case "7d":
			sinceParam = now.Add(-7 * 24 * time.Hour)
		case "30d":
			sinceParam = now.Add(-30 * 24 * time.Hour)
		}
	}

	rawTenantID := c.Query("tenantId")
	rawUserID := c.Query("userId")

	scopeType := "all"
	var scopeTenantID *uuid.UUID
	var scopeUserID *uuid.UUID
	if rawTenantID != "" {
		tid, err := uuid.Parse(rawTenantID)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(adminUsageResponse{
				Success: false,
				Code:    "BAD_REQUEST",
				Error:   "invalid tenantId",
			})
		}
		scopeType = "tenant"
		scopeTenantID = &tid
	} else if rawUserID != "" {
		uid, err := uuid.Parse(rawUserID)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(adminUsageResponse{
				Success: false,
				Code:    "BAD_REQUEST",
				Error:   "invalid userId",
			})
		}
		scopeType = "user"
		scopeUserID = &uid

		// For user scope, resolve the user's personal tenant.
		var personalTenantID uuid.UUID
		err = st.DB.QueryRowContext(
			ctx,
			"SELECT id FROM tenants WHERE owner_user_id = $1 AND type = 'personal' ORDER BY created_at ASC LIMIT 1",
			uid,
		).Scan(&personalTenantID)
		if err != nil {
			if err == sql.ErrNoRows {
				return c.Status(fiber.StatusNotFound).JSON(adminUsageResponse{
					Success: false,
					Code:    "NOT_FOUND",
					Error:   "personal tenant not found for user",
				})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(adminUsageResponse{
				Success: false,
				Code:    "USAGE_QUERY_FAILED",
				Error:   err.Error(),
			})
		}
		scopeTenantID = &personalTenantID
	}

	// Users + tenants (all-time) (only shown for global scope in the UI).
	var usersCount int64
	var tenantsCount int64
	tenantsByType := make(map[string]int64)
	if scopeType == "all" {
		if err := st.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&usersCount); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(adminUsageResponse{
				Success: false,
				Code:    "USAGE_QUERY_FAILED",
				Error:   err.Error(),
			})
		}

		if err := st.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM tenants").Scan(&tenantsCount); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(adminUsageResponse{
				Success: false,
				Code:    "USAGE_QUERY_FAILED",
				Error:   err.Error(),
			})
		}

		rows, err := st.DB.QueryContext(ctx, "SELECT type, COUNT(*) FROM tenants GROUP BY type")
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var tenantType string
				var cnt int64
				if err := rows.Scan(&tenantType, &cnt); err != nil {
					continue
				}
				tenantsByType[tenantType] = cnt
			}
		}
	}

	// Count jobs.
	var jobsCount int64
	if scopeTenantID != nil {
		jobsQuery := "SELECT COUNT(*) FROM jobs WHERE tenant_id = $1"
		if sinceParam != nil {
			jobsQuery = jobsQuery + " AND created_at >= $2"
			if err := st.DB.QueryRowContext(ctx, jobsQuery, *scopeTenantID, sinceParam).Scan(&jobsCount); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(adminUsageResponse{
					Success: false,
					Code:    "USAGE_QUERY_FAILED",
					Error:   err.Error(),
				})
			}
		} else {
			if err := st.DB.QueryRowContext(ctx, jobsQuery, *scopeTenantID).Scan(&jobsCount); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(adminUsageResponse{
					Success: false,
					Code:    "USAGE_QUERY_FAILED",
					Error:   err.Error(),
				})
			}
		}
	} else {
		jobsQuery := "SELECT COUNT(*) FROM jobs"
		args := []any{}
		if sinceParam != nil {
			jobsQuery = jobsQuery + " WHERE created_at >= $1"
			args = append(args, sinceParam)
		}
		if err := st.DB.QueryRowContext(ctx, jobsQuery, args...).Scan(&jobsCount); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(adminUsageResponse{
				Success: false,
				Code:    "USAGE_QUERY_FAILED",
				Error:   err.Error(),
			})
		}
	}

	// Jobs by type.
	jobsByType := make(map[string]int64)
	var rows *sql.Rows
	var err error
	if scopeTenantID != nil {
		jobsByTypeQuery := "SELECT type, COUNT(*) FROM jobs WHERE tenant_id = $1"
		if sinceParam != nil {
			jobsByTypeQuery = jobsByTypeQuery + " AND created_at >= $2"
			rows, err = st.DB.QueryContext(ctx, jobsByTypeQuery+" GROUP BY type", *scopeTenantID, sinceParam)
		} else {
			rows, err = st.DB.QueryContext(ctx, jobsByTypeQuery+" GROUP BY type", *scopeTenantID)
		}
	} else {
		jobsByTypeQuery := "SELECT type, COUNT(*) FROM jobs"
		args := []any{}
		if sinceParam != nil {
			jobsByTypeQuery = jobsByTypeQuery + " WHERE created_at >= $1"
			args = append(args, sinceParam)
		}
		jobsByTypeQuery = jobsByTypeQuery + " GROUP BY type"
		rows, err = st.DB.QueryContext(ctx, jobsByTypeQuery, args...)
	}
	if err == nil && rows != nil {
		defer rows.Close()
		for rows.Next() {
			var jobType string
			var cnt int64
			if err := rows.Scan(&jobType, &cnt); err != nil {
				continue
			}
			jobsByType[jobType] = cnt
		}
	}

	// Count documents (via documents table + output-based results).
	var docsCount int64
	if scopeTenantID != nil {
		docsQuery := "SELECT COUNT(*) FROM documents d JOIN jobs j ON d.job_id = j.id WHERE j.tenant_id = $1"
		args := []any{*scopeTenantID}
		if sinceParam != nil {
			docsQuery = docsQuery + " AND j.created_at >= $2"
			args = append(args, sinceParam)
		}
		if err := st.DB.QueryRowContext(ctx, docsQuery, args...).Scan(&docsCount); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(adminUsageResponse{
				Success: false,
				Code:    "USAGE_QUERY_FAILED",
				Error:   err.Error(),
			})
		}
	} else {
		docsQuery := "SELECT COUNT(*) FROM documents d JOIN jobs j ON d.job_id = j.id"
		args := []any{}
		if sinceParam != nil {
			docsQuery = docsQuery + " WHERE j.created_at >= $1"
			args = append(args, sinceParam)
		}
		if err := st.DB.QueryRowContext(ctx, docsQuery, args...).Scan(&docsCount); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(adminUsageResponse{
				Success: false,
				Code:    "USAGE_QUERY_FAILED",
				Error:   err.Error(),
			})
		}
	}

	// Count scrape/map jobs with persisted output as one document each.
	var outputJobCount int64
	if scopeTenantID != nil {
		outputJobQuery := "SELECT COUNT(*) FROM jobs WHERE tenant_id = $1 AND type IN ('scrape','map') AND output IS NOT NULL"
		if sinceParam != nil {
			outputJobQuery = outputJobQuery + " AND created_at >= $2"
			if err := st.DB.QueryRowContext(ctx, outputJobQuery, *scopeTenantID, sinceParam).Scan(&outputJobCount); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(adminUsageResponse{
					Success: false,
					Code:    "USAGE_QUERY_FAILED",
					Error:   err.Error(),
				})
			}
		} else {
			if err := st.DB.QueryRowContext(ctx, outputJobQuery, *scopeTenantID).Scan(&outputJobCount); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(adminUsageResponse{
					Success: false,
					Code:    "USAGE_QUERY_FAILED",
					Error:   err.Error(),
				})
			}
		}
	} else {
		outputJobQuery := "SELECT COUNT(*) FROM jobs WHERE type IN ('scrape','map') AND output IS NOT NULL"
		if sinceParam != nil {
			outputJobQuery = outputJobQuery + " AND created_at >= $1"
			if err := st.DB.QueryRowContext(ctx, outputJobQuery, sinceParam).Scan(&outputJobCount); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(adminUsageResponse{
					Success: false,
					Code:    "USAGE_QUERY_FAILED",
					Error:   err.Error(),
				})
			}
		} else {
			if err := st.DB.QueryRowContext(ctx, outputJobQuery).Scan(&outputJobCount); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(adminUsageResponse{
					Success: false,
					Code:    "USAGE_QUERY_FAILED",
					Error:   err.Error(),
				})
			}
		}
	}

	// Extract jobs store an object with `results: []` in jobs.output; count each element as a document.
	var extractResultCount int64
	if scopeTenantID != nil {
		extractCountQuery := "SELECT COALESCE(SUM(jsonb_array_length(output->'results')), 0) FROM jobs WHERE tenant_id = $1 AND type = 'extract' AND output IS NOT NULL"
		if sinceParam != nil {
			extractCountQuery = extractCountQuery + " AND created_at >= $2"
			if err := st.DB.QueryRowContext(ctx, extractCountQuery, *scopeTenantID, sinceParam).Scan(&extractResultCount); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(adminUsageResponse{
					Success: false,
					Code:    "USAGE_QUERY_FAILED",
					Error:   err.Error(),
				})
			}
		} else {
			if err := st.DB.QueryRowContext(ctx, extractCountQuery, *scopeTenantID).Scan(&extractResultCount); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(adminUsageResponse{
					Success: false,
					Code:    "USAGE_QUERY_FAILED",
					Error:   err.Error(),
				})
			}
		}
	} else {
		extractCountQuery := "SELECT COALESCE(SUM(jsonb_array_length(output->'results')), 0) FROM jobs WHERE type = 'extract' AND output IS NOT NULL"
		if sinceParam != nil {
			extractCountQuery = extractCountQuery + " AND created_at >= $1"
			if err := st.DB.QueryRowContext(ctx, extractCountQuery, sinceParam).Scan(&extractResultCount); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(adminUsageResponse{
					Success: false,
					Code:    "USAGE_QUERY_FAILED",
					Error:   err.Error(),
				})
			}
		} else {
			if err := st.DB.QueryRowContext(ctx, extractCountQuery).Scan(&extractResultCount); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(adminUsageResponse{
					Success: false,
					Code:    "USAGE_QUERY_FAILED",
					Error:   err.Error(),
				})
			}
		}
	}

	docsCount = docsCount + outputJobCount + extractResultCount

	// Documents by job type (documents table), then augment with output-based results.
	docsByType := make(map[string]int64)
	if scopeTenantID != nil {
		docsByTypeQuery := "SELECT j.type, COUNT(*) FROM documents d JOIN jobs j ON d.job_id = j.id WHERE j.tenant_id = $1"
		args := []any{*scopeTenantID}
		if sinceParam != nil {
			docsByTypeQuery = docsByTypeQuery + " AND j.created_at >= $2"
			args = append(args, sinceParam)
		}
		docsByTypeQuery = docsByTypeQuery + " GROUP BY j.type"
		rows, err = st.DB.QueryContext(ctx, docsByTypeQuery, args...)
	} else {
		docsByTypeQuery := "SELECT j.type, COUNT(*) FROM documents d JOIN jobs j ON d.job_id = j.id"
		args := []any{}
		if sinceParam != nil {
			docsByTypeQuery = docsByTypeQuery + " WHERE j.created_at >= $1"
			args = append(args, sinceParam)
		}
		docsByTypeQuery = docsByTypeQuery + " GROUP BY j.type"
		rows, err = st.DB.QueryContext(ctx, docsByTypeQuery, args...)
	}
	if err == nil && rows != nil {
		defer rows.Close()
		for rows.Next() {
			var jobType string
			var cnt int64
			if err := rows.Scan(&jobType, &cnt); err != nil {
				continue
			}
			docsByType[jobType] = cnt
		}
	}

	if outputJobCount > 0 {
		var scrapeCount int64
		scrapeQuery := "SELECT COUNT(*) FROM jobs WHERE type = 'scrape' AND output IS NOT NULL"
		args := []any{}
		if scopeTenantID != nil {
			scrapeQuery = "SELECT COUNT(*) FROM jobs WHERE tenant_id = $1 AND type = 'scrape' AND output IS NOT NULL"
			args = append(args, *scopeTenantID)
		}
		if sinceParam != nil {
			if scopeTenantID != nil {
				scrapeQuery = scrapeQuery + " AND created_at >= $2"
				args = append(args, sinceParam)
			} else {
				scrapeQuery = scrapeQuery + " AND created_at >= $1"
				args = append(args, sinceParam)
			}
		} else {
			// nothing
		}
		_ = st.DB.QueryRowContext(ctx, scrapeQuery, args...).Scan(&scrapeCount)
		if scrapeCount > 0 {
			docsByType["scrape"] = docsByType["scrape"] + scrapeCount
		}

		var mapCount int64
		mapQuery := "SELECT COUNT(*) FROM jobs WHERE type = 'map' AND output IS NOT NULL"
		args = []any{}
		if scopeTenantID != nil {
			mapQuery = "SELECT COUNT(*) FROM jobs WHERE tenant_id = $1 AND type = 'map' AND output IS NOT NULL"
			args = append(args, *scopeTenantID)
		}
		if sinceParam != nil {
			if scopeTenantID != nil {
				mapQuery = mapQuery + " AND created_at >= $2"
				args = append(args, sinceParam)
			} else {
				mapQuery = mapQuery + " AND created_at >= $1"
				args = append(args, sinceParam)
			}
		} else {
			// nothing
		}
		_ = st.DB.QueryRowContext(ctx, mapQuery, args...).Scan(&mapCount)
		if mapCount > 0 {
			docsByType["map"] = docsByType["map"] + mapCount
		}
	}

	if extractResultCount > 0 {
		docsByType["extract"] = docsByType["extract"] + extractResultCount
	}

	var scopeTenantIDStr *string
	if scopeTenantID != nil {
		s := scopeTenantID.String()
		scopeTenantIDStr = &s
	}
	var scopeUserIDStr *string
	if scopeUserID != nil {
		s := scopeUserID.String()
		scopeUserIDStr = &s
	}

	return c.Status(fiber.StatusOK).JSON(adminUsageResponse{
		Success:         true,
		ScopeType:       scopeType,
		ScopeTenantID:   scopeTenantIDStr,
		ScopeUserID:     scopeUserIDStr,
		Jobs:            jobsCount,
		Documents:       docsCount,
		Users:           usersCount,
		Tenants:         tenantsCount,
		TenantsByType:   tenantsByType,
		JobsByType:      jobsByType,
		DocumentsByType: docsByType,
	})
}
