package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"raito/internal/store"
)

func TestJobsList_Unauthenticated(t *testing.T) {
	app := fiber.New()
	st := &store.Store{}

	app.Get("/v1/jobs", func(c *fiber.Ctx) error {
		c.Locals("store", st)
		return jobsListHandler(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestJobsList_NonAdminMissingTenant(t *testing.T) {
	app := fiber.New()
	st := &store.Store{}

	app.Get("/v1/jobs", func(c *fiber.Ctx) error {
		c.Locals("store", st)
		// Non-admin user without tenant context
		id := uuid.New()
		p := Principal{UserID: &id}
		c.Locals("principal", p)
		return jobsListHandler(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestJobDetail_Unauthenticated(t *testing.T) {
	app := fiber.New()
	st := &store.Store{}

	app.Get("/v1/jobs/:id", func(c *fiber.Ctx) error {
		c.Locals("store", st)
		return jobDetailHandler(c)
	})

	jobID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+jobID, nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestJobDetail_InvalidID(t *testing.T) {
	app := fiber.New()
	st := &store.Store{}

	app.Get("/v1/jobs/:id", func(c *fiber.Ctx) error {
		c.Locals("store", st)
		id := uuid.New()
		p := Principal{UserID: &id, IsSystemAdmin: true}
		c.Locals("principal", p)
		return jobDetailHandler(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/not-a-uuid", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}
