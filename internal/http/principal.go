package http

import (
	"github.com/google/uuid"

	"raito/internal/db"
)

// Principal represents the authenticated identity for a request.
// Initially it is constructed from API keys; later it will also
// be constructed from user sessions (local or OIDC).
type Principal struct {
	UserID        *uuid.UUID
	IsSystemAdmin bool

	APIKeyID       *uuid.UUID
	APIKeyTenantID *string

	TenantID   *uuid.UUID
	TenantRole string
}

// principalFromAPIKey builds a Principal from a db.ApiKey. As we
// introduce users and tenants, this function can be extended to
// populate user and tenant context based on api_keys.user_id and
// tenant membership.
func principalFromAPIKey(k db.ApiKey) Principal {
	p := Principal{}

	id := k.ID
	p.APIKeyID = &id

	if k.TenantID.Valid {
		idStr := k.TenantID.String
		p.APIKeyTenantID = &idStr
		if parsed, err := uuid.Parse(idStr); err == nil {
			p.TenantID = &parsed
		}
	}

	if k.UserID.Valid {
		uid := k.UserID.UUID
		p.UserID = &uid
	}

	if k.IsAdmin {
		p.IsSystemAdmin = true
	}

	return p
}
