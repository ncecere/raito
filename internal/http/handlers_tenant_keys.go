package http

import (
	"database/sql"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"raito/internal/db"
	"raito/internal/store"
)

type TenantAPIKeyItem struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	IsAdmin   bool   `json:"isAdmin"`
	CreatedAt string `json:"createdAt"`
}

type TenantAPIKeysResponse struct {
	Success bool               `json:"success"`
	Code    string             `json:"code,omitempty"`
	Error   string             `json:"error,omitempty"`
	Keys    []TenantAPIKeyItem `json:"keys,omitempty"`
}

type TenantCreateAPIKeyRequest struct {
	Label              string `json:"label"`
	RateLimitPerMinute *int   `json:"rateLimitPerMinute,omitempty"`
}

type TenantCreateAPIKeyResponse struct {
	Success bool   `json:"success"`
	Code    string `json:"code,omitempty"`
	Error   string `json:"error,omitempty"`
	Key     string `json:"key,omitempty"`
}

// tenantCreateAPIKeyHandler creates a tenant-scoped API key for the given tenant.
// System admins and tenant admins are allowed.
func tenantCreateAPIKeyHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)

	val := c.Locals("principal")
	p, ok := val.(Principal)
	if !ok || p.UserID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(TenantCreateAPIKeyResponse{
			Success: false,
			Code:    "UNAUTHENTICATED",
			Error:   "User context is not available for this request",
		})
	}

	rawID := c.Params("id")
	tenantID, err := uuid.Parse(rawID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(TenantCreateAPIKeyResponse{
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

	var req TenantCreateAPIKeyRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(TenantCreateAPIKeyResponse{
			Success: false,
			Code:    "BAD_REQUEST_INVALID_JSON",
			Error:   "Bad request, malformed JSON",
		})
	}

	if req.Label == "" {
		return c.Status(fiber.StatusBadRequest).JSON(TenantCreateAPIKeyResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "label is required",
		})
	}

	raw, _, err := st.CreateRandomAPIKey(c.Context(), req.Label, false, req.RateLimitPerMinute, func() *string {
		s := tenantID.String()
		return &s
	}())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(TenantCreateAPIKeyResponse{
			Success: false,
			Code:    "API_KEY_CREATE_FAILED",
			Error:   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(TenantCreateAPIKeyResponse{
		Success: true,
		Key:     raw,
	})
}

// tenantListAPIKeysHandler lists tenant-scoped API keys for a tenant.
func tenantListAPIKeysHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)
	q := db.New(st.DB)

	val := c.Locals("principal")
	p, ok := val.(Principal)
	if !ok || p.UserID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(TenantAPIKeysResponse{
			Success: false,
			Code:    "UNAUTHENTICATED",
			Error:   "User context is not available for this request",
		})
	}

	rawID := c.Params("id")
	tenantID, err := uuid.Parse(rawID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(TenantAPIKeysResponse{
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

	rows, err := q.ListAPIKeysByTenant(c.Context(), sql.NullString{String: tenantID.String(), Valid: true})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(TenantAPIKeysResponse{
			Success: false,
			Code:    "API_KEY_LIST_FAILED",
			Error:   err.Error(),
		})
	}

	items := make([]TenantAPIKeyItem, 0, len(rows))
	for _, k := range rows {
		item := TenantAPIKeyItem{
			ID:        k.ID.String(),
			Label:     k.Label,
			IsAdmin:   k.IsAdmin,
			CreatedAt: k.CreatedAt.UTC().Format(time.RFC3339),
		}
		items = append(items, item)
	}

	return c.Status(fiber.StatusOK).JSON(TenantAPIKeysResponse{
		Success: true,
		Keys:    items,
	})
}

// tenantRevokeAPIKeyHandler revokes a tenant API key.
func tenantRevokeAPIKeyHandler(c *fiber.Ctx) error {
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

	rawTenantID := c.Params("id")
	tenantID, err := uuid.Parse(rawTenantID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "invalid tenant id",
		})
	}

	rawKeyID := c.Params("keyID")
	keyID, err := uuid.Parse(rawKeyID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "invalid keyID",
		})
	}

	// Enforce tenant admin (or system admin) for this tenant.
	if !p.IsSystemAdmin {
		if err := RequireTenantAdmin(c, p, tenantID.String()); err != nil {
			return err
		}
	}

	// Ensure key belongs to this tenant before revoking.
	rows, err := q.ListAPIKeysByTenant(c.Context(), sql.NullString{String: tenantID.String(), Valid: true})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "API_KEY_LIST_FAILED",
			Error:   err.Error(),
		})
	}

	found := false
	for _, k := range rows {
		if k.ID == keyID {
			found = true
			break
		}
	}
	if !found {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Success: false,
			Code:    "NOT_FOUND",
			Error:   "api key not found for tenant",
		})
	}

	if err := q.RevokeAPIKey(c.Context(), keyID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "API_KEY_REVOKE_FAILED",
			Error:   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"success": true})
}
