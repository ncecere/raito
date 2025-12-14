package http

import (
	"database/sql"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"raito/internal/config"
	"raito/internal/db"
	"raito/internal/store"
)

type TenantItem struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
	Type string `json:"type"`
	Role string `json:"role"`
}

type ListTenantsResponse struct {
	Success bool         `json:"success"`
	Code    string       `json:"code,omitempty"`
	Error   string       `json:"error,omitempty"`
	Tenants []TenantItem `json:"tenants,omitempty"`
}

type SelectTenantResponse struct {
	Success bool        `json:"success"`
	Code    string      `json:"code,omitempty"`
	Error   string      `json:"error,omitempty"`
	Tenant  *TenantItem `json:"tenant,omitempty"`
}

type TenantUsageResponse struct {
	Success         bool             `json:"success"`
	Code            string           `json:"code,omitempty"`
	Error           string           `json:"error,omitempty"`
	Jobs            int64            `json:"jobs"`
	Documents       int64            `json:"documents"`
	JobsByType      map[string]int64 `json:"jobsByType,omitempty"`
	DocumentsByType map[string]int64 `json:"documentsByType,omitempty"`
}

type TenantMemberRequest struct {
	UserID string `json:"userId"`
	Role   string `json:"role"`
}

// listTenantsHandler lists all tenants the current user is a member of.
func listTenantsHandler(c *fiber.Ctx) error {
	cfg := c.Locals("config").(*config.Config)
	_ = cfg // reserved for future per-tenant overrides
	st := c.Locals("store").(*store.Store)

	val := c.Locals("principal")
	p, ok := val.(Principal)
	if !ok || p.UserID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ListTenantsResponse{
			Success: false,
			Code:    "UNAUTHENTICATED",
			Error:   "User context is not available for this request",
		})
	}

	userID := *p.UserID
	q := db.New(st.DB)

	tenants, err := q.ListTenantsForUser(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ListTenantsResponse{
			Success: false,
			Code:    "INTERNAL_ERROR",
			Error:   err.Error(),
		})
	}

	items := make([]TenantItem, 0, len(tenants))
	for _, t := range tenants {
		role := ""
		member, err := q.GetTenantMember(c.Context(), db.GetTenantMemberParams{
			TenantID: t.ID,
			UserID:   userID,
		})
		if err == nil {
			role = member.Role
		}
		items = append(items, TenantItem{
			ID:   t.ID.String(),
			Slug: t.Slug,
			Name: t.Name,
			Type: t.Type,
			Role: role,
		})
	}

	return c.Status(fiber.StatusOK).JSON(ListTenantsResponse{
		Success: true,
		Tenants: items,
	})
}

// selectTenantHandler switches the active/default tenant for the current
// session by re-issuing the session cookie with the chosen tenant ID.
func selectTenantHandler(c *fiber.Ctx) error {
	cfg := c.Locals("config").(*config.Config)
	st := c.Locals("store").(*store.Store)

	val := c.Locals("principal")
	p, ok := val.(Principal)
	if !ok || p.UserID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(SelectTenantResponse{
			Success: false,
			Code:    "UNAUTHENTICATED",
			Error:   "User context is not available for this request",
		})
	}

	tenantIDStr := c.Params("id")
	if tenantIDStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(SelectTenantResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "tenant id is required",
		})
	}

	// Enforce membership: only tenant members/admins (or system admins) may select.
	if err := RequireTenantMemberOrAdmin(c, p, tenantIDStr); err != nil {
		return err
	}

	tid, err := uuid.Parse(tenantIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(SelectTenantResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "invalid tenant id",
		})
	}

	q := db.New(st.DB)
	tenant, err := q.GetTenantByID(c.Context(), tid)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(SelectTenantResponse{
				Success: false,
				Code:    "NOT_FOUND",
				Error:   "tenant not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(SelectTenantResponse{
			Success: false,
			Code:    "INTERNAL_ERROR",
			Error:   err.Error(),
		})
	}

	// Re-issue session cookie with new tenant ID when sessions are enabled.
	_ = issueSessionCookie(c, cfg, *p.UserID, &tid, p.IsSystemAdmin)

	return c.Status(fiber.StatusOK).JSON(SelectTenantResponse{
		Success: true,
		Tenant: &TenantItem{
			ID:   tenant.ID.String(),
			Slug: tenant.Slug,
			Name: tenant.Name,
			Type: tenant.Type,
			Role: "", // role can be fetched from /v1/tenants if needed
		},
	})
}

