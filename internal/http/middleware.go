package http

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"raito/internal/config"
	"raito/internal/db"
	"raito/internal/store"
)

// authMiddleware validates the Authorization: Bearer <key> header and
// attaches the resolved API key to the context as "apiKey".
func authMiddleware(cfg *config.Config, st *store.Store) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if !cfg.Auth.Enabled {
			return c.Next()
		}

		rawAuth := c.Get("Authorization")
		if rawAuth == "" || !strings.HasPrefix(rawAuth, "Bearer ") {
			return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
				Success: false,
				Code:    "UNAUTHENTICATED",
				Error:   "Missing Authorization Bearer token",
			})
		}

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
		return c.Next()
	}
}

// rateLimitMiddleware enforces a simple per-minute fixed-window rate limit
// per API key using Redis.
func rateLimitMiddleware(cfg *config.Config, rdb *redis.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if !cfg.Auth.Enabled || cfg.RateLimit.DefaultPerMinute <= 0 {
			return c.Next()
		}

		val := c.Locals("apiKey")
		apiKey, ok := val.(db.ApiKey)
		if !ok {
			// If there's no apiKey in context, auth should have failed already.
			return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
				Success: false,
				Code:    "UNAUTHENTICATED",
				Error:   "API key not found in context",
			})
		}

		limit := cfg.RateLimit.DefaultPerMinute
		if apiKey.RateLimitPerMinute.Valid && apiKey.RateLimitPerMinute.Int32 > 0 {
			limit = int(apiKey.RateLimitPerMinute.Int32)
		}
		if limit <= 0 {
			return c.Next()
		}

		now := time.Now().UTC()
		window := now.Format("200601021504") // YYYYMMDDHHMM minute window
		key := fmt.Sprintf("raito:rl:%s:%s", apiKey.ID.String(), window)

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

// adminOnlyMiddleware ensures the current API key has admin privileges.
func adminOnlyMiddleware(c *fiber.Ctx) error {
	val := c.Locals("apiKey")
	apiKey, ok := val.(db.ApiKey)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Success: false,
			Code:    "UNAUTHENTICATED",
			Error:   "API key not found in context",
		})
	}

	if !apiKey.IsAdmin {
		return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{
			Success: false,
			Code:    "FORBIDDEN",
			Error:   "Admin privileges required",
		})
	}

	return c.Next()
}
