package http

import (
	"database/sql"
	"encoding/json"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"raito/internal/db"
	"raito/internal/store"
)

type auditEventOptions struct {
	TenantID       *uuid.UUID
	ResourceType   string
	ResourceID     string
	Metadata       any
	OverrideUser   *uuid.UUID
	OverrideAPIKey *uuid.UUID
}

func recordAuditEvent(c *fiber.Ctx, st *store.Store, action string, opts auditEventOptions) {
	q := db.New(st.DB)

	val := c.Locals("principal")
	p, _ := val.(Principal)

	actorUserID := uuid.NullUUID{}
	actorAPIKeyID := uuid.NullUUID{}
	if opts.OverrideUser != nil {
		actorUserID = uuid.NullUUID{UUID: *opts.OverrideUser, Valid: true}
	} else if p.UserID != nil {
		actorUserID = uuid.NullUUID{UUID: *p.UserID, Valid: true}
	}
	if opts.OverrideAPIKey != nil {
		actorAPIKeyID = uuid.NullUUID{UUID: *opts.OverrideAPIKey, Valid: true}
	} else if p.APIKeyID != nil {
		actorAPIKeyID = uuid.NullUUID{UUID: *p.APIKeyID, Valid: true}
	}

	tenantID := uuid.NullUUID{}
	if opts.TenantID != nil {
		tenantID = uuid.NullUUID{UUID: *opts.TenantID, Valid: true}
	}

	resourceType := sql.NullString{}
	if opts.ResourceType != "" {
		resourceType = sql.NullString{String: opts.ResourceType, Valid: true}
	}
	resourceID := sql.NullString{}
	if opts.ResourceID != "" {
		resourceID = sql.NullString{String: opts.ResourceID, Valid: true}
	}

	ip := c.IP()
	ipVal := sql.NullString{}
	if ip != "" {
		ipVal = sql.NullString{String: ip, Valid: true}
	}
	userAgent := c.Get("User-Agent")
	userAgentVal := sql.NullString{}
	if userAgent != "" {
		userAgentVal = sql.NullString{String: userAgent, Valid: true}
	}

	meta := json.RawMessage([]byte("{}"))
	if opts.Metadata != nil {
		if b, err := json.Marshal(opts.Metadata); err == nil {
			meta = b
		}
	}

	_, _ = q.InsertAuditEvent(c.Context(), db.InsertAuditEventParams{
		Action:        action,
		ActorUserID:   actorUserID,
		ActorApiKeyID: actorAPIKeyID,
		TenantID:      tenantID,
		ResourceType:  resourceType,
		ResourceID:    resourceID,
		Ip:            ipVal,
		UserAgent:     userAgentVal,
		Metadata:      meta,
	})
}