// tenantUsageHandler returns basic usage metrics (jobs, documents) for a tenant.
// System admins can see any tenant; tenant members/admins can see their own tenant.
func tenantUsageHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)

	val := c.Locals("principal")
	p, ok := val.(Principal)
	if !ok || p.UserID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(TenantUsageResponse{
			Success: false,
			Code:    "UNAUTHENTICATED",
			Error:   "User context is not available for this request",
		})
	}

	rawID := c.Params("id")
	tenantID, err := uuid.Parse(rawID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(TenantUsageResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "invalid tenant id",
		})
	}

	// Enforce membership/admin for this tenant for non-system admins.
	if !p.IsSystemAdmin {
		if err := RequireTenantMemberOrAdmin(c, p, tenantID.String()); err != nil {
			return err
		}
	}

	ctx := c.Context()

	// Optional since/window filter for time-bounded stats.
	var sinceParam any
	sinceClause := ""
	if s := c.Query("since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			sinceParam = t
			sinceClause = " AND created_at >= $2"
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
		if sinceParam != nil {
			sinceClause = " AND created_at >= $2"
		}
	}

	// Count jobs for this tenant.
	var jobsCount int64
	jobsQuery := "SELECT COUNT(*) FROM jobs WHERE tenant_id = $1" + sinceClause
	if sinceParam != nil {
		if err := st.DB.QueryRowContext(ctx, jobsQuery, tenantID, sinceParam).Scan(&jobsCount); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(TenantUsageResponse{
				Success: false,
				Code:    "USAGE_QUERY_FAILED",
				Error:   err.Error(),
			})
		}
	} else {
		if err := st.DB.QueryRowContext(ctx, jobsQuery, tenantID).Scan(&jobsCount); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(TenantUsageResponse{
				Success: false,
				Code:    "USAGE_QUERY_FAILED",
				Error:   err.Error(),
			})
		}
	}

	// Jobs by type.
	jobsByType := make(map[string]int64)
	jobsByTypeQuery := "SELECT type, COUNT(*) FROM jobs WHERE tenant_id = $1" + sinceClause + " GROUP BY type"
	var rows *sql.Rows
	var err2 error
	if sinceParam != nil {
		rows, err2 = st.DB.QueryContext(ctx, jobsByTypeQuery, tenantID, sinceParam)
	} else {
		rows, err2 = st.DB.QueryContext(ctx, jobsByTypeQuery, tenantID)
	}
	if err2 == nil {
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

	// Count documents for this tenant (via jobs).
	var docsCount int64
	docsQuery := "SELECT COUNT(*) FROM documents d JOIN jobs j ON d.job_id = j.id WHERE j.tenant_id = $1" + sinceClause
	if sinceParam != nil {
		if err := st.DB.QueryRowContext(ctx, docsQuery, tenantID, sinceParam).Scan(&docsCount); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(TenantUsageResponse{
				Success: false,
				Code:    "USAGE_QUERY_FAILED",
				Error:   err.Error(),
			})
		}
	} else {
		if err := st.DB.QueryRowContext(ctx, docsQuery, tenantID).Scan(&docsCount); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(TenantUsageResponse{
				Success: false,
				Code:    "USAGE_QUERY_FAILED",
				Error:   err.Error(),
			})
		}
	}

	// Documents by job type.
	docsByType := make(map[string]int64)
	docsByTypeQuery := "SELECT j.type, COUNT(*) FROM documents d JOIN jobs j ON d.job_id = j.id WHERE j.tenant_id = $1" + sinceClause + " GROUP BY j.type"
	if sinceParam != nil {
		rows, err2 = st.DB.QueryContext(ctx, docsByTypeQuery, tenantID, sinceParam)
	} else {
		rows, err2 = st.DB.QueryContext(ctx, docsByTypeQuery, tenantID)
	}
	if err2 == nil {
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

	return c.Status(fiber.StatusOK).JSON(TenantUsageResponse{
		Success:         true,
		Jobs:            jobsCount,
		Documents:       docsCount,
		JobsByType:      jobsByType,
		DocumentsByType: docsByType,
	})
}

