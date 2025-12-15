package http

import (
	"database/sql"
	"errors"
	"strings"

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
	ID              string  `json:"id"`
	Email           string  `json:"email"`
	Name            *string `json:"name,omitempty"`
	IsSystemAdmin   bool    `json:"isSystemAdmin"`
	DefaultTenantID *string `json:"defaultTenantId,omitempty"`
	ThemePreference string  `json:"themePreference,omitempty"`
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

	var name *string
	if user.Name.Valid {
		n := user.Name.String
		name = &n
	}

	var defaultTenantID *string
	if user.DefaultTenantID.Valid {
		id := user.DefaultTenantID.UUID.String()
		defaultTenantID = &id
	}

	meUser := &MeUser{
		ID:              user.ID.String(),
		Email:           user.Email,
		Name:            name,
		IsSystemAdmin:   user.IsSystemAdmin,
		DefaultTenantID: defaultTenantID,
		ThemePreference: user.ThemePreference,
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

type UpdateMeRequest struct {
	Name            *string `json:"name,omitempty"`
	ThemePreference *string `json:"themePreference,omitempty"`
	DefaultTenantID *string `json:"defaultTenantId,omitempty"`
}

func updateMeHandler(c *fiber.Ctx) error {
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

	var req UpdateMeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(MeResponse{
			Success: false,
			Code:    "BAD_REQUEST_INVALID_JSON",
			Error:   "Bad request, malformed JSON",
		})
	}

	q := db.New(st.DB)
	user, err := q.GetUserByID(c.Context(), *p.UserID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(MeResponse{
			Success: false,
			Code:    "INTERNAL_ERROR",
			Error:   err.Error(),
		})
	}

	nextName := user.Name
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			nextName = sql.NullString{}
		} else {
			nextName = sql.NullString{String: trimmed, Valid: true}
		}
	}

	nextTheme := user.ThemePreference
	if req.ThemePreference != nil {
		switch *req.ThemePreference {
		case "light", "dark", "system":
			nextTheme = *req.ThemePreference
		default:
			return c.Status(fiber.StatusBadRequest).JSON(MeResponse{
				Success: false,
				Code:    "BAD_REQUEST",
				Error:   "themePreference must be one of: light, dark, system",
			})
		}
	}

	nextDefaultTenant := user.DefaultTenantID
	if req.DefaultTenantID != nil {
		if *req.DefaultTenantID == "" {
			nextDefaultTenant = uuid.NullUUID{}
		} else {
			id, err := uuid.Parse(*req.DefaultTenantID)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(MeResponse{
					Success: false,
					Code:    "BAD_REQUEST",
					Error:   "defaultTenantId must be a valid UUID",
				})
			}
			// Ensure the user is a member of the requested tenant.
			_, err = q.GetTenantMember(c.Context(), db.GetTenantMemberParams{
				TenantID: id,
				UserID:   user.ID,
			})
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return c.Status(fiber.StatusForbidden).JSON(MeResponse{
						Success: false,
						Code:    "FORBIDDEN",
						Error:   "You are not a member of the requested tenant",
					})
				}
				return c.Status(fiber.StatusInternalServerError).JSON(MeResponse{
					Success: false,
					Code:    "INTERNAL_ERROR",
					Error:   err.Error(),
				})
			}
			nextDefaultTenant = uuid.NullUUID{UUID: id, Valid: true}
		}
	}

	updated, err := q.UpdateUserProfile(c.Context(), db.UpdateUserProfileParams{
		ID:              user.ID,
		Name:            nextName,
		ThemePreference: nextTheme,
		DefaultTenantID: nextDefaultTenant,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(MeResponse{
			Success: false,
			Code:    "INTERNAL_ERROR",
			Error:   err.Error(),
		})
	}

	// Respond with the updated /me payload for convenience.
	// Note: active tenant is derived from the current session cookie.
	_ = updated
	return meHandler(c)
}
