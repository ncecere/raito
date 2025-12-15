package http

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"raito/internal/db"
	"raito/internal/store"
)

type AdminUser struct {
	ID            string     `json:"id"`
	Email         string     `json:"email"`
	Name          *string    `json:"name,omitempty"`
	AuthProvider  string     `json:"authProvider"`
	IsSystemAdmin bool       `json:"isSystemAdmin"`
	IsDisabled    bool       `json:"isDisabled"`
	DisabledAt    *time.Time `json:"disabledAt,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	DefaultTenant *string    `json:"defaultTenantId,omitempty"`
	ThemePref     string     `json:"themePreference,omitempty"`
	PasswordSet   bool       `json:"passwordSet"`
}

type adminUsersResponse struct {
	Success bool        `json:"success"`
	Users   []AdminUser `json:"users"`
	Total   int64       `json:"total"`
}

type adminUserResponse struct {
	Success bool      `json:"success"`
	User    AdminUser `json:"user"`
}

type adminUpdateUserRequest struct {
	Name          *string `json:"name,omitempty"`
	IsSystemAdmin *bool   `json:"isSystemAdmin,omitempty"`
	IsDisabled    *bool   `json:"isDisabled,omitempty"`
}

func marshalAdminUser(u db.User) AdminUser {
	var name *string
	if u.Name.Valid {
		n := u.Name.String
		name = &n
	}

	var defaultTenant *string
	if u.DefaultTenantID.Valid {
		id := u.DefaultTenantID.UUID.String()
		defaultTenant = &id
	}

	passwordSet := u.PasswordHash.Valid

	var disabledAt *time.Time
	if u.DisabledAt.Valid {
		t := u.DisabledAt.Time
		disabledAt = &t
	}

	return AdminUser{
		ID:            u.ID.String(),
		Email:         u.Email,
		Name:          name,
		AuthProvider:  u.AuthProvider,
		IsSystemAdmin: u.IsSystemAdmin,
		IsDisabled:    u.IsDisabled,
		DisabledAt:    disabledAt,
		CreatedAt:     u.CreatedAt,
		UpdatedAt:     u.UpdatedAt,
		DefaultTenant: defaultTenant,
		ThemePref:     u.ThemePreference,
		PasswordSet:   passwordSet,
	}
}

type adminCreateUserRequest struct {
	Email         string `json:"email"`
	Password      string `json:"password"`
	Name          string `json:"name,omitempty"`
	IsSystemAdmin bool   `json:"isSystemAdmin,omitempty"`
}

func adminCreateUserHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)

	var req adminCreateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST_INVALID_JSON",
			Error:   "Bad request, malformed JSON",
		})
	}

	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" || !strings.Contains(email, "@") {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "email is required",
		})
	}
	if strings.TrimSpace(req.Password) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "password is required",
		})
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "INTERNAL_ERROR",
			Error:   "password hashing failed",
		})
	}

	tx, err := st.DB.BeginTx(c.Context(), nil)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "INTERNAL_ERROR",
			Error:   err.Error(),
		})
	}
	defer func() {
		_ = tx.Rollback()
	}()

	q := db.New(tx)
	userID := uuid.New()
	name := strings.TrimSpace(req.Name)

	var nameVal sql.NullString
	if name != "" {
		nameVal = sql.NullString{String: name, Valid: true}
	}

	user, err := q.CreateUser(c.Context(), db.CreateUserParams{
		ID:              userID,
		Email:           email,
		Name:            nameVal,
		AuthProvider:    "local",
		AuthSubject:     sql.NullString{},
		IsSystemAdmin:   req.IsSystemAdmin,
		PasswordHash:    sql.NullString{String: string(hash), Valid: true},
		PasswordVersion: sql.NullInt32{Int32: 1, Valid: true},
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   err.Error(),
		})
	}

	tenantID := uuid.New()
	slug := generatePersonalTenantSlug(email, tenantID)
	_, err = q.CreateTenant(c.Context(), db.CreateTenantParams{
		ID:          tenantID,
		Slug:        slug,
		Name:        email,
		Type:        "personal",
		OwnerUserID: uuid.NullUUID{UUID: userID, Valid: true},
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "INTERNAL_ERROR",
			Error:   fmt.Sprintf("personal tenant create failed: %v", err),
		})
	}

	_, err = q.AddTenantMember(c.Context(), db.AddTenantMemberParams{
		TenantID: tenantID,
		UserID:   userID,
		Role:     "tenant_admin",
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "INTERNAL_ERROR",
			Error:   fmt.Sprintf("tenant member create failed: %v", err),
		})
	}

	if err := tx.Commit(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "INTERNAL_ERROR",
			Error:   err.Error(),
		})
	}

	recordAuditEvent(c, st, "admin.user.create", auditEventOptions{
		TenantID:     &tenantID,
		ResourceType: "user",
		ResourceID:   user.ID.String(),
		Metadata: map[string]any{
			"email":         user.Email,
			"isSystemAdmin": user.IsSystemAdmin,
		},
	})

	return c.Status(fiber.StatusOK).JSON(adminUserResponse{
		Success: true,
		User:    marshalAdminUser(user),
	})
}

type adminResetPasswordRequest struct {
	Password string `json:"password"`
}

func adminResetUserPasswordHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)
	q := db.New(st.DB)

	rawID := c.Params("id")
	userID, err := uuid.Parse(rawID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "invalid user id",
		})
	}

	var req adminResetPasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST_INVALID_JSON",
			Error:   "Bad request, malformed JSON",
		})
	}
	if strings.TrimSpace(req.Password) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "password is required",
		})
	}

	user, err := q.GetUserByID(c.Context(), userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Success: false,
				Code:    "NOT_FOUND",
				Error:   "user not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "INTERNAL_ERROR",
			Error:   err.Error(),
		})
	}
	if user.AuthProvider != "local" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "password can only be reset for local users",
		})
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "INTERNAL_ERROR",
			Error:   "password hashing failed",
		})
	}

	nextVersion := int32(1)
	if user.PasswordVersion.Valid && user.PasswordVersion.Int32 > 0 {
		nextVersion = user.PasswordVersion.Int32 + 1
	}

	updated, err := q.AdminSetUserPassword(c.Context(), db.AdminSetUserPasswordParams{
		ID:              userID,
		PasswordHash:    sql.NullString{String: string(hash), Valid: true},
		PasswordVersion: sql.NullInt32{Int32: nextVersion, Valid: true},
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "INTERNAL_ERROR",
			Error:   err.Error(),
		})
	}

	recordAuditEvent(c, st, "admin.user.reset_password", auditEventOptions{
		ResourceType: "user",
		ResourceID:   updated.ID.String(),
		Metadata: map[string]any{
			"email": updated.Email,
		},
	})

	return c.Status(fiber.StatusOK).JSON(adminUserResponse{
		Success: true,
		User:    marshalAdminUser(updated),
	})
}

func adminListUsersHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)
	q := db.New(st.DB)

	query := strings.TrimSpace(c.Query("query"))

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

	total, err := q.AdminCountUsers(c.Context(), query)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "INTERNAL_ERROR",
			Error:   err.Error(),
		})
	}

	users, err := q.AdminListUsers(c.Context(), db.AdminListUsersParams{
		Column1: query,
		Limit:   int32(limit),
		Offset:  int32(offset),
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "INTERNAL_ERROR",
			Error:   err.Error(),
		})
	}

	out := make([]AdminUser, 0, len(users))
	for _, u := range users {
		out = append(out, marshalAdminUser(u))
	}

	return c.Status(fiber.StatusOK).JSON(adminUsersResponse{
		Success: true,
		Users:   out,
		Total:   total,
	})
}

func adminGetUserHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)
	q := db.New(st.DB)

	rawID := c.Params("id")
	userID, err := uuid.Parse(rawID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "invalid user id",
		})
	}

	user, err := q.GetUserByID(c.Context(), userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Success: false,
				Code:    "NOT_FOUND",
				Error:   "user not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "INTERNAL_ERROR",
			Error:   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(adminUserResponse{
		Success: true,
		User:    marshalAdminUser(user),
	})
}

func adminUpdateUserHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)
	q := db.New(st.DB)

	val := c.Locals("principal")
	p, ok := val.(Principal)
	if !ok || p.UserID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Success: false,
			Code:    "UNAUTHENTICATED",
			Error:   "Principal not found in context",
		})
	}

	rawID := c.Params("id")
	userID, err := uuid.Parse(rawID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "invalid user id",
		})
	}

	var req adminUpdateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST_INVALID_JSON",
			Error:   "Bad request, malformed JSON",
		})
	}

	user, err := q.GetUserByID(c.Context(), userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Success: false,
				Code:    "NOT_FOUND",
				Error:   "user not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
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

	nextAdmin := user.IsSystemAdmin
	if req.IsSystemAdmin != nil {
		if userID == *p.UserID && !*req.IsSystemAdmin {
			return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{
				Success: false,
				Code:    "FORBIDDEN",
				Error:   "You cannot revoke your own admin privileges",
			})
		}
		nextAdmin = *req.IsSystemAdmin
	}

	nextDisabled := user.IsDisabled
	var nextDisabledAt sql.NullTime
	if user.DisabledAt.Valid {
		nextDisabledAt = user.DisabledAt
	}
	if req.IsDisabled != nil {
		if userID == *p.UserID && *req.IsDisabled {
			return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{
				Success: false,
				Code:    "FORBIDDEN",
				Error:   "You cannot disable your own account",
			})
		}
		nextDisabled = *req.IsDisabled
		if nextDisabled {
			nextDisabledAt = sql.NullTime{Time: time.Now().UTC(), Valid: true}
		} else {
			nextDisabledAt = sql.NullTime{}
		}
	}

	updated, err := q.AdminUpdateUser(c.Context(), db.AdminUpdateUserParams{
		ID:            userID,
		Name:          nextName,
		IsSystemAdmin: nextAdmin,
		IsDisabled:    nextDisabled,
		DisabledAt:    nextDisabledAt,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "INTERNAL_ERROR",
			Error:   err.Error(),
		})
	}

	recordAuditEvent(c, st, "admin.user.update", auditEventOptions{
		ResourceType: "user",
		ResourceID:   updated.ID.String(),
		Metadata: map[string]any{
			"email":         updated.Email,
			"isSystemAdmin": updated.IsSystemAdmin,
			"isDisabled":    updated.IsDisabled,
		},
	})

	return c.Status(fiber.StatusOK).JSON(adminUserResponse{
		Success: true,
		User:    marshalAdminUser(updated),
	})
}

func generatePersonalTenantSlug(email string, id uuid.UUID) string {
	local := email
	if i := strings.Index(local, "@"); i > 0 {
		local = local[:i]
	}
	local = strings.ReplaceAll(local, " ", "-")
	local = strings.ToLower(local)
	if local == "" {
		local = "user"
	}
	return fmt.Sprintf("%s-%s", local, id.String())
}
