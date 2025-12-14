package http

import (
	"database/sql"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/oauth2"

	"raito/internal/config"
	"raito/internal/db"
	"raito/internal/services"
	"raito/internal/store"
)

const oidcStateCookieName = "raito_oidc_state"

type LocalLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LocalLoginResponse struct {
	Success    bool   `json:"success"`
	Code       string `json:"code,omitempty"`
	Error      string `json:"error,omitempty"`
	FirstLogin bool   `json:"firstLogin,omitempty"`
}

type OIDCLoginResponse struct {
	Success    bool   `json:"success"`
	Code       string `json:"code,omitempty"`
	Error      string `json:"error,omitempty"`
	FirstLogin bool   `json:"firstLogin,omitempty"`
}

func loginHandler(c *fiber.Ctx) error {
	cfg := c.Locals("config").(*config.Config)
	st := c.Locals("store").(*store.Store)

	if !cfg.Auth.Local.Enabled {
		return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
			Success: false,
			Code:    "LOCAL_AUTH_DISABLED",
			Error:   "local auth is disabled in server configuration",
		})
	}

	var req LocalLoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST_INVALID_JSON",
			Error:   "Bad request, malformed JSON",
		})
	}

	authSvc := services.NewAuthService(cfg, st)
	res, err := authSvc.LoginLocal(c.Context(), req.Email, req.Password)
	if err != nil {
		switch err {
		case services.ErrInvalidCredentials:
			return c.Status(fiber.StatusUnauthorized).JSON(LocalLoginResponse{
				Success: false,
				Code:    "INVALID_CREDENTIALS",
				Error:   "invalid email or password",
			})
		case services.ErrAuthProviderMismatch:
			return c.Status(fiber.StatusBadRequest).JSON(LocalLoginResponse{
				Success: false,
				Code:    "AUTH_PROVIDER_MISMATCH",
				Error:   "user exists but is not configured for local auth",
			})
		default:
			// Surface DB not-found errors as invalid creds for security.
			if err == sql.ErrNoRows {
				return c.Status(fiber.StatusUnauthorized).JSON(LocalLoginResponse{
					Success: false,
					Code:    "INVALID_CREDENTIALS",
					Error:   "invalid email or password",
				})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(LocalLoginResponse{
				Success: false,
				Code:    "INTERNAL_ERROR",
				Error:   err.Error(),
			})
		}
	}

	// Issue a browser session cookie for UI clients.
	var defaultTenantID *uuid.UUID
	{
		q := db.New(st.DB)
		personalTenants, err := q.ListPersonalTenantsForUser(c.Context(), uuid.NullUUID{UUID: res.User.ID, Valid: true})
		if err == nil && len(personalTenants) > 0 {
			id := personalTenants[0].ID
			defaultTenantID = &id
		}
	}
	_ = issueSessionCookie(c, cfg, res.User.ID, defaultTenantID, res.User.IsSystemAdmin)

	return c.Status(fiber.StatusOK).JSON(LocalLoginResponse{
		Success:    true,
		FirstLogin: res.FirstLogin,
	})
}

func logoutHandler(c *fiber.Ctx) error {
	// Stateless placeholder for now; when a session/JWT mechanism is
	// introduced, this can be extended to revoke tokens or clear
	// server-side sessions.
	// Clear session cookie for browser clients.
	cfg := c.Locals("config").(*config.Config)
	name := cfg.Auth.Session.CookieName
	if name == "" {
		name = "raito_session"
	}
	c.Cookie(&fiber.Cookie{
		Name:     name,
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Lax",
	})

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
	})
}

