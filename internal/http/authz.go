package http

import (
	"database/sql"
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"raito/internal/db"
	"raito/internal/store"
)

// RequireSystemAdmin ensures the principal is a system/global admin.
func RequireSystemAdmin(c *fiber.Ctx, p Principal) error {
	if !p.IsSystemAdmin {
		return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{
			Success: false,
			Code:    "FORBIDDEN",
			Error:   "Admin privileges required",
		})
	}
	return nil
}

// RequireTenantAdmin ensures the principal is a tenant admin for the
// given tenant. System admins are always allowed.
func RequireTenantAdmin(c *fiber.Ctx, p Principal, tenantID string) error {
	if p.IsSystemAdmin {
		return nil
	}

	if tenantID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "tenant id is required",
		})
	}

	if p.UserID == nil {
		return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{
			Success: false,
			Code:    "FORBIDDEN",
			Error:   "Tenant admin privileges required",
		})
	}

	tid, err := uuid.Parse(tenantID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "invalid tenant id",
		})
	}

	stVal := c.Locals("store")
	st, ok := stVal.(*store.Store)
	if !ok || st == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "INTERNAL_ERROR",
			Error:   "store not available in context",
		})
	}

	q := db.New(st.DB)
	member, err := q.GetTenantMember(c.Context(), db.GetTenantMemberParams{
		TenantID: tid,
		UserID:   *p.UserID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{
				Success: false,
				Code:    "FORBIDDEN",
				Error:   "Tenant admin privileges required",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "TENANT_LOOKUP_FAILED",
			Error:   err.Error(),
		})
	}

	if member.Role != "tenant_admin" {
		return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{
			Success: false,
			Code:    "FORBIDDEN",
			Error:   "Tenant admin privileges required",
		})
	}

	return nil
}

// RequireTenantMemberOrAdmin ensures the principal is a member (or
// admin) of the given tenant. System admins are always allowed.
func RequireTenantMemberOrAdmin(c *fiber.Ctx, p Principal, tenantID string) error {
	if p.IsSystemAdmin {
		return nil
	}

	if tenantID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "tenant id is required",
		})
	}

	if p.UserID == nil {
		return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{
			Success: false,
			Code:    "FORBIDDEN",
			Error:   "Tenant membership required",
		})
	}

	tid, err := uuid.Parse(tenantID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "invalid tenant id",
		})
	}

	stVal := c.Locals("store")
	st, ok := stVal.(*store.Store)
	if !ok || st == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "INTERNAL_ERROR",
			Error:   "store not available in context",
		})
	}

	q := db.New(st.DB)
	member, err := q.GetTenantMember(c.Context(), db.GetTenantMemberParams{
		TenantID: tid,
		UserID:   *p.UserID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{
				Success: false,
				Code:    "FORBIDDEN",
				Error:   "Tenant membership required",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "TENANT_LOOKUP_FAILED",
			Error:   err.Error(),
		})
	}

	if member.Role != "tenant_admin" && member.Role != "tenant_member" {
		return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{
			Success: false,
			Code:    "FORBIDDEN",
			Error:   "Tenant membership required",
		})
	}

	return nil
}
