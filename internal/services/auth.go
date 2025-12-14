package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"

	"github.com/google/uuid"

	"raito/internal/config"
	"raito/internal/db"
	"raito/internal/store"
)

var (
	ErrInvalidCredentials   = errors.New("invalid email or password")
	ErrAuthProviderMismatch = errors.New("user exists but is not configured for this auth method")
	ErrOIDCDisabled         = errors.New("oidc auth is disabled")
	ErrOIDCEmailNotAllowed  = errors.New("email domain is not allowed for oidc")
	ErrOIDCEmailMissing     = errors.New("oidc token did not contain an email")
)

// AuthService encapsulates user login flows (local and OIDC).
type AuthService interface {
	LoginLocal(ctx context.Context, email, password string) (*LocalAuthResult, error)
	LoginOIDC(ctx context.Context, code, state string) (*OIDCAuthResult, error)
}

type LocalAuthResult struct {
	User       db.User
	FirstLogin bool
}

type OIDCAuthResult struct {
	User       db.User
	FirstLogin bool
}

type authService struct {
	cfg *config.Config
	st  *store.Store
}

func NewAuthService(cfg *config.Config, st *store.Store) AuthService {
	return &authService{cfg: cfg, st: st}
}

// LoginOIDC performs an OIDC authorization code flow token exchange
// and upserts a user + personal tenant based on the ID token claims.
func (s *authService) LoginOIDC(ctx context.Context, code, state string) (*OIDCAuthResult, error) {
	if !s.cfg.Auth.OIDC.Enabled {
		return nil, ErrOIDCDisabled
	}

	provider, err := oidc.NewProvider(ctx, s.cfg.Auth.OIDC.IssuerURL)
	if err != nil {
		return nil, err
	}

	oauthCfg := oauth2.Config{
		ClientID:     s.cfg.Auth.OIDC.ClientID,
		ClientSecret: s.cfg.Auth.OIDC.ClientSecret,
		RedirectURL:  s.cfg.Auth.OIDC.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

	token, err := oauthCfg.Exchange(ctx, code)
	if err != nil {
		return nil, err
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return nil, errors.New("oidc: id_token not found in token response")
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: s.cfg.Auth.OIDC.ClientID})
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, err
	}

	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, err
	}

	email := strings.TrimSpace(strings.ToLower(claims.Email))
	if email == "" {
		return nil, ErrOIDCEmailMissing
	}

	// Enforce allowed domains if configured.
	if len(s.cfg.Auth.OIDC.AllowedDomains) > 0 {
		domain := ""
		if i := strings.LastIndex(email, "@"); i != -1 && i+1 < len(email) {
			domain = email[i+1:]
		}
		allowed := false
		for _, d := range s.cfg.Auth.OIDC.AllowedDomains {
			if strings.EqualFold(strings.TrimSpace(d), domain) {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, ErrOIDCEmailNotAllowed
		}
	}

	q := db.New(s.st.DB)

	// First, try to find by provider + subject.
	subject := sql.NullString{String: idToken.Subject, Valid: idToken.Subject != ""}
	if subject.Valid {
		user, err := q.GetUserByProviderSubject(ctx, db.GetUserByProviderSubjectParams{
			AuthProvider: "oidc",
			AuthSubject:  subject,
		})
		if err == nil {
			if err := s.ensurePersonalTenantForUser(ctx, q, user); err != nil {
				return nil, err
			}
			return &OIDCAuthResult{User: user, FirstLogin: false}, nil
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}

	// Next, see if a user already exists for this email.
	existing, err := q.GetUserByEmail(ctx, email)
	if err == nil {
		// User exists but not wired for OIDC with this subject.
		if existing.AuthProvider != "oidc" || !existing.AuthSubject.Valid || existing.AuthSubject.String != subject.String {
			return nil, ErrAuthProviderMismatch
		}
		if err := s.ensurePersonalTenantForUser(ctx, q, existing); err != nil {
			return nil, err
		}
		return &OIDCAuthResult{User: existing, FirstLogin: false}, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	// Create a new OIDC-backed user.
	userID := uuid.New()
	user, err := q.CreateUser(ctx, db.CreateUserParams{
		ID:              userID,
		Email:           email,
		Name:            sql.NullString{},
		AuthProvider:    "oidc",
		AuthSubject:     subject,
		IsSystemAdmin:   false,
		PasswordHash:    sql.NullString{},
		PasswordVersion: sql.NullInt32{},
	})
	if err != nil {
		// If another concurrent login created the user, try refetching.
		u2, getErr := q.GetUserByEmail(ctx, email)
		if getErr != nil {
			return nil, err
		}
		user = u2
	}

	if err := s.ensurePersonalTenantForUser(ctx, q, user); err != nil {
		return nil, err
	}

	return &OIDCAuthResult{User: user, FirstLogin: true}, nil
}

// ensurePersonalTenantForUser makes sure the given user has a personal
// tenant and is a tenant_admin of it.
func (s *authService) ensurePersonalTenantForUser(ctx context.Context, q *db.Queries, user db.User) error {
	owner := uuid.NullUUID{UUID: user.ID, Valid: true}

	tenants, err := q.ListPersonalTenantsForUser(ctx, owner)
	if err != nil {
		return err
	}
	if len(tenants) > 0 {
		return nil
	}

	tenantID := uuid.New()
	slug := generatePersonalTenantSlug(user.Email, tenantID)
	_, err = q.CreateTenant(ctx, db.CreateTenantParams{
		ID:          tenantID,
		Slug:        slug,
		Name:        user.Email,
		Type:        "personal",
		OwnerUserID: owner,
	})
	if err != nil {
		return err
	}

	_, err = q.AddTenantMember(ctx, db.AddTenantMemberParams{
		TenantID: tenantID,
		UserID:   user.ID,
		Role:     "tenant_admin",
	})
	return err
}

func (s *authService) LoginLocal(ctx context.Context, email, password string) (*LocalAuthResult, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || password == "" {
		return nil, ErrInvalidCredentials
	}

	q := db.New(s.st.DB)

	// Try to find existing user by email.
	user, err := q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// First login for this email: create user + personal tenant.
			return s.createLocalUserWithPersonalTenant(ctx, q, email, password)
		}
		return nil, err
	}

	if user.AuthProvider != "local" {
		return nil, ErrAuthProviderMismatch
	}
	if !user.PasswordHash.Valid {
		return nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash.String), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	if err := s.ensurePersonalTenantForUser(ctx, q, user); err != nil {
		return nil, err
	}

	return &LocalAuthResult{User: user, FirstLogin: false}, nil
}

