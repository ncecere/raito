package migrate

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Run applies all pending migrations in db/migrations using goose.
// It opens and closes its own DB handle so it is independent of the app store.
func Run(dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	// On fresh docker-compose startup, Postgres may not be ready immediately.
	// Do a short retry loop to avoid failing hard on initial connection refusal.
	deadline := time.Now().Add(30 * time.Second)
	for {
		if err := db.Ping(); err == nil {
			break
		}
		if time.Now().After(deadline) {
			if err := db.Ping(); err != nil {
				return fmt.Errorf("db not ready: %w", err)
			}
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}

	migrationsDir := "db/migrations"
	if err := goose.Up(db, migrationsDir); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}

	return nil
}
