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

func TestListTenants_Unauthenticated(t *testing.T) {
	app := fiber.New()
	st := &store.Store{}
	cfg := &config.Config{}

	app.Get("/v1/tenants", func(c *fiber.Ctx) error {
		c.Locals("store", st)
		c.Locals("config", cfg)
		return listTenantsHandler(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/tenants", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestSelectTenant_Unauthenticated(t *testing.T) {
	app := fiber.New()
	st := &store.Store{}
	cfg := &config.Config{}

	app.Post("/v1/tenants/:id/select", func(c *fiber.Ctx) error {
		c.Locals("store", st)
		c.Locals("config", cfg)
		return selectTenantHandler(c)
	})

	id := uuid.New().String()
	req := httptest.NewRequest(http.MethodPost, "/v1/tenants/"+id+"/select", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestSelectTenant_InvalidTenantID(t *testing.T) {
	app := fiber.New()
	st := &store.Store{}
	cfg := &config.Config{}

	app.Post("/v1/tenants/:id/select", func(c *fiber.Ctx) error {
		c.Locals("store", st)
		c.Locals("config", cfg)
		id := uuid.New()
		p := Principal{UserID: &id, IsSystemAdmin: true}
		c.Locals("principal", p)
		return selectTenantHandler(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/tenants/not-a-uuid/select", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}
