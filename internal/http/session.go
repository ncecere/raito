package http

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"raito/internal/config"
)

// sessionClaims are the JWT claims we use for browser sessions.
type sessionClaims struct {
	UserID        string `json:"uid"`
	TenantID      string `json:"tid,omitempty"`
	IsSystemAdmin bool   `json:"is_admin"`
	jwt.RegisteredClaims
}

func issueSessionCookie(c *fiber.Ctx, cfg *config.Config, userID uuid.UUID, tenantID *uuid.UUID, isSystemAdmin bool) error {
	// If no session secret is configured, skip issuing a cookie (API-key only).
	secret := cfg.Auth.Session.Secret
	if secret == "" {
		return nil
	}

	name := cfg.Auth.Session.CookieName
	if name == "" {
		name = "raito_session"
	}

	ttlMinutes := cfg.Auth.Session.TTLMinutes
	if ttlMinutes <= 0 {
		ttlMinutes = 1440 // default 24h
	}

	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(ttlMinutes) * time.Minute)

	claims := sessionClaims{
		UserID:        userID.String(),
		IsSystemAdmin: isSystemAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	if tenantID != nil {
		claims.TenantID = tenantID.String()
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return err
	}

	c.Cookie(&fiber.Cookie{
		Name:     name,
		Value:    signed,
		Expires:  expiresAt,
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Lax",
	})

	return nil
}

func parseSessionFromRequest(c *fiber.Ctx, cfg *config.Config) (*sessionClaims, error) {
	secret := cfg.Auth.Session.Secret
	if secret == "" {
		return nil, fiber.ErrUnauthorized
	}

	name := cfg.Auth.Session.CookieName
	if name == "" {
		name = "raito_session"
	}

	cookie := c.Cookies(name)
	if cookie == "" {
		return nil, fiber.ErrUnauthorized
	}

	parsed, err := jwt.ParseWithClaims(cookie, &sessionClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fiber.ErrUnauthorized
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, fiber.ErrUnauthorized
	}

	claims, ok := parsed.Claims.(*sessionClaims)
	if !ok || !parsed.Valid {
		return nil, fiber.ErrUnauthorized
	}

	return claims, nil
}