// oidcLoginStartHandler initiates an OIDC login by redirecting the
// user agent to the provider's authorization endpoint with a
// cookie-backed state value for CSRF protection.
func oidcLoginStartHandler(c *fiber.Ctx) error {
	cfg := c.Locals("config").(*config.Config)

	if !cfg.Auth.OIDC.Enabled {
		return c.Status(fiber.StatusServiceUnavailable).JSON(OIDCLoginResponse{
			Success: false,
			Code:    "OIDC_DISABLED",
			Error:   "oidc auth is disabled in server configuration",
		})
	}

	provider, err := oidc.NewProvider(c.Context(), cfg.Auth.OIDC.IssuerURL)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(OIDCLoginResponse{
			Success: false,
			Code:    "OIDC_PROVIDER_ERROR",
			Error:   err.Error(),
		})
	}

	oauthCfg := oauth2.Config{
		ClientID:     cfg.Auth.OIDC.ClientID,
		ClientSecret: cfg.Auth.OIDC.ClientSecret,
		RedirectURL:  cfg.Auth.OIDC.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

	state := uuid.NewString()
	c.Cookie(&fiber.Cookie{
		Name:     oidcStateCookieName,
		Value:    state,
		Expires:  time.Now().Add(10 * time.Minute),
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Lax",
	})

	authURL := oauthCfg.AuthCodeURL(state, oauth2.AccessTypeOnline)
	return c.Redirect(authURL, fiber.StatusFound)
}

// oidcCallbackHandler handles the OIDC redirect, validates state, and
// delegates to AuthService.LoginOIDC to upsert the user + personal tenant.
func oidcCallbackHandler(c *fiber.Ctx) error {
	cfg := c.Locals("config").(*config.Config)
	st := c.Locals("store").(*store.Store)

	if !cfg.Auth.OIDC.Enabled {
		return c.Status(fiber.StatusServiceUnavailable).JSON(OIDCLoginResponse{
			Success: false,
			Code:    "OIDC_DISABLED",
			Error:   "oidc auth is disabled in server configuration",
		})
	}

	code := c.Query("code")
	state := c.Query("state")
	if code == "" || state == "" {
		return c.Status(fiber.StatusBadRequest).JSON(OIDCLoginResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "missing code or state",
		})
	}

	cookieState := c.Cookies(oidcStateCookieName)
	if cookieState == "" || cookieState != state {
		return c.Status(fiber.StatusBadRequest).JSON(OIDCLoginResponse{
			Success: false,
			Code:    "OIDC_STATE_MISMATCH",
			Error:   "oidc state mismatch",
		})
	}

	// Clear the state cookie.
	c.Cookie(&fiber.Cookie{
		Name:     oidcStateCookieName,
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Lax",
	})

	authSvc := services.NewAuthService(cfg, st)
	res, err := authSvc.LoginOIDC(c.Context(), code, state)
	if err != nil {
		switch err {
		case services.ErrOIDCDisabled:
			return c.Status(fiber.StatusServiceUnavailable).JSON(OIDCLoginResponse{
				Success: false,
				Code:    "OIDC_DISABLED",
				Error:   "oidc auth is disabled in server configuration",
			})
		case services.ErrOIDCEmailMissing:
			return c.Status(fiber.StatusBadRequest).JSON(OIDCLoginResponse{
				Success: false,
				Code:    "OIDC_EMAIL_MISSING",
				Error:   "oidc id token did not contain an email",
			})
		case services.ErrOIDCEmailNotAllowed:
			return c.Status(fiber.StatusForbidden).JSON(OIDCLoginResponse{
				Success: false,
				Code:    "OIDC_EMAIL_NOT_ALLOWED",
				Error:   "email domain is not allowed for oidc",
			})
		case services.ErrAuthProviderMismatch:
			return c.Status(fiber.StatusBadRequest).JSON(OIDCLoginResponse{
				Success: false,
				Code:    "AUTH_PROVIDER_MISMATCH",
				Error:   "user exists but is not configured for oidc auth",
			})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(OIDCLoginResponse{
				Success: false,
				Code:    "INTERNAL_ERROR",
				Error:   err.Error(),
			})
		}
	}

	// Issue a browser session cookie for UI clients.
	var defaultTenantID *uuid.UUID
	{
		q := db.New(st.DB)
		personalTenants, err := q.ListPersonalTenantsForUser(c.Context(), uuid.NullUUID{UUID: res.User.ID, Valid: true})
		if err == nil && len(personalTenants) > 0 {
			id := personalTenants[0].ID
			defaultTenantID = &id
		}
	}
	_ = issueSessionCookie(c, cfg, res.User.ID, defaultTenantID, res.User.IsSystemAdmin)

	return c.Status(fiber.StatusOK).JSON(OIDCLoginResponse{
		Success:    true,
		FirstLogin: res.FirstLogin,
	})
}
