package http

import (
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"raito/internal/db"
	"raito/internal/store"
)

type adminAPIKeyItem struct {
	ID                 string     `json:"id"`
	Label              string     `json:"label"`
	IsAdmin            bool       `json:"isAdmin"`
	RateLimitPerMinute *int       `json:"rateLimitPerMinute,omitempty"`
	TenantID           *string    `json:"tenantId,omitempty"`
	TenantName         *string    `json:"tenantName,omitempty"`
	TenantSlug         *string    `json:"tenantSlug,omitempty"`
	TenantType         *string    `json:"tenantType,omitempty"`
	UserID             *string    `json:"userId,omitempty"`
	UserEmail          *string    `json:"userEmail,omitempty"`
	UserName           *string    `json:"userName,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	RevokedAt          *time.Time `json:"revokedAt,omitempty"`
}

type adminAPIKeysResponse struct {
	Success bool              `json:"success"`
	Total   int64             `json:"total"`
	Keys    []adminAPIKeyItem `json:"keys"`
}

type adminRevokeAPIKeyResponse struct {
	Success   bool      `json:"success"`
	ID        string    `json:"id"`
	RevokedAt time.Time `json:"revokedAt"`
}

func adminListAPIKeysHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)
	q := db.New(st.DB)

	query := c.Query("query")

	includeRevoked := false
	if v := c.Query("includeRevoked"); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Success: false,
				Code:    "BAD_REQUEST",
				Error:   "invalid includeRevoked value; expected true or false",
			})
		}
		includeRevoked = parsed
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

	total, err := q.AdminCountAPIKeys(c.Context(), db.AdminCountAPIKeysParams{
		Column1: query,
		Column2: includeRevoked,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "ADMIN_API_KEYS_COUNT_FAILED",
			Error:   err.Error(),
		})
	}

	rows, err := q.AdminListAPIKeys(c.Context(), db.AdminListAPIKeysParams{
		Column1: query,
		Column2: includeRevoked,
		Limit:   int32(limit),
		Offset:  int32(offset),
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "ADMIN_API_KEYS_LIST_FAILED",
			Error:   err.Error(),
		})
	}

	keys := make([]adminAPIKeyItem, 0, len(rows))
	for _, row := range rows {
		item := adminAPIKeyItem{
			ID:        row.ID.String(),
			Label:     row.Label,
			IsAdmin:   row.IsAdmin,
			CreatedAt: row.CreatedAt,
		}
		if row.RateLimitPerMinute.Valid {
			v := int(row.RateLimitPerMinute.Int32)
			item.RateLimitPerMinute = &v
		}
		if row.TenantID.Valid {
			v := row.TenantID.String
			item.TenantID = &v
		}
		if row.UserID.Valid {
			v := row.UserID.UUID.String()
			item.UserID = &v
		}
		if row.RevokedAt.Valid {
			v := row.RevokedAt.Time
			item.RevokedAt = &v
		}
		if row.TenantName.Valid {
			v := row.TenantName.String
			item.TenantName = &v
		}
		if row.TenantSlug.Valid {
			v := row.TenantSlug.String
			item.TenantSlug = &v
		}
		if row.TenantType.Valid {
			v := row.TenantType.String
			item.TenantType = &v
		}
		if row.UserEmail.Valid {
			v := row.UserEmail.String
			item.UserEmail = &v
		}
		if row.UserName.Valid {
			v := row.UserName.String
			item.UserName = &v
		}
		keys = append(keys, item)
	}

	return c.Status(fiber.StatusOK).JSON(adminAPIKeysResponse{
		Success: true,
		Total:   total,
		Keys:    keys,
	})
}

func adminRevokeAPIKeyHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)
	q := db.New(st.DB)

	rawID := c.Params("id")
	keyID, err := uuid.Parse(rawID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "invalid api key id",
		})
	}

	row, err := q.AdminRevokeAPIKey(c.Context(), keyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Success: false,
				Code:    "NOT_FOUND",
				Error:   "api key not found or already revoked",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "ADMIN_API_KEY_REVOKE_FAILED",
			Error:   err.Error(),
		})
	}

	if !row.RevokedAt.Valid {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "ADMIN_API_KEY_REVOKE_FAILED",
			Error:   "api key revoked timestamp missing",
		})
	}

	recordAuditEvent(c, st, "admin.api_key.revoke", auditEventOptions{
		ResourceType: "api_key",
		ResourceID:   row.ID.String(),
	})

	return c.Status(fiber.StatusOK).JSON(adminRevokeAPIKeyResponse{
		Success:   true,
		ID:        row.ID.String(),
		RevokedAt: row.RevokedAt.Time,
	})
}