func (s *authService) createLocalUserWithPersonalTenant(ctx context.Context, q *db.Queries, email, password string) (*LocalAuthResult, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	_ = time.Now().UTC() // reserved for future audit fields
	userID := uuid.New()

	user, err := q.CreateUser(ctx, db.CreateUserParams{
		ID:              userID,
		Email:           email,
		Name:            sql.NullString{},
		AuthProvider:    "local",
		AuthSubject:     sql.NullString{},
		IsSystemAdmin:   false,
		PasswordHash:    sql.NullString{String: string(hash), Valid: true},
		PasswordVersion: sql.NullInt32{Int32: 1, Valid: true},
	})
	if err != nil {
		// If another concurrent login created the user, try to refetch.
		u2, getErr := q.GetUserByEmail(ctx, email)
		if getErr != nil {
			return nil, err
		}
		user = u2
	}

	// Create a personal tenant for this user.
	tenantID := uuid.New()
	slug := generatePersonalTenantSlug(email, tenantID)
	_, err = q.CreateTenant(ctx, db.CreateTenantParams{
		ID:          tenantID,
		Slug:        slug,
		Name:        email,
		Type:        "personal",
		OwnerUserID: uuid.NullUUID{UUID: userID, Valid: true},
	})
	if err != nil {
		return nil, err
	}

	// Add tenant membership as tenant_admin.
	_, err = q.AddTenantMember(ctx, db.AddTenantMemberParams{
		TenantID: tenantID,
		UserID:   userID,
		Role:     "tenant_admin",
	})
	if err != nil {
		return nil, err
	}

	return &LocalAuthResult{User: user, FirstLogin: true}, nil
}

func generatePersonalTenantSlug(email string, id uuid.UUID) string {
	local := email
	if i := strings.Index(local, "@"); i > 0 {
		local = local[:i]
	}
	local = strings.ReplaceAll(local, " ", "-")
	local = strings.ToLower(local)
	if local == "" {
		local = "user"
	}
	return fmt.Sprintf("%s-%s", local, id.String())
}
