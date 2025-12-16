package http

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"raito/internal/config"
	"raito/internal/db"
	"raito/internal/store"
)

// authMiddleware validates either an API key (Authorization: Bearer
// raito_...) or a browser session cookie (JWT) and attaches a Principal
// to the context. API keys remain the primary mechanism for automation.
func authMiddleware(cfg *config.Config, st *store.Store) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if !cfg.Auth.Enabled {
			return c.Next()
		}

		var q *db.Queries
		if st != nil && st.DB != nil {
			q = db.New(st.DB)
		}

		// First, prefer API keys for automation.
		rawAuth := c.Get("Authorization")
		if rawAuth != "" && strings.HasPrefix(rawAuth, "Bearer ") {
			token := strings.TrimSpace(strings.TrimPrefix(rawAuth, "Bearer "))
			if token == "" || !strings.HasPrefix(token, "raito_") {
				return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
					Success: false,
					Code:    "UNAUTHENTICATED",
					Error:   "Invalid API key format",
				})
			}

			apiKey, err := st.GetAPIKeyByRawKey(c.Context(), token)
			if err != nil {
				if err == sql.ErrNoRows {
					return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
						Success: false,
						Code:    "UNAUTHENTICATED",
						Error:   "Invalid or revoked API key",
					})
				}
				return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
					Success: false,
					Code:    "INTERNAL_ERROR",
					Error:   fmt.Sprintf("API key lookup failed: %v", err),
				})
			}

			c.Locals("apiKey", apiKey)
			p := principalFromAPIKey(apiKey)

			// If the API key is associated with a user, make sure the user is not disabled.
			if q != nil && p.UserID != nil {
				user, err := q.GetUserByID(c.Context(), *p.UserID)
				if err != nil {
					if err == sql.ErrNoRows {
						return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
							Success: false,
							Code:    "UNAUTHENTICATED",
							Error:   "Invalid or revoked API key",
						})
					}
					return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
						Success: false,
						Code:    "INTERNAL_ERROR",
						Error:   fmt.Sprintf("user lookup failed: %v", err),
					})
				}
				if user.IsDisabled {
					return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{
						Success: false,
						Code:    "FORBIDDEN",
						Error:   "User account is disabled",
					})
				}
				p.IsSystemAdmin = user.IsSystemAdmin
			}

			c.Locals("principal", p)
			return c.Next()
		}

		// Otherwise, try browser session cookie.
		claims, err := parseSessionFromRequest(c, cfg)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
				Success: false,
				Code:    "UNAUTHENTICATED",
				Error:   "Missing or invalid authentication (API key or session)",
			})
		}

		// Build a Principal from session claims.
		p := Principal{}
		if claims.UserID != "" {
			if uid, parseErr := uuid.Parse(claims.UserID); parseErr == nil {
				p.UserID = &uid
			}
		}
		if claims.TenantID != "" {
			if tid, parseErr := uuid.Parse(claims.TenantID); parseErr == nil {
				p.TenantID = &tid
			}
		}
		p.IsSystemAdmin = claims.IsSystemAdmin

		// Validate the user exists and is not disabled, and prefer DB for admin bit.
		if q != nil && p.UserID != nil {
			user, err := q.GetUserByID(c.Context(), *p.UserID)
			if err != nil {
				return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
					Success: false,
					Code:    "UNAUTHENTICATED",
					Error:   "Missing or invalid authentication (API key or session)",
				})
			}
			if user.IsDisabled {
				return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{
					Success: false,
					Code:    "FORBIDDEN",
					Error:   "User account is disabled",
				})
			}
			p.IsSystemAdmin = user.IsSystemAdmin
		}

		c.Locals("principal", p)
		return c.Next()
	}
}

// rateLimitMiddleware enforces a simple per-minute fixed-window rate limit
// per API key using Redis. For browser sessions, it falls back to a
// per-user limit keyed by user ID when available.
func rateLimitMiddleware(cfg *config.Config, rdb *redis.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if !cfg.Auth.Enabled || cfg.RateLimit.DefaultPerMinute <= 0 {
			return c.Next()
		}

		limit := cfg.RateLimit.DefaultPerMinute
		var bucketID string

		if val := c.Locals("apiKey"); val != nil {
			if apiKey, ok := val.(db.ApiKey); ok {
				if apiKey.RateLimitPerMinute.Valid && apiKey.RateLimitPerMinute.Int32 > 0 {
					limit = int(apiKey.RateLimitPerMinute.Int32)
				}
				bucketID = apiKey.ID.String()
			}
		}

		if bucketID == "" {
			// Fall back to per-user bucket for session-based access.
			if val := c.Locals("principal"); val != nil {
				if p, ok := val.(Principal); ok && p.UserID != nil {
					bucketID = p.UserID.String()
				}
			}
		}

		if bucketID == "" || limit <= 0 {
			return c.Next()
		}

		now := time.Now().UTC()
		window := now.Format("200601021504") // YYYYMMDDHHMM minute window
		key := fmt.Sprintf("raito:rl:%s:%s", bucketID, window)

		ctx := c.Context()
		count, err := rdb.Incr(ctx, key).Result()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Success: false,
				Code:    "INTERNAL_ERROR",
				Error:   fmt.Sprintf("rate limit increment failed: %v", err),
			})
		}
		if count == 1 {
			// First hit in this window; set TTL
			_ = rdb.Expire(ctx, key, time.Minute)
		}

		if count > int64(limit) {
			return c.Status(fiber.StatusTooManyRequests).JSON(ErrorResponse{
				Success: false,
				Code:    "RATE_LIMIT_EXCEEDED",
				Error:   "Rate limit exceeded, try again later",
			})
		}

		return c.Next()
	}
}

// adminOnlyMiddleware ensures the current principal has system admin privileges.
func adminOnlyMiddleware(c *fiber.Ctx) error {
	val := c.Locals("principal")
	p, ok := val.(Principal)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Success: false,
			Code:    "UNAUTHENTICATED",
			Error:   "Principal not found in context",
		})
	}

	if !p.IsSystemAdmin {
		return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{
			Success: false,
			Code:    "FORBIDDEN",
			Error:   "Admin privileges required",
		})
	}

	return c.Next()
}
