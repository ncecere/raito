package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"raito/internal/config"
)

func TestIssueAndParseSessionCookie_RoundTrip(t *testing.T) {
	app := fiber.New()
	cfg := &config.Config{}
	cfg.Auth.Session.Secret = "test-secret"
	cfg.Auth.Session.CookieName = "raito_session_test"
	cfg.Auth.Session.TTLMinutes = 60

	userID := uuid.New()
	tenantID := uuid.New()

	app.Get("/set", func(c *fiber.Ctx) error {
		if err := issueSessionCookie(c, cfg, userID, &tenantID, true); err != nil {
			t.Fatalf("issueSessionCookie error: %v", err)
		}
		return c.SendStatus(http.StatusOK)
	})

	app.Get("/get", func(c *fiber.Ctx) error {
		claims, err := parseSessionFromRequest(c, cfg)
		if err != nil {
			return c.Status(http.StatusUnauthorized).SendString("unauthorized")
		}
		if claims.UserID != userID.String() {
			t.Fatalf("expected uid %s, got %s", userID.String(), claims.UserID)
		}
		if claims.TenantID != tenantID.String() {
			t.Fatalf("expected tid %s, got %s", tenantID.String(), claims.TenantID)
		}
		if !claims.IsSystemAdmin {
			t.Fatalf("expected is_admin=true")
		}
		if claims.ExpiresAt == nil || time.Until(claims.ExpiresAt.Time) <= 0 {
			t.Fatalf("expected future ExpiresAt, got %#v", claims.ExpiresAt)
		}
		return c.SendStatus(http.StatusOK)
	})

	// First call /set to obtain a session cookie.
	req := httptest.NewRequest(http.MethodGet, "/set", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test(/set) error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /set, got %d", resp.StatusCode)
	}

	cookies := resp.Cookies()
	if len(cookies) == 0 {
		t.Fatalf("expected at least one cookie")
	}

	// Now call /get with the cookie.
	req2 := httptest.NewRequest(http.MethodGet, "/get", nil)
	for _, c := range cookies {
		req2.AddCookie(c)
	}
	resp2, err := app.Test(req2, -1)
	if err != nil {
		t.Fatalf("app.Test(/get) error: %v", err)
	}
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /get, got %d", resp2.StatusCode)
	}
}
