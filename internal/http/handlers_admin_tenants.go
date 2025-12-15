package http

import (
	"database/sql"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"raito/internal/db"
	"raito/internal/store"
)

type AdminTenantItem struct {
	ID                              string    `json:"id"`
	Slug                            string    `json:"slug"`
	Name                            string    `json:"name"`
	Type                            string    `json:"type"`
	OwnerUserID                     string    `json:"ownerUserId,omitempty"`
	CreatedAt                       time.Time `json:"createdAt"`
	UpdatedAt                       time.Time `json:"updatedAt"`
	DefaultAPIKeyRateLimitPerMinute *int      `json:"defaultApiKeyRateLimitPerMinute,omitempty"`
}

type AdminTenantResponse struct {
	Success bool             `json:"success"`
	Code    string           `json:"code,omitempty"`
	Error   string           `json:"error,omitempty"`
	Tenant  *AdminTenantItem `json:"tenant,omitempty"`
}

type AdminTenantsListResponse struct {
	Success bool              `json:"success"`
	Code    string            `json:"code,omitempty"`
	Error   string            `json:"error,omitempty"`
	Tenants []AdminTenantItem `json:"tenants,omitempty"`
	Total   int64             `json:"total,omitempty"`
}

type AdminCreateTenantRequest struct {
	Slug                            string                     `json:"slug"`
	Name                            string                     `json:"name"`
	Type                            string                     `json:"type,omitempty"` // personal or org; default org
	DefaultAPIKeyRateLimitPerMinute *int                       `json:"defaultApiKeyRateLimitPerMinute,omitempty"`
	Members                         []AdminTenantMemberRequest `json:"members,omitempty"`
}