// Tenant-admin membership operations under /v1/tenants/:id/members.

// tenantAddMemberHandler adds or updates a member for a tenant.
func tenantAddMemberHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)
	q := db.New(st.DB)

	val := c.Locals("principal")
	p, ok := val.(Principal)
	if !ok || p.UserID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Success: false,
			Code:    "UNAUTHENTICATED",
			Error:   "User context is not available for this request",
		})
	}

	rawID := c.Params("id")
	tenantID, err := uuid.Parse(rawID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "invalid tenant id",
		})
	}

	// Enforce tenant admin (or system admin) for this tenant.
	if !p.IsSystemAdmin {
		if err := RequireTenantAdmin(c, p, tenantID.String()); err != nil {
			return err
		}
	}

	var req TenantMemberRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST_INVALID_JSON",
			Error:   "Bad request, malformed JSON",
		})
	}

	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "invalid userId",
		})
	}

	role := strings.ToLower(strings.TrimSpace(req.Role))
	if role != "tenant_admin" && role != "tenant_member" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "role must be 'tenant_admin' or 'tenant_member'",
		})
	}

	if _, err := q.AddTenantMember(c.Context(), db.AddTenantMemberParams{
		TenantID: tenantID,
		UserID:   userID,
		Role:     role,
	}); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "TENANT_MEMBER_ADD_FAILED",
			Error:   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"success": true})
}

// tenantUpdateMemberHandler updates the role for a tenant member.
func tenantUpdateMemberHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)
	q := db.New(st.DB)

	val := c.Locals("principal")
	p, ok := val.(Principal)
	if !ok || p.UserID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Success: false,
			Code:    "UNAUTHENTICATED",
			Error:   "User context is not available for this request",
		})
	}

	rawID := c.Params("id")
	tenantID, err := uuid.Parse(rawID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "invalid tenant id",
		})
	}

	rawUserID := c.Params("userID")
	userID, err := uuid.Parse(rawUserID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "invalid userID",
		})
	}

	// Enforce tenant admin (or system admin) for this tenant.
	if !p.IsSystemAdmin {
		if err := RequireTenantAdmin(c, p, tenantID.String()); err != nil {
			return err
		}
	}

	var req TenantMemberRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST_INVALID_JSON",
			Error:   "Bad request, malformed JSON",
		})
	}

	role := strings.ToLower(strings.TrimSpace(req.Role))
	if role != "tenant_admin" && role != "tenant_member" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "role must be 'tenant_admin' or 'tenant_member'",
		})
	}

	if _, err := q.UpdateTenantMemberRole(c.Context(), db.UpdateTenantMemberRoleParams{
		TenantID: tenantID,
		UserID:   userID,
		Role:     role,
	}); err != nil {
		if err == sql.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Success: false,
				Code:    "NOT_FOUND",
				Error:   "tenant member not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "TENANT_MEMBER_UPDATE_FAILED",
			Error:   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"success": true})
}

// tenantRemoveMemberHandler removes a member from a tenant.
func tenantRemoveMemberHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)
	q := db.New(st.DB)

	val := c.Locals("principal")
	p, ok := val.(Principal)
	if !ok || p.UserID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Success: false,
			Code:    "UNAUTHENTICATED",
			Error:   "User context is not available for this request",
		})
	}

	rawID := c.Params("id")
	tenantID, err := uuid.Parse(rawID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "invalid tenant id",
		})
	}

	rawUserID := c.Params("userID")
	userID, err := uuid.Parse(rawUserID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "invalid userID",
		})
	}

	// Enforce tenant admin (or system admin) for this tenant.
	if !p.IsSystemAdmin {
		if err := RequireTenantAdmin(c, p, tenantID.String()); err != nil {
			return err
		}
	}

	if err := q.RemoveTenantMember(c.Context(), db.RemoveTenantMemberParams{
		TenantID: tenantID,
		UserID:   userID,
	}); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "TENANT_MEMBER_REMOVE_FAILED",
			Error:   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"success": true})
}
