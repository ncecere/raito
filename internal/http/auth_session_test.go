package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"raito/internal/config"
	"raito/internal/store"
)

// TestAuthSession_Unauthenticated verifies that /auth/session guarded by authMiddleware
// rejects requests without API key or session cookie, returning 401.
func TestAuthSession_Unauthenticated(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.Enabled = true
	cfg.Auth.Session.Secret = "test-secret"

	st := &store.Store{}

	app := fiber.New()
	authMw := authMiddleware(cfg, st)

	app.Get("/auth/session", authMw, func(c *fiber.Ctx) error {
		// If we get here, authentication unexpectedly succeeded.
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/session", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}