// adminCreateTenantHandler creates a new tenant (typically org) for system admins.
func adminCreateTenantHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)
	val := c.Locals("principal")
	p, _ := val.(Principal)

	var req AdminCreateTenantRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(AdminTenantResponse{
			Success: false,
			Code:    "BAD_REQUEST_INVALID_JSON",
			Error:   "Bad request, malformed JSON",
		})
	}

	req.Slug = strings.TrimSpace(strings.ToLower(req.Slug))
	req.Name = strings.TrimSpace(req.Name)
	if req.Slug == "" || req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(AdminTenantResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "slug and name are required",
		})
	}

	typeVal := strings.TrimSpace(strings.ToLower(req.Type))
	if typeVal == "" {
		typeVal = "org"
	}
	if typeVal != "org" && typeVal != "personal" {
		return c.Status(fiber.StatusBadRequest).JSON(AdminTenantResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "type must be 'org' or 'personal'",
		})
	}

	q := db.New(st.DB)
	id := uuid.New()
	tenant, err := q.CreateTenant(c.Context(), db.CreateTenantParams{
		ID:          id,
		Slug:        req.Slug,
		Name:        req.Name,
		Type:        typeVal,
		OwnerUserID: uuid.NullUUID{},
	})
	if err != nil {
		// Best-effort slug conflict detection.
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return c.Status(fiber.StatusBadRequest).JSON(AdminTenantResponse{
				Success: false,
				Code:    "CONFLICT",
				Error:   "tenant slug already exists",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(AdminTenantResponse{
			Success: false,
			Code:    "TENANT_CREATE_FAILED",
			Error:   err.Error(),
		})
	}

	// Persist tenant-scoped default rate limit (optional).
	if req.DefaultAPIKeyRateLimitPerMinute != nil {
		if *req.DefaultAPIKeyRateLimitPerMinute < 0 {
			return c.Status(fiber.StatusBadRequest).JSON(AdminTenantResponse{
				Success: false,
				Code:    "BAD_REQUEST",
				Error:   "defaultApiKeyRateLimitPerMinute must be >= 0",
			})
		}
		tenant, err = q.AdminSetTenantDefaultAPIKeyRateLimit(c.Context(), db.AdminSetTenantDefaultAPIKeyRateLimitParams{
			ID:                              tenant.ID,
			DefaultApiKeyRateLimitPerMinute: sql.NullInt32{Int32: int32(*req.DefaultAPIKeyRateLimitPerMinute), Valid: true},
		})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(AdminTenantResponse{
				Success: false,
				Code:    "TENANT_UPDATE_FAILED",
				Error:   err.Error(),
			})
		}
	}

	// Add requested members (and include the creator as tenant_admin by default).
	members := make([]AdminTenantMemberRequest, 0, len(req.Members)+1)
	if p.UserID != nil {
		members = append(members, AdminTenantMemberRequest{UserID: p.UserID.String(), Role: "tenant_admin"})
	}
	members = append(members, req.Members...)

	seen := map[string]struct{}{}
	for _, m := range members {
		uid, err := uuid.Parse(strings.TrimSpace(m.UserID))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(AdminTenantResponse{
				Success: false,
				Code:    "BAD_REQUEST",
				Error:   "invalid member userId",
			})
		}
		if _, ok := seen[uid.String()]; ok {
			continue
		}
		seen[uid.String()] = struct{}{}

		role := strings.ToLower(strings.TrimSpace(m.Role))
		if role == "" {
			role = "tenant_member"
		}
		if role != "tenant_admin" && role != "tenant_member" {
			return c.Status(fiber.StatusBadRequest).JSON(AdminTenantResponse{
				Success: false,
				Code:    "BAD_REQUEST",
				Error:   "member role must be 'tenant_admin' or 'tenant_member'",
			})
		}
		if _, err := q.AddTenantMember(c.Context(), db.AddTenantMemberParams{
			TenantID: tenant.ID,
			UserID:   uid,
			Role:     role,
		}); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(AdminTenantResponse{
				Success: false,
				Code:    "TENANT_MEMBER_ADD_FAILED",
				Error:   err.Error(),
			})
		}
	}

	out := AdminTenantItem{
		ID:        tenant.ID.String(),
		Slug:      tenant.Slug,
		Name:      tenant.Name,
		Type:      tenant.Type,
		CreatedAt: tenant.CreatedAt,
		UpdatedAt: tenant.UpdatedAt,
	}
	if tenant.OwnerUserID.Valid {
		out.OwnerUserID = tenant.OwnerUserID.UUID.String()
	}
	if tenant.DefaultApiKeyRateLimitPerMinute.Valid {
		v := int(tenant.DefaultApiKeyRateLimitPerMinute.Int32)
		out.DefaultAPIKeyRateLimitPerMinute = &v
	}

	return c.Status(fiber.StatusOK).JSON(AdminTenantResponse{
		Success: true,
		Tenant:  &out,
	})
}

