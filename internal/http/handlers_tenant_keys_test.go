package http

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"raito/internal/config"
	"raito/internal/store"
)

// TestTenantCreateAPIKey_Unauthenticated ensures we reject calls without a principal.
func TestTenantCreateAPIKey_Unauthenticated(t *testing.T) {
	app := fiber.New()
	st := &store.Store{}
	cfg := &config.Config{}

	app.Post("/v1/tenants/:id/api-keys", func(c *fiber.Ctx) error {
		c.Locals("store", st)
		c.Locals("config", cfg)
		return tenantCreateAPIKeyHandler(c)
	})

	id := uuid.New().String()
	req := httptest.NewRequest(http.MethodPost, "/v1/tenants/"+id+"/api-keys", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

// TestTenantCreateAPIKey_InvalidTenantID ensures invalid ids are rejected before DB access.
func TestTenantCreateAPIKey_InvalidTenantID(t *testing.T) {
	app := fiber.New()
	st := &store.Store{}
	cfg := &config.Config{}

	app.Post("/v1/tenants/:id/api-keys", func(c *fiber.Ctx) error {
		c.Locals("store", st)
		c.Locals("config", cfg)
		id := uuid.New()
		p := Principal{UserID: &id, IsSystemAdmin: true}
		c.Locals("principal", p)
		return tenantCreateAPIKeyHandler(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/tenants/not-a-uuid/api-keys", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// TestTenantCreateAPIKey_MissingLabel ensures label validation triggers before DB usage.
func TestTenantCreateAPIKey_MissingLabel(t *testing.T) {
	app := fiber.New()
	st := &store.Store{}
	cfg := &config.Config{}

	app.Post("/v1/tenants/:id/api-keys", func(c *fiber.Ctx) error {
		c.Locals("store", st)
		c.Locals("config", cfg)
		id := uuid.New()
		p := Principal{UserID: &id, IsSystemAdmin: true}
		c.Locals("principal", p)
		return tenantCreateAPIKeyHandler(c)
	})

	id := uuid.New().String()
	body := bytes.NewBufferString("{}")
	req := httptest.NewRequest(http.MethodPost, "/v1/tenants/"+id+"/api-keys", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestTenantListAPIKeys_Unauthenticated(t *testing.T) {
	app := fiber.New()
	st := &store.Store{}
	cfg := &config.Config{}

	app.Get("/v1/tenants/:id/api-keys", func(c *fiber.Ctx) error {
		c.Locals("store", st)
		c.Locals("config", cfg)
		return tenantListAPIKeysHandler(c)
	})

	id := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/v1/tenants/"+id+"/api-keys", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestTenantListAPIKeys_InvalidTenantID(t *testing.T) {
	app := fiber.New()
	st := &store.Store{}
	cfg := &config.Config{}

	app.Get("/v1/tenants/:id/api-keys", func(c *fiber.Ctx) error {
		c.Locals("store", st)
		c.Locals("config", cfg)
		id := uuid.New()
		p := Principal{UserID: &id, IsSystemAdmin: true}
		c.Locals("principal", p)
		return tenantListAPIKeysHandler(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/tenants/not-a-uuid/api-keys", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestTenantRevokeAPIKey_Unauthenticated(t *testing.T) {
	app := fiber.New()
	st := &store.Store{}
	cfg := &config.Config{}

	app.Delete("/v1/tenants/:id/api-keys/:keyID", func(c *fiber.Ctx) error {
		c.Locals("store", st)
		c.Locals("config", cfg)
		return tenantRevokeAPIKeyHandler(c)
	})

	id := uuid.New().String()
	keyID := uuid.New().String()
	req := httptest.NewRequest(http.MethodDelete, "/v1/tenants/"+id+"/api-keys/"+keyID, nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestTenantRevokeAPIKey_InvalidIDs(t *testing.T) {
	app := fiber.New()
	st := &store.Store{}
	cfg := &config.Config{}

	app.Delete("/v1/tenants/:id/api-keys/:keyID", func(c *fiber.Ctx) error {
		c.Locals("store", st)
		c.Locals("config", cfg)
		id := uuid.New()
		p := Principal{UserID: &id, IsSystemAdmin: true}
		c.Locals("principal", p)
		return tenantRevokeAPIKeyHandler(c)
	})

	req := httptest.NewRequest(http.MethodDelete, "/v1/tenants/not-a-uuid/api-keys/not-a-uuid", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}
