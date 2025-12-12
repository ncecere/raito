package http

import (
	"github.com/gofiber/fiber/v2"
	"raito/internal/store"
)

type createAPIKeyRequest struct {
	Label              string `json:"label"`
	RateLimitPerMinute *int   `json:"rateLimitPerMinute,omitempty"`
}

type createAPIKeyResponse struct {
	Success bool   `json:"success"`
	Key     string `json:"key"`
}

// registerAdminRoutes registers admin-only endpoints under /admin.
func registerAdminRoutes(group fiber.Router) {
	group.Post("/api-keys", adminCreateAPIKeyHandler)
}

// adminCreateAPIKeyHandler creates a new user API key and returns the raw key once.
func adminCreateAPIKeyHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)

	var req createAPIKeyRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST_INVALID_JSON",
			Error:   "Bad request, malformed JSON",
		})
	}

	if req.Label == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "label is required",
		})
	}

	// For now, always create non-admin keys via this endpoint.
	rawKey, _, err := st.CreateRandomAPIKey(c.Context(), req.Label, false, req.RateLimitPerMinute, nil)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "API_KEY_CREATE_FAILED",
			Error:   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(createAPIKeyResponse{
		Success: true,
		Key:     rawKey,
	})
}
