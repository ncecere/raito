package http

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"

	"raito/internal/db"
)

func TestPrincipalFromAPIKey_PopulatesFields(t *testing.T) {
	var (
		userID     = uuid.New()
		tenantUUID = uuid.New()
	)

	apiKey := db.ApiKey{
		ID:      uuid.New(),
		IsAdmin: true,
		UserID:  uuid.NullUUID{UUID: userID, Valid: true},
		TenantID: sql.NullString{
			String: tenantUUID.String(),
			Valid:  true,
		},
	}

	p := principalFromAPIKey(apiKey)

	if p.APIKeyID == nil || *p.APIKeyID != apiKey.ID {
		t.Fatalf("expected APIKeyID %v, got %#v", apiKey.ID, p.APIKeyID)
	}
	if !p.IsSystemAdmin {
		t.Fatalf("expected IsSystemAdmin=true")
	}
	if p.UserID == nil || *p.UserID != userID {
		t.Fatalf("expected UserID %v, got %#v", userID, p.UserID)
	}
	if p.APIKeyTenantID == nil || *p.APIKeyTenantID != tenantUUID.String() {
		t.Fatalf("expected APIKeyTenantID %q, got %#v", tenantUUID.String(), p.APIKeyTenantID)
	}
	if p.TenantID == nil || *p.TenantID != tenantUUID {
		t.Fatalf("expected TenantID %v, got %#v", tenantUUID, p.TenantID)
	}
}
