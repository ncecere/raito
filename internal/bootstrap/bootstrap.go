package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"raito/internal/config"
	"raito/internal/db"
	"raito/internal/store"
)

// Run applies bootstrap configuration for users and tenants. It is
// designed to be idempotent and safe to run multiple times.
func Run(ctx context.Context, cfg *config.Config, st *store.Store) error {
	if cfg == nil || st == nil {
		return nil
	}
	if len(cfg.Bootstrap.Users) == 0 && len(cfg.Bootstrap.Tenants) == 0 {
		return nil
	}

	q := db.New(st.DB)

	// Bootstrap users first so that tenant membership references are valid.
	for i := range cfg.Bootstrap.Users {
		if err := bootstrapUser(ctx, q, &cfg.Bootstrap.Users[i]); err != nil {
			return err
		}
	}

	// Bootstrap tenants and membership.
	for i := range cfg.Bootstrap.Tenants {
		if err := bootstrapTenant(ctx, q, &cfg.Bootstrap.Tenants[i], cfg.Bootstrap.Users); err != nil {
			return err
		}
	}

	return nil
}

func bootstrapUser(ctx context.Context, q *db.Queries, u *config.BootstrapUserConfig) error {
	email := strings.TrimSpace(strings.ToLower(u.Email))
	if email == "" {
		return nil
	}

	provider := strings.ToLower(strings.TrimSpace(u.Provider))
	if provider == "" {
		provider = "local"
	}

	_, err := q.GetUserByEmail(ctx, email)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil {
		// User already exists; do not modify existing credentials or flags
		// via bootstrap to avoid surprising changes.
		return nil
	}

	userID := uuid.New()
	name := sql.NullString{}
	if strings.TrimSpace(u.Name) != "" {
		name = sql.NullString{String: u.Name, Valid: true}
	}
	passwordHash := sql.NullString{}
	passwordVersion := sql.NullInt32{}
	if provider == "local" && strings.TrimSpace(u.Password) != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		passwordHash = sql.NullString{String: string(hash), Valid: true}
		passwordVersion = sql.NullInt32{Int32: 1, Valid: true}
	}

	_, err = q.CreateUser(ctx, db.CreateUserParams{
		ID:              userID,
		Email:           email,
		Name:            name,
		AuthProvider:    provider,
		AuthSubject:     sql.NullString{},
		IsSystemAdmin:   u.IsSystemAdmin,
		PasswordHash:    passwordHash,
		PasswordVersion: passwordVersion,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// Another process created this user concurrently; treat as success.
			return nil
		}
		return err
	}
	return nil
}

func bootstrapTenant(ctx context.Context, q *db.Queries, t *config.BootstrapTenantConfig, users []config.BootstrapUserConfig) error {
	slug := strings.TrimSpace(t.Slug)
	if slug == "" {
		return nil
	}

	tenant, err := q.GetTenantBySlug(ctx, slug)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == sql.ErrNoRows {
		// Create new tenant
		id := uuid.New()
		name := t.Name
		if strings.TrimSpace(name) == "" {
			name = slug
		}
		typeVal := t.Type
		if strings.TrimSpace(typeVal) == "" {
			typeVal = "org"
		}
		tenant, err = q.CreateTenant(ctx, db.CreateTenantParams{
			ID:          id,
			Slug:        slug,
			Name:        name,
			Type:        typeVal,
			OwnerUserID: uuid.NullUUID{},
		})
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				// Tenant was created concurrently; fetch the existing row.
				tenant, err = q.GetTenantBySlug(ctx, slug)
				if err != nil {
					return err
				}
			} else {
				return err
			}
		}
	}

	// Helper to ensure a user exists by email; reuse bootstrapUser for
	// new users, then re-fetch.
	ensureUser := func(email string) (db.User, error) {
		email = strings.TrimSpace(strings.ToLower(email))
		if email == "" {
			return db.User{}, sql.ErrNoRows
		}
		u, err := q.GetUserByEmail(ctx, email)
		if err == nil {
			return u, nil
		}
		if err != sql.ErrNoRows {
			return db.User{}, err
		}

		// Try to find bootstrap spec for this user.
		var spec *config.BootstrapUserConfig
		for i := range users {
			if strings.EqualFold(strings.TrimSpace(users[i].Email), email) {
				spec = &users[i]
				break
			}
		}
		if spec == nil {
			// Create a minimal OIDC-backed stub.
			stub := config.BootstrapUserConfig{Email: email, Provider: "oidc"}
			if err := bootstrapUser(ctx, q, &stub); err != nil {
				return db.User{}, err
			}
		} else {
			if err := bootstrapUser(ctx, q, spec); err != nil {
				return db.User{}, err
			}
		}
		return q.GetUserByEmail(ctx, email)
	}

	// Add admins
	for _, email := range t.Admins {
		u, err := ensureUser(email)
		if err != nil {
			return err
		}
		_, _ = q.AddTenantMember(ctx, db.AddTenantMemberParams{
			TenantID: tenant.ID,
			UserID:   u.ID,
			Role:     "tenant_admin",
		})
	}

	// Add members
	for _, email := range t.Members {
		u, err := ensureUser(email)
		if err != nil {
			return err
		}
		_, _ = q.AddTenantMember(ctx, db.AddTenantMemberParams{
			TenantID: tenant.ID,
			UserID:   u.ID,
			Role:     "tenant_member",
		})
	}

	return nil
}
