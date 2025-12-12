package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"log/slog"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"raito/internal/config"
	server "raito/internal/http"
	"raito/internal/migrate"
	"raito/internal/store"
)

func main() {
	configPath := flag.String("config", "config/config.yaml", "path to config file")
	flag.Parse()

	cfg := config.Load(*configPath)

	// Run migrations on a short-lived connection
	if err := migrate.Run(cfg.Database.DSN); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	// Create a shared *sql.DB with pooling for the Store
	db, err := sql.Open("pgx", cfg.Database.DSN)
	if err != nil {
		log.Fatalf("open db failed: %v", err)
	}
	// Basic pool settings; adjust as needed
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)

	st := store.New(db)

	// Ensure initial admin API key if configured
	if cfg.Auth.Enabled && cfg.Auth.InitialAdminKey != "" {
		if _, err := st.EnsureAdminAPIKey(context.Background(), cfg.Auth.InitialAdminKey, "initial-admin"); err != nil {
			log.Fatalf("ensure admin api key failed: %v", err)
		}
	}

	// Set up logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))

	// Start background crawl worker to process pending jobs.
	rootCtx := context.Background()
	server.StartCrawlWorker(rootCtx, cfg, st)

	s := server.NewServer(cfg, st, logger)

	if err := s.Listen(); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