// adminListTenantsHandler lists tenants for system admins.
func adminListTenantsHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)
	q := db.New(st.DB)

	query := strings.TrimSpace(c.Query("query"))
	includePersonal := false
	if raw := c.Query("includePersonal"); raw != "" {
		val, err := strconv.ParseBool(raw)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(AdminTenantsListResponse{
				Success: false,
				Code:    "BAD_REQUEST",
				Error:   "invalid includePersonal value; expected true or false",
			})
		}
		includePersonal = val
	}

	limit := 50
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return c.Status(fiber.StatusBadRequest).JSON(AdminTenantsListResponse{
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
			return c.Status(fiber.StatusBadRequest).JSON(AdminTenantsListResponse{
				Success: false,
				Code:    "BAD_REQUEST",
				Error:   "invalid offset value",
			})
		}
		offset = n
	}

	total, err := q.AdminCountTenants(c.Context(), db.AdminCountTenantsParams{
		Column1: query,
		Column2: includePersonal,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(AdminTenantsListResponse{
			Success: false,
			Code:    "TENANT_LIST_FAILED",
			Error:   err.Error(),
		})
	}

	rows, err := q.AdminListTenants(c.Context(), db.AdminListTenantsParams{
		Column1: query,
		Column2: includePersonal,
		Limit:   int32(limit),
		Offset:  int32(offset),
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(AdminTenantsListResponse{
			Success: false,
			Code:    "TENANT_LIST_FAILED",
			Error:   err.Error(),
		})
	}

	items := make([]AdminTenantItem, 0, len(rows))
	for _, t := range rows {
		item := AdminTenantItem{
			ID:        t.ID.String(),
			Slug:      t.Slug,
			Name:      t.Name,
			Type:      t.Type,
			CreatedAt: t.CreatedAt,
			UpdatedAt: t.UpdatedAt,
		}
		if t.OwnerUserID.Valid {
			item.OwnerUserID = t.OwnerUserID.UUID.String()
		}
		if t.DefaultApiKeyRateLimitPerMinute.Valid {
			v := int(t.DefaultApiKeyRateLimitPerMinute.Int32)
			item.DefaultAPIKeyRateLimitPerMinute = &v
		}
		items = append(items, item)
	}

	return c.Status(fiber.StatusOK).JSON(AdminTenantsListResponse{
		Success: true,
		Tenants: items,
		Total:   total,
	})
}

// adminGetTenantHandler returns details for a single tenant.
func adminGetTenantHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)
	q := db.New(st.DB)

	rawID := c.Params("id")
	tenantID, err := uuid.Parse(rawID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(AdminTenantResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "invalid tenant id",
		})
	}

	tenant, err := q.GetTenantByID(c.Context(), tenantID)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(AdminTenantResponse{
				Success: false,
				Code:    "NOT_FOUND",
				Error:   "tenant not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(AdminTenantResponse{
			Success: false,
			Code:    "TENANT_LOOKUP_FAILED",
			Error:   err.Error(),
		})
	}

	item := AdminTenantItem{
		ID:        tenant.ID.String(),
		Slug:      tenant.Slug,
		Name:      tenant.Name,
		Type:      tenant.Type,
		CreatedAt: tenant.CreatedAt,
		UpdatedAt: tenant.UpdatedAt,
	}
	if tenant.OwnerUserID.Valid {
		item.OwnerUserID = tenant.OwnerUserID.UUID.String()
	}
	if tenant.DefaultApiKeyRateLimitPerMinute.Valid {
		v := int(tenant.DefaultApiKeyRateLimitPerMinute.Int32)
		item.DefaultAPIKeyRateLimitPerMinute = &v
	}

	return c.Status(fiber.StatusOK).JSON(AdminTenantResponse{
		Success: true,
		Tenant:  &item,
	})
}

