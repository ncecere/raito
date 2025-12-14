package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"raito/internal/config"
	"raito/internal/store"
)

func newTestAppWithTenantHandlers() *fiber.App {
	app := fiber.New()

	// Minimal setup: inject store and config for handlers.
	st := &store.Store{}
	cfg := &config.Config{}

	app.Get("/v1/tenants/:id/usage", func(c *fiber.Ctx) error {
		c.Locals("store", st)
		c.Locals("config", cfg)
		// principal is set per test when needed
		return tenantUsageHandler(c)
	})

	app.Post("/v1/tenants/:id/members", func(c *fiber.Ctx) error {
		c.Locals("store", st)
		c.Locals("config", cfg)
		return tenantAddMemberHandler(c)
	})

	app.Patch("/v1/tenants/:id/members/:userID", func(c *fiber.Ctx) error {
		c.Locals("store", st)
		c.Locals("config", cfg)
		return tenantUpdateMemberHandler(c)
	})

	app.Delete("/v1/tenants/:id/members/:userID", func(c *fiber.Ctx) error {
		c.Locals("store", st)
		c.Locals("config", cfg)
		return tenantRemoveMemberHandler(c)
	})

	return app
}

func TestTenantUsage_Unauthenticated(t *testing.T) {
	app := newTestAppWithTenantHandlers()

	req := httptest.NewRequest(http.MethodGet, "/v1/tenants/00000000-0000-0000-0000-000000000000/usage", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestTenantUsage_InvalidTenantID(t *testing.T) {
	app := fiber.New()
	st := &store.Store{}
	cfg := &config.Config{}

	app.Get("/v1/tenants/:id/usage", func(c *fiber.Ctx) error {
		c.Locals("store", st)
		c.Locals("config", cfg)
		// inject a dummy principal
		p := Principal{UserID: func() *uuid.UUID { id := uuid.New(); return &id }()}
		c.Locals("principal", p)
		return tenantUsageHandler(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/tenants/not-a-uuid/usage", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestTenantAddMember_Unauthenticated(t *testing.T) {
	app := newTestAppWithTenantHandlers()

	req := httptest.NewRequest(http.MethodPost, "/v1/tenants/00000000-0000-0000-0000-000000000000/members", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestTenantAddMember_InvalidTenantID(t *testing.T) {
	app := fiber.New()
	st := &store.Store{}
	cfg := &config.Config{}

	app.Post("/v1/tenants/:id/members", func(c *fiber.Ctx) error {
		c.Locals("store", st)
		c.Locals("config", cfg)
		p := Principal{UserID: func() *uuid.UUID { id := uuid.New(); return &id }(), IsSystemAdmin: true}
		c.Locals("principal", p)
		return tenantAddMemberHandler(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/tenants/not-a-uuid/members", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}
