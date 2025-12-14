package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"raito/internal/config"
	"raito/internal/metrics"
	"raito/internal/search"
	"raito/internal/services"
)

func searchHandler(c *fiber.Ctx) error {
	var reqBody SearchRequest
	if err := c.BodyParser(&reqBody); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST_INVALID_JSON",
			Error:   "Bad request, malformed JSON",
		})
	}

	if strings.TrimSpace(reqBody.Query) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "Missing required field 'query'",
		})
	}

	cfg := c.Locals("config").(*config.Config)

	if !cfg.Search.Enabled {
		return c.Status(http.StatusServiceUnavailable).JSON(ErrorResponse{
			Success: false,
			Code:    "SEARCH_DISABLED",
			Error:   "search is disabled in server configuration",
		})
	}

	// Determine sources; v1 currently only supports "web".
	sources := reqBody.Sources
	if len(sources) == 0 {
		sources = []string{"web"}
	} else {
		for _, s := range sources {
			if strings.ToLower(strings.TrimSpace(s)) != "web" {
				return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
					Success: false,
					Code:    "UNSUPPORTED_SOURCE",
					Error:   "only 'web' source is supported in this version",
				})
			}
		}
	}

	// Derive limit from request and config defaults.
	limit := cfg.Search.MaxResults
	if limit <= 0 {
		limit = 5
	}
	if reqBody.Limit != nil && *reqBody.Limit > 0 {
		limit = *reqBody.Limit
	}
	if cfg.Search.MaxResults > 0 && limit > cfg.Search.MaxResults {
		limit = cfg.Search.MaxResults
	}

	// Derive timeout for the overall search operation.
	timeoutMs := cfg.Search.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = cfg.Scraper.TimeoutMs
	}
	if reqBody.Timeout != nil && *reqBody.Timeout > 0 {
		timeoutMs = *reqBody.Timeout
	}
	if timeoutMs <= 0 {
		timeoutMs = 60000
	}

	ignoreInvalid := false
	if reqBody.IgnoreInvalidURLs != nil {
		ignoreInvalid = *reqBody.IgnoreInvalidURLs
	}

	// For /v1/search, only a limited set of formats are supported
	// when scrapeOptions are provided. Reject unsupported formats
	// early so clients get a clear error instead of silently ignored
	// options or unexpectedly large payloads.
	if reqBody.ScrapeOptions != nil && len(reqBody.ScrapeOptions.Formats) > 0 {
		allowed := map[string]struct{}{
			"markdown": {},
			"html":     {},
			"rawhtml":  {},
		}

		for _, f := range reqBody.ScrapeOptions.Formats {
			formatName := ""
			switch v := f.(type) {
			case string:
				formatName = strings.ToLower(v)
			case map[string]interface{}:
				if t, ok := v["type"].(string); ok {
					formatName = strings.ToLower(t)
				}
			default:
				// Unknown format shape; treat as unsupported.
			}

			if formatName == "" {
				return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
					Success: false,
					Code:    "UNSUPPORTED_FORMAT",
					Error:   "Unsupported format for /v1/search; allowed formats are: markdown, html, rawHtml",
				})
			}

			if _, ok := allowed[formatName]; !ok {
				return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
					Success: false,
					Code:    "UNSUPPORTED_FORMAT",
					Error:   fmt.Sprintf("Unsupported format %q for /v1/search; allowed formats are: markdown, html, rawHtml", formatName),
				})
			}
		}
	}

	hasScrape := reqBody.ScrapeOptions != nil

	// Fast path: search-only (no scrapeOptions); use SearchService and
	// return immediately.
	if !hasScrape {
		ctx, cancel := context.WithTimeout(c.Context(), time.Duration(timeoutMs)*time.Millisecond)
		defer cancel()

		svc := services.NewSearchService(cfg)
		res, err := svc.Search(ctx, &services.SearchRequest{
			Query:             reqBody.Query,
			Sources:           sources,
			Limit:             limit,
			Country:           reqBody.Country,
			Location:          reqBody.Location,
			TBS:               reqBody.TBS,
			TimeoutMs:         timeoutMs,
			IgnoreInvalidURLs: ignoreInvalid,
		})
		if err != nil {
			status := http.StatusBadGateway
			if errors.Is(err, context.DeadlineExceeded) {
				status = http.StatusGatewayTimeout
			}
			return c.Status(status).JSON(ErrorResponse{
				Success: false,
				Code:    "SEARCH_FAILED",
				Error:   err.Error(),
			})
		}

		web := make([]SearchWebResult, 0, len(res.Web))
		for _, r := range res.Web {
			web = append(web, SearchWebResult{
				Title:       r.Title,
				Description: r.Description,
				URL:         r.URL,
			})
		}

		providerName := strings.TrimSpace(res.ProviderName)
		if providerName == "" {
			providerName = strings.ToLower(strings.TrimSpace(cfg.Search.Provider))
			if providerName == "" {
				providerName = "searxng"
			}
		}

		metrics.RecordSearch(providerName, false, len(web), 0)

		if loggerVal := c.Locals("logger"); loggerVal != nil {
			if lg, ok := loggerVal.(interface{ Info(msg string, args ...any) }); ok {
				attrs := []any{
					"query", reqBody.Query,
					"provider", providerName,
					"sources", strings.Join(sources, ","),
					"limit", limit,
					"results", len(web),
					"scraped_results", 0,
					"invalid_url_results", 0,
					"scrape_error_results", 0,
					"ignore_invalid_urls", ignoreInvalid,
				}
				lg.Info("search_request", attrs...)
			}
		}

		resp := SearchResponse{
			Success: true,
			Data: &SearchData{
				Web: web,
			},
		}

		return c.Status(http.StatusOK).JSON(resp)
	}

	// Scrape path: use SearchService to scrape results while preserving
	// existing error mapping, metrics, and response shape.
	ctx, cancel := context.WithTimeout(c.Context(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	provider, err := search.NewProviderFromConfig(cfg)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "SEARCH_PROVIDER_ERROR",
			Error:   err.Error(),
		})
	}

	searchReq := &search.Request{
		Query:            reqBody.Query,
		Sources:          sources,
		Limit:            limit,
		Country:          reqBody.Country,
		Location:         reqBody.Location,
		TBS:              reqBody.TBS,
		Timeout:          time.Duration(timeoutMs) * time.Millisecond,
		IgnoreInvalidURL: ignoreInvalid,
	}

	results, err := provider.Search(ctx, searchReq)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		return c.Status(status).JSON(ErrorResponse{
			Success: false,
			Code:    "SEARCH_FAILED",
			Error:   err.Error(),
		})
	}

	// Enforce the effective limit at the API layer as a
	// defensive measure in case the provider returns more
	// results than requested.
	if limit > 0 && len(results.Web) > limit {
		results.Web = results.Web[:limit]
	}

	svc := services.NewSearchService(cfg)
	var locOpts *services.LocationOptions
	if reqBody.ScrapeOptions != nil && reqBody.ScrapeOptions.Location != nil {
		loc := reqBody.ScrapeOptions.Location
		locOpts = &services.LocationOptions{
			Country:   loc.Country,
			Languages: loc.Languages,
		}
	}

	scrapeOpts := &services.SearchScrapeOptions{
		Formats:    nil,
		Headers:    nil,
		UseBrowser: nil,
		Location:   locOpts,
		TimeoutMs:  timeoutMs,
	}
	if reqBody.ScrapeOptions != nil {
		scrapeOpts.Formats = reqBody.ScrapeOptions.Formats
		scrapeOpts.Headers = reqBody.ScrapeOptions.Headers
		scrapeOpts.UseBrowser = reqBody.ScrapeOptions.UseBrowser
	}

	scraped, err := svc.ScrapeResults(ctx, results.Web, scrapeOpts, ignoreInvalid)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		return c.Status(status).JSON(ErrorResponse{
			Success: false,
			Code:    "SEARCH_FAILED",
			Error:   err.Error(),
		})
	}

	web := make([]SearchWebResult, 0, len(scraped.Web))
	for _, r := range scraped.Web {
		entry := SearchWebResult{
			Title:       r.Title,
			Description: r.Description,
			URL:         r.URL,
		}
		if r.Document != nil {
			entry.Document = (*Document)(r.Document)
			entry.Metadata = entry.Document.Metadata
			entry.Engine = entry.Document.Engine
		}
		web = append(web, entry)
	}

	invalidURLCount := scraped.InvalidURLCount
	scrapeErrorCount := scraped.ScrapeErrorCount
	scrapedCount := scraped.ScrapedCount

	warning := ""
	var warningParts []string
	if invalidURLCount > 0 {
		if ignoreInvalid {
			warningParts = append(warningParts, fmt.Sprintf("%d search results had invalid URLs and were dropped", invalidURLCount))
		} else {
			warningParts = append(warningParts, fmt.Sprintf("%d search results had invalid URLs and were returned without documents", invalidURLCount))
		}
	}
	if scrapeErrorCount > 0 {
		if ignoreInvalid {
			warningParts = append(warningParts, fmt.Sprintf("%d search results failed to scrape and were dropped", scrapeErrorCount))
		} else {
			warningParts = append(warningParts, fmt.Sprintf("%d search results failed to scrape; returning partial data", scrapeErrorCount))
		}
	}
	if len(warningParts) > 0 {
		warning = strings.Join(warningParts, "; ")
	}

	providerName := strings.ToLower(strings.TrimSpace(cfg.Search.Provider))
	if providerName == "" {
		providerName = "searxng"
	}
	metrics.RecordSearch(providerName, hasScrape, len(web), scrapedCount)

	if loggerVal := c.Locals("logger"); loggerVal != nil {
		if lg, ok := loggerVal.(interface{ Info(msg string, args ...any) }); ok {
			attrs := []any{
				"query", reqBody.Query,
				"provider", providerName,
				"sources", strings.Join(sources, ","),
				"limit", limit,
				"results", len(web),
				"scraped_results", scrapedCount,
				"invalid_url_results", invalidURLCount,
				"scrape_error_results", scrapeErrorCount,
				"ignore_invalid_urls", ignoreInvalid,
			}
			lg.Info("search_request", attrs...)
		}
	}

	resp := SearchResponse{
		Success: true,
		Data: &SearchData{
			Web: web,
		},
	}
	if warning != "" {
		resp.Warning = warning
	}

	return c.Status(http.StatusOK).JSON(resp)
}
