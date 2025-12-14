package http

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"

	"raito/internal/config"
	"raito/internal/services"
)

func mapHandler(c *fiber.Ctx) error {
	var reqBody MapRequest
	if err := c.BodyParser(&reqBody); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(MapResponse{
			Success: false,
			Links:   []MapLink{},
			Code:    "BAD_REQUEST_INVALID_JSON",
			Error:   "Bad request, malformed JSON",
		})
	}

	if reqBody.URL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(MapResponse{
			Success: false,
			Links:   []MapLink{},
			Code:    "BAD_REQUEST",
			Error:   "Missing required field 'url'",
		})
	}

	cfg := c.Locals("config").(*config.Config)

	// Derive timeout from request and config
	timeoutMs := cfg.Scraper.TimeoutMs
	if reqBody.Timeout != nil && *reqBody.Timeout > 0 {
		timeoutMs = *reqBody.Timeout
	}

	// Prefer the job queue-backed executor when available so API-only
	// nodes remain lightweight and workers perform discovery.
	if execVal := c.Locals("executor"); execVal != nil {
		if exec, ok := execVal.(WorkExecutor); ok && exec != nil {
			ctx, cancel := context.WithTimeout(c.Context(), time.Duration(timeoutMs)*time.Millisecond)
			defer cancel()

			res, err := exec.Map(ctx, &reqBody)
			if err != nil {
				status := http.StatusBadGateway
				if errors.Is(err, context.DeadlineExceeded) {
					status = http.StatusGatewayTimeout
				}
				return c.Status(status).JSON(MapResponse{
					Success: false,
					Links:   []MapLink{},
					Code:    "MAP_FAILED",
					Error:   err.Error(),
				})
			}
			if res == nil {
				return c.Status(fiber.StatusInternalServerError).JSON(MapResponse{
					Success: false,
					Links:   []MapLink{},
					Code:    "MAP_FAILED",
					Error:   "empty map response",
				})
			}

			status := http.StatusOK
			if !res.Success {
				status = http.StatusBadGateway
				if res.Code == "MAP_TIMEOUT" || res.Code == "JOB_NOT_STARTED" {
					status = http.StatusGatewayTimeout
				}
			}
			return c.Status(status).JSON(res)

		}
	}

	// Fallback: run map inline when no executor is configured.
	// Derive options from request and config
	limit := cfg.Crawler.MaxPagesDefault
	if reqBody.Limit != nil && *reqBody.Limit > 0 {
		limit = *reqBody.Limit
	}

	includeSubdomains := false
	if reqBody.IncludeSubdomains != nil {
		includeSubdomains = *reqBody.IncludeSubdomains
	}

	ignoreQueryParams := true
	if reqBody.IgnoreQueryParams != nil {
		ignoreQueryParams = *reqBody.IgnoreQueryParams
	}

	allowExternal := false
	if reqBody.AllowExternal != nil {
		allowExternal = *reqBody.AllowExternal
	}

	sitemapMode := reqBody.Sitemap
	if sitemapMode == "" {
		sitemapMode = "include"
	}

	svc := services.NewMapService(cfg)
	svcReq := &services.MapRequest{
		URL:               reqBody.URL,
		Limit:             limit,
		Search:            reqBody.Search,
		IncludeSubdomains: includeSubdomains,
		IgnoreQueryParams: ignoreQueryParams,
		AllowExternal:     allowExternal,
		SitemapMode:       sitemapMode,
		TimeoutMs:         timeoutMs,
	}

	res, err := svc.Map(c.Context(), svcReq)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		return c.Status(status).JSON(MapResponse{
			Success: false,
			Links:   []MapLink{},
			Code:    "MAP_FAILED",
			Error:   err.Error(),
		})
	}

	linksResp := make([]MapLink, 0, len(res.Links))
	for _, l := range res.Links {
		linksResp = append(linksResp, MapLink{
			URL:         l.URL,
			Title:       l.Title,
			Description: l.Description,
		})
	}

	return c.Status(http.StatusOK).JSON(MapResponse{
		Success: true,
		Links:   linksResp,
		Warning: res.Warning,
	})
}
