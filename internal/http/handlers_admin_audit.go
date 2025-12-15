package http

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"raito/internal/db"
	"raito/internal/store"
)

type adminAuditEvent struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	Action    string    `json:"action"`

	ActorUserID      *string `json:"actorUserId,omitempty"`
	ActorUserEmail   *string `json:"actorUserEmail,omitempty"`
	ActorUserName    *string `json:"actorUserName,omitempty"`
	ActorAPIKeyID    *string `json:"actorApiKeyId,omitempty"`
	ActorAPIKeyLabel *string `json:"actorApiKeyLabel,omitempty"`
	TenantID         *string `json:"tenantId,omitempty"`
	TenantName       *string `json:"tenantName,omitempty"`
	TenantSlug       *string `json:"tenantSlug,omitempty"`
	TenantType       *string `json:"tenantType,omitempty"`
	ResourceType     *string `json:"resourceType,omitempty"`
	ResourceID       *string `json:"resourceId,omitempty"`
	IP               *string `json:"ip,omitempty"`
	UserAgent        *string `json:"userAgent,omitempty"`
	Metadata         any     `json:"metadata,omitempty"`
}

type adminAuditResponse struct {
	Success bool              `json:"success"`
	Total   int64             `json:"total"`
	Events  []adminAuditEvent `json:"events"`
}

func adminListAuditEventsHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)
	q := db.New(st.DB)

	query := strings.TrimSpace(c.Query("query"))
	action := strings.TrimSpace(c.Query("action"))
	actorType := strings.TrimSpace(c.Query("actorType")) // "", "session", "api_key"

	var hasTenant bool
	var tenantID uuid.UUID
	if v := strings.TrimSpace(c.Query("tenantId")); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Success: false,
				Code:    "BAD_REQUEST",
				Error:   "invalid tenantId",
			})
		}
		hasTenant = true
		tenantID = id
	}

	var hasActorUser bool
	var actorUserID uuid.UUID
	if v := strings.TrimSpace(c.Query("userId")); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Success: false,
				Code:    "BAD_REQUEST",
				Error:   "invalid userId",
			})
		}
		hasActorUser = true
		actorUserID = id
	}

	var hasActorKey bool
	var actorAPIKeyID uuid.UUID
	if v := strings.TrimSpace(c.Query("apiKeyId")); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Success: false,
				Code:    "BAD_REQUEST",
				Error:   "invalid apiKeyId",
			})
		}
		hasActorKey = true
		actorAPIKeyID = id
	}

	var hasSince bool
	var since time.Time
	if s := c.Query("since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			hasSince = true
			since = t
		}
	} else if w := c.Query("window"); w != "" {
		now := time.Now().UTC()
		switch w {
		case "24h":
			hasSince = true
			since = now.Add(-24 * time.Hour)
		case "7d":
			hasSince = true
			since = now.Add(-7 * 24 * time.Hour)
		case "30d":
			hasSince = true
			since = now.Add(-30 * 24 * time.Hour)
		}
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

	total, err := q.AdminCountAuditEvents(c.Context(), db.AdminCountAuditEventsParams{
		Column1:       query,
		Column2:       action,
		Column3:       hasTenant,
		TenantID:      uuid.NullUUID{UUID: tenantID, Valid: hasTenant},
		Column5:       hasActorUser,
		ActorUserID:   uuid.NullUUID{UUID: actorUserID, Valid: hasActorUser},
		Column7:       hasActorKey,
		ActorApiKeyID: uuid.NullUUID{UUID: actorAPIKeyID, Valid: hasActorKey},
		Column9:       hasSince,
		CreatedAt:     since,
		Column11:      actorType,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "AUDIT_LIST_FAILED",
			Error:   err.Error(),
		})
	}

	rows, err := q.AdminListAuditEvents(c.Context(), db.AdminListAuditEventsParams{
		Column1:       query,
		Column2:       action,
		Column3:       hasTenant,
		TenantID:      uuid.NullUUID{UUID: tenantID, Valid: hasTenant},
		Column5:       hasActorUser,
		ActorUserID:   uuid.NullUUID{UUID: actorUserID, Valid: hasActorUser},
		Column7:       hasActorKey,
		ActorApiKeyID: uuid.NullUUID{UUID: actorAPIKeyID, Valid: hasActorKey},
		Column9:       hasSince,
		CreatedAt:     since,
		Column11:      actorType,
		Limit:         int32(limit),
		Offset:        int32(offset),
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "AUDIT_LIST_FAILED",
			Error:   err.Error(),
		})
	}

	events := make([]adminAuditEvent, 0, len(rows))
	for _, row := range rows {
		ev := adminAuditEvent{
			ID:        row.ID,
			CreatedAt: row.CreatedAt,
			Action:    row.Action,
		}
		if row.ActorUserID.Valid {
			v := row.ActorUserID.UUID.String()
			ev.ActorUserID = &v
		}
		if row.ActorApiKeyID.Valid {
			v := row.ActorApiKeyID.UUID.String()
			ev.ActorAPIKeyID = &v
		}
		if row.TenantID.Valid {
			v := row.TenantID.UUID.String()
			ev.TenantID = &v
		}
		if row.ResourceType.Valid {
			v := row.ResourceType.String
			ev.ResourceType = &v
		}
		if row.ResourceID.Valid {
			v := row.ResourceID.String
			ev.ResourceID = &v
		}
		if row.Ip.Valid {
			v := row.Ip.String
			ev.IP = &v
		}
		if row.UserAgent.Valid {
			v := row.UserAgent.String
			ev.UserAgent = &v
		}
		if row.ActorUserEmail.Valid {
			v := row.ActorUserEmail.String
			ev.ActorUserEmail = &v
		}
		if row.ActorUserName.Valid {
			v := row.ActorUserName.String
			ev.ActorUserName = &v
		}
		if row.ActorApiKeyLabel.Valid {
			v := row.ActorApiKeyLabel.String
			ev.ActorAPIKeyLabel = &v
		}
		if row.TenantName.Valid {
			v := row.TenantName.String
			ev.TenantName = &v
		}
		if row.TenantSlug.Valid {
			v := row.TenantSlug.String
			ev.TenantSlug = &v
		}
		if row.TenantType.Valid {
			v := row.TenantType.String
			ev.TenantType = &v
		}
		if len(row.Metadata) > 0 {
			var meta any
			if err := json.Unmarshal(row.Metadata, &meta); err == nil {
				ev.Metadata = meta
			}
		}
		events = append(events, ev)
	}

	return c.Status(fiber.StatusOK).JSON(adminAuditResponse{
		Success: true,
		Total:   total,
		Events:  events,
	})
}
