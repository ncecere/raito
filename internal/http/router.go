package http

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"raito/internal/config"
	"raito/internal/metrics"
	"raito/internal/store"
)

type Server struct {
	app    *fiber.App
	config *config.Config
	store  *store.Store
	logger *slog.Logger
}

func NewServer(cfg *config.Config, st *store.Store, logger *slog.Logger) *Server {
	app := fiber.New()

	// Construct a job queue-backed executor for heavy operations
	exec := NewJobQueueExecutor(cfg, st, logger)

	// Inject config, store, and executor into context for handlers
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("config", cfg)
		c.Locals("store", st)
		c.Locals("executor", exec)
		return c.Next()
	})

	// Request logging + metrics middleware
	app.Use(func(c *fiber.Ctx) error {
		start := time.Now()

		// Ensure a request ID exists
		reqID := c.Get("X-Request-Id")
		if reqID == "" {
			reqID = uuid.New().String()
		}
		c.Locals("request_id", reqID)
		if logger != nil {
			c.Locals("logger", logger)
		}

		err := c.Next()

		latency := time.Since(start)
		status := c.Response().StatusCode()
		method := c.Method()
		path := c.Path()

		metrics.RecordRequest(method, path, status, latency.Milliseconds())

		if logger != nil {
			attrs := []any{
				"request_id", reqID,
				"method", method,
				"path", path,
				"status", status,
				"latency_ms", latency.Milliseconds(),
			}
			if provVal := c.Locals("llm_provider"); provVal != nil {
				attrs = append(attrs, "llm_provider", provVal)
			}
			if modelVal := c.Locals("llm_model"); modelVal != nil {
				attrs = append(attrs, "llm_model", modelVal)
			}
			logger.Info("request", attrs...)
		}

		return err
	})

	// Redis client for rate limiting and health checks
	var rdb *redis.Client
	if cfg.Auth.Enabled && cfg.Redis.URL != "" {
		if opt, err := redis.ParseURL(cfg.Redis.URL); err == nil {
			rdb = redis.NewClient(opt)
		}
	}

	// Health endpoints
	app.Get("/healthz", func(c *fiber.Ctx) error {
		// Shallow health: process is up
		if c.Query("deep") != "true" {
			return c.JSON(fiber.Map{"status": "ok"})
		}

		// Deep health: check DB and Redis connectivity, and rod configuration.
		ctx, cancel := context.WithTimeout(c.Context(), 2*time.Second)
		defer cancel()

		dbStatus := "ok"
		if err := st.DB.PingContext(ctx); err != nil {
			dbStatus = "error"
		}

		redisStatus := "disabled"
		if rdb != nil {
			if err := rdb.Ping(ctx).Err(); err != nil {
				redisStatus = "error"
			} else {
				redisStatus = "ok"
			}
		}

		rodStatus := "disabled"
		if cfg.Rod.Enabled {
			// For now, just report that rod is enabled; a full browser connectivity
			// check would be more expensive and is left as a future enhancement.
			rodStatus = "enabled"
		}

		status := "ok"
		if dbStatus != "ok" || redisStatus == "error" {
			status = "error"
		}

		return c.JSON(fiber.Map{
			"status": status,
			"db":     dbStatus,
			"redis":  redisStatus,
			"rod":    rodStatus,
		})
	})

	// Prometheus-style metrics endpoint
	app.Get("/metrics", func(c *fiber.Ctx) error {
		c.Type("text/plain")
		return c.SendString(metrics.Export())
	})

	authMw := authMiddleware(cfg, st)
	var rateMw fiber.Handler
	if rdb != nil {
		rateMw = rateLimitMiddleware(cfg, rdb)
	} else {
		rateMw = func(c *fiber.Ctx) error { return c.Next() }
	}

	v1 := app.Group("/v1", authMw, rateMw)
	registerV1Routes(v1)

	admin := app.Group("/admin", authMw, adminOnlyMiddleware)
	registerAdminRoutes(admin)

	return &Server{
		app:    app,
		config: cfg,
		store:  st,
		logger: logger,
	}
}

func (s *Server) Listen() error {
	addr := fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.Port)
	return s.app.Listen(addr)
}

func registerV1Routes(group fiber.Router) {
	group.Post("/scrape", scrapeHandler)
	group.Post("/map", mapHandler)
	group.Post("/crawl", crawlHandler)
	group.Get("/crawl/:id", crawlStatusHandler)
	group.Post("/extract", extractHandler)
	group.Get("/extract/:id", extractStatusHandler)
	group.Post("/batch/scrape", batchScrapeHandler)
	group.Get("/batch/scrape/:id", batchScrapeStatusHandler)
	group.Post("/search", searchHandler)
}
