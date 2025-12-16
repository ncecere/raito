package http

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"raito/internal/config"
)

type authProvidersResponse struct {
	Success bool `json:"success"`
	Auth    struct {
		Enabled bool `json:"enabled"`
		Local   struct {
			Enabled bool `json:"enabled"`
		} `json:"local"`
		OIDC struct {
			Enabled bool   `json:"enabled"`
			Issuer  string `json:"issuer,omitempty"`
		} `json:"oidc"`
	} `json:"auth"`
}

func authProvidersHandler(c *fiber.Ctx) error {
	cfg := c.Locals("config").(*config.Config)

	var resp authProvidersResponse
	resp.Success = true
	resp.Auth.Enabled = cfg.Auth.Enabled
	resp.Auth.Local.Enabled = cfg.Auth.Enabled && cfg.Auth.Local.Enabled
	resp.Auth.OIDC.Enabled = cfg.Auth.Enabled && cfg.Auth.OIDC.Enabled
	resp.Auth.OIDC.Issuer = strings.TrimSpace(cfg.Auth.OIDC.IssuerURL)

	return c.Status(fiber.StatusOK).JSON(resp)
}