// adminUpdateTenantHandler updates tenant name (and optionally slug) for system admins.
func adminUpdateTenantHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)

	rawID := c.Params("id")
	tenantID, err := uuid.Parse(rawID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(AdminTenantResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "invalid tenant id",
		})
	}

	var req struct {
		Name string `json:"name,omitempty"`
		Slug string `json:"slug,omitempty"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(AdminTenantResponse{
			Success: false,
			Code:    "BAD_REQUEST_INVALID_JSON",
			Error:   "Bad request, malformed JSON",
		})
	}

	updates := make([]string, 0, 2)
	args := []interface{}{tenantID}
	if n := strings.TrimSpace(req.Name); n != "" {
		updates = append(updates, "name = $2")
		args = append(args, n)
	}
	if s := strings.TrimSpace(strings.ToLower(req.Slug)); s != "" {
		updates = append(updates, "slug = $3")
		args = append(args, s)
	}

	if len(updates) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(AdminTenantResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "no fields to update",
		})
	}

	setClause := strings.Join(updates, ", ")
	query := "UPDATE tenants SET " + setClause + ", updated_at = NOW() WHERE id = $1 RETURNING id, slug, name, type, owner_user_id, created_at, updated_at, default_api_key_rate_limit_per_minute"

	row := st.DB.QueryRowContext(c.Context(), query, args...)
	var t db.Tenant
	if err := row.Scan(&t.ID, &t.Slug, &t.Name, &t.Type, &t.OwnerUserID, &t.CreatedAt, &t.UpdatedAt, &t.DefaultApiKeyRateLimitPerMinute); err != nil {
		if err == sql.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(AdminTenantResponse{
				Success: false,
				Code:    "NOT_FOUND",
				Error:   "tenant not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(AdminTenantResponse{
			Success: false,
			Code:    "TENANT_UPDATE_FAILED",
			Error:   err.Error(),
		})
	}

	item := AdminTenantItem{
		ID:        t.ID.String(),
		Slug:      t.Slug,
		Name:      t.Name,
		Type:      t.Type,
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
	}
	if t.OwnerUserID.Valid {
		item.OwnerUserID = t.OwnerUserID.UUID.String()
	}
	if t.DefaultApiKeyRateLimitPerMinute.Valid {
		v := int(t.DefaultApiKeyRateLimitPerMinute.Int32)
		item.DefaultAPIKeyRateLimitPerMinute = &v
	}

	return c.Status(fiber.StatusOK).JSON(AdminTenantResponse{
		Success: true,
		Tenant:  &item,
	})
}

// Tenant membership management for admins.
type AdminTenantMemberRequest struct {
	UserID string `json:"userId"`
	Role   string `json:"role"`
}

type AdminTenantMemberItem struct {
	UserID    string    `json:"userId"`
	Email     string    `json:"email"`
	Name      *string   `json:"name,omitempty"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type AdminTenantMembersResponse struct {
	Success bool                    `json:"success"`
	Members []AdminTenantMemberItem `json:"members"`
	Total   int64                   `json:"total"`
}

func adminListTenantMembersHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)
	q := db.New(st.DB)

	rawID := c.Params("id")
	tenantID, err := uuid.Parse(rawID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "invalid tenant id",
		})
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

	rows, err := q.AdminListTenantMembers(c.Context(), db.AdminListTenantMembersParams{
		TenantID: tenantID,
		Limit:    int32(limit),
		Offset:   int32(offset),
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "TENANT_MEMBER_LIST_FAILED",
			Error:   err.Error(),
		})
	}

	items := make([]AdminTenantMemberItem, 0, len(rows))
	for _, r := range rows {
		var name *string
		if r.Name.Valid {
			n := r.Name.String
			name = &n
		}
		items = append(items, AdminTenantMemberItem{
			UserID:    r.UserID.String(),
			Email:     r.Email,
			Name:      name,
			Role:      r.Role,
			CreatedAt: r.CreatedAt,
			UpdatedAt: r.UpdatedAt,
		})
	}

	// Total is derived from the current list length; pagination can be extended later.
	total := int64(len(items))
	return c.Status(fiber.StatusOK).JSON(AdminTenantMembersResponse{
		Success: true,
		Members: items,
		Total:   total,
	})
}

// adminAddTenantMemberHandler adds or updates a member for a tenant.
func adminAddTenantMemberHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)
	q := db.New(st.DB)

	rawID := c.Params("id")
	tenantID, err := uuid.Parse(rawID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "invalid tenant id",
		})
	}

	var req AdminTenantMemberRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST_INVALID_JSON",
			Error:   "Bad request, malformed JSON",
		})
	}

	userID, err := uuid.Parse(strings.TrimSpace(req.UserID))
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

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
	})
}

// adminUpdateTenantMemberHandler updates the role for a tenant member.
func adminUpdateTenantMemberHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)
	q := db.New(st.DB)

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

	var req AdminTenantMemberRequest
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

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
	})
}

// adminRemoveTenantMemberHandler removes a member from a tenant.
func adminRemoveTenantMemberHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)
	q := db.New(st.DB)

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

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
	})
}
