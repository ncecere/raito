package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"raito/internal/config"
	"raito/internal/store"
)

// Test that authMiddleware builds a Principal from a valid session cookie.
func TestAuthMiddleware_SessionPrincipal(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.Enabled = true
	cfg.Auth.Session.Secret = "test-secret"
	cfg.Auth.Session.CookieName = "raito_session_test_mw"
	cfg.Auth.Session.TTLMinutes = 60

	st := &store.Store{}

	app := fiber.New()
	app.Use(authMiddleware(cfg, st))

	var captured Principal
	app.Get("/protected", func(c *fiber.Ctx) error {
		val := c.Locals("principal")
		p, ok := val.(Principal)
		if !ok {
			t.Fatalf("expected Principal in context, got %T", val)
		}
		captured = p
		return c.SendStatus(http.StatusOK)
	})

	userID := uuid.New()
	tenantID := uuid.New()

	claims := sessionClaims{
		UserID:        userID.String(),
		TenantID:      tenantID.String(),
		IsSystemAdmin: true,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(time.Hour)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(cfg.Auth.Session.Secret))
	if err != nil {
		t.Fatalf("SignedString error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{Name: cfg.Auth.Session.CookieName, Value: signed})

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	if captured.UserID == nil || *captured.UserID != userID {
		t.Fatalf("expected UserID %v, got %#v", userID, captured.UserID)
	}
	if captured.TenantID == nil || *captured.TenantID != tenantID {
		t.Fatalf("expected TenantID %v, got %#v", tenantID, captured.TenantID)
	}
	if !captured.IsSystemAdmin {
		t.Fatalf("expected IsSystemAdmin=true")
	}
}

// Test that authMiddleware rejects when no API key or session is provided.
func TestAuthMiddleware_SessionMissing(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.Enabled = true
	cfg.Auth.Session.Secret = "test-secret"
	cfg.Auth.Session.CookieName = "raito_session_test_mw"

	st := &store.Store{}

	app := fiber.New()
	app.Use(authMiddleware(cfg, st))
	app.Get("/protected", func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}
