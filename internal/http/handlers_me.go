package http

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"raito/internal/config"
	"raito/internal/db"
	"raito/internal/store"
)

type MeResponse struct {
	Success        bool           `json:"success"`
	Code           string         `json:"code,omitempty"`
	Error          string         `json:"error,omitempty"`
	User           *MeUser        `json:"user,omitempty"`
	PersonalTenant *MeTenantBrief `json:"personalTenant,omitempty"`
	ActiveTenant   *MeTenantBrief `json:"activeTenant,omitempty"`
}

type MeUser struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	IsSystemAdmin bool   `json:"isSystemAdmin"`
}

type MeTenantBrief struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

func meHandler(c *fiber.Ctx) error {
	cfg := c.Locals("config").(*config.Config)
	_ = cfg // reserved for future tenant-level overrides
	st := c.Locals("store").(*store.Store)

	val := c.Locals("principal")
	p, ok := val.(Principal)
	if !ok || p.UserID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(MeResponse{
			Success: false,
			Code:    "UNAUTHENTICATED",
			Error:   "User context is not available for this request",
		})
	}

	userID := *p.UserID

	q := db.New(st.DB)
	user, err := q.GetUserByID(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(MeResponse{
			Success: false,
			Code:    "INTERNAL_ERROR",
			Error:   err.Error(),
		})
	}

	meUser := &MeUser{
		ID:            user.ID.String(),
		Email:         user.Email,
		IsSystemAdmin: user.IsSystemAdmin,
	}

	// Try to locate the user's personal tenant by owner_user_id/type.
	personalTenants, err := q.ListPersonalTenantsForUser(c.Context(), uuid.NullUUID{UUID: userID, Valid: true})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(MeResponse{
			Success: false,
			Code:    "INTERNAL_ERROR",
			Error:   err.Error(),
		})
	}

	var meTenant *MeTenantBrief
	if len(personalTenants) > 0 {
		pt := personalTenants[0]
		meTenant = &MeTenantBrief{
			ID:   pt.ID.String(),
			Slug: pt.Slug,
			Name: pt.Name,
			Type: pt.Type,
		}
	}

	var activeTenant *MeTenantBrief
	if p.TenantID != nil {
		t, err := q.GetTenantByID(c.Context(), *p.TenantID)
		if err == nil {
			activeTenant = &MeTenantBrief{
				ID:   t.ID.String(),
				Slug: t.Slug,
				Name: t.Name,
				Type: t.Type,
			}
		}
	}

	return c.Status(fiber.StatusOK).JSON(MeResponse{
		Success:        true,
		User:           meUser,
		PersonalTenant: meTenant,
		ActiveTenant:   activeTenant,
	})
}
