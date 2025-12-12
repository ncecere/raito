package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"raito/internal/config"
	"raito/internal/crawler"
	"raito/internal/llm"
	"raito/internal/metrics"
	"raito/internal/model"
	"raito/internal/scraper"
	"raito/internal/store"
)

// scrapeHandler implements a minimal Firecrawl v2-compatible scrape endpoint.
// It currently supports basic HTML pages via the HTTP scraper.
func scrapeHandler(c *fiber.Ctx) error {
	var reqBody ScrapeRequest
	if err := c.BodyParser(&reqBody); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST_INVALID_JSON",
			Error:   "Bad request, malformed JSON",
		})
	}

	if reqBody.URL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "Missing required field 'url'",
		})
	}

	cfg := c.Locals("config").(*config.Config)

	timeoutMs := cfg.Scraper.TimeoutMs
	if reqBody.Timeout != nil && *reqBody.Timeout > 0 {
		timeoutMs = *reqBody.Timeout
	}

	// Choose scraper engine: HTTP by default, rod when requested and enabled
	useBrowser := false
	if reqBody.UseBrowser != nil {
		useBrowser = *reqBody.UseBrowser
	}

	var engine scraper.Scraper
	if useBrowser && cfg.Rod.Enabled {
		engine = scraper.NewRodScraper(cfg.Rod.BrowserURL, time.Duration(timeoutMs)*time.Millisecond)
	} else {
		engine = scraper.NewHTTPScraper(time.Duration(timeoutMs) * time.Millisecond)
	}

	headers := map[string]string{}
	for k, v := range reqBody.Headers {
		headers[k] = v
	}

	scrapeReq := scraper.Request{
		URL:       reqBody.URL,
		Headers:   headers,
		Timeout:   time.Duration(timeoutMs) * time.Millisecond,
		UserAgent: cfg.Scraper.UserAgent,
	}

	ctx, cancel := context.WithTimeout(c.Context(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	res, err := engine.Scrape(ctx, scrapeReq)
	if err != nil {
		status := fiber.StatusBadGateway
		if errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		return c.Status(status).JSON(ErrorResponse{
			Success: false,
			Code:    "SCRAPE_FAILED",
			Error:   err.Error(),
		})
	}

	md := Metadata{
		Title:         toString(res.Metadata["title"]),
		Description:   toString(res.Metadata["description"]),
		Language:      toString(res.Metadata["language"]),
		Keywords:      toString(res.Metadata["keywords"]),
		Robots:        toString(res.Metadata["robots"]),
		OgTitle:       toString(res.Metadata["ogTitle"]),
		OgDescription: toString(res.Metadata["ogDescription"]),
		OgURL:         toString(res.Metadata["ogUrl"]),
		OgImage:       toString(res.Metadata["ogImage"]),
		OgSiteName:    toString(res.Metadata["ogSiteName"]),
		SourceURL:     toString(res.Metadata["sourceURL"]),
		StatusCode:    res.Status,
	}

	doc := &Document{
		Markdown: res.Markdown,
		HTML:     res.HTML,
		RawHTML:  res.RawHTML,
		Links:    res.Links,
		Engine:   res.Engine,
		Metadata: md,
	}

	response := ScrapeResponse{
		Success: true,
		Data:    doc,
	}

	return c.Status(http.StatusOK).JSON(response)
}

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

	timeoutMs := cfg.Scraper.TimeoutMs
	if reqBody.Timeout != nil && *reqBody.Timeout > 0 {
		timeoutMs = *reqBody.Timeout
	}

	ctx, cancel := context.WithTimeout(c.Context(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	res, err := crawler.Map(ctx, crawler.MapOptions{
		URL:               reqBody.URL,
		Limit:             limit,
		Search:            reqBody.Search,
		IncludeSubdomains: includeSubdomains,
		IgnoreQueryParams: ignoreQueryParams,
		AllowExternal:     allowExternal,
		SitemapMode:       sitemapMode,
		Timeout:           time.Duration(timeoutMs) * time.Millisecond,
		RespectRobots:     cfg.Robots.Respect,
		UserAgent:         cfg.Scraper.UserAgent,
	})
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

	links := make([]MapLink, 0, len(res.Links))
	for _, l := range res.Links {
		links = append(links, MapLink{
			URL:         l.URL,
			Title:       l.Title,
			Description: l.Description,
		})
	}

	return c.Status(http.StatusOK).JSON(MapResponse{
		Success: true,
		Links:   links,
		Warning: res.Warning,
	})
}

// extractHandler implements a minimal POST /v1/extract endpoint that:
// - Scrapes a single URL using the HTTP scraper
// - Calls a configured LLM provider to extract structured fields
func extractHandler(c *fiber.Ctx) error {
	var reqBody ExtractRequest
	if err := c.BodyParser(&reqBody); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ExtractResponse{
			Success: false,
			Code:    "BAD_REQUEST_INVALID_JSON",
			Error:   "Bad request, malformed JSON",
		})
	}

	if reqBody.URL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ExtractResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "Missing required field 'url'",
		})
	}
	if len(reqBody.Fields) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ExtractResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "Missing required field 'fields'",
		})
	}

	cfg := c.Locals("config").(*config.Config)

	// Scrape the URL first using the HTTP scraper (no browser for v1 extract).
	timeoutMs := cfg.Scraper.TimeoutMs
	s := scraper.NewHTTPScraper(time.Duration(timeoutMs) * time.Millisecond)

	scrapeCtx, cancel := context.WithTimeout(c.Context(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	scrapeRes, err := s.Scrape(scrapeCtx, scraper.Request{
		URL:       reqBody.URL,
		Headers:   map[string]string{},
		Timeout:   time.Duration(timeoutMs) * time.Millisecond,
		UserAgent: cfg.Scraper.UserAgent,
	})
	if err != nil {
		status := fiber.StatusBadGateway
		if errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		return c.Status(status).JSON(ExtractResponse{
			Success: false,
			Code:    "SCRAPE_FAILED",
			Error:   err.Error(),
		})
	}

	// Build metadata and raw document from the scrape result.
	md := Metadata{
		Title:         toString(scrapeRes.Metadata["title"]),
		Description:   toString(scrapeRes.Metadata["description"]),
		Language:      toString(scrapeRes.Metadata["language"]),
		Keywords:      toString(scrapeRes.Metadata["keywords"]),
		Robots:        toString(scrapeRes.Metadata["robots"]),
		OgTitle:       toString(scrapeRes.Metadata["ogTitle"]),
		OgDescription: toString(scrapeRes.Metadata["ogDescription"]),
		OgURL:         toString(scrapeRes.Metadata["ogUrl"]),
		OgImage:       toString(scrapeRes.Metadata["ogImage"]),
		OgSiteName:    toString(scrapeRes.Metadata["ogSiteName"]),
		SourceURL:     toString(scrapeRes.Metadata["sourceURL"]),
		StatusCode:    scrapeRes.Status,
	}

	rawDoc := &Document{
		Markdown: scrapeRes.Markdown,
		HTML:     scrapeRes.HTML,
		RawHTML:  scrapeRes.RawHTML,
		Links:    scrapeRes.Links,
		Engine:   scrapeRes.Engine,
		Metadata: md,
	}

	// Prepare LLM client and field specs.
	client, provider, model, err := llm.NewClientFromConfig(cfg, reqBody.Provider, reqBody.Model)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(ExtractResponse{
			Success: false,
			Code:    "LLM_NOT_CONFIGURED",
			Error:   err.Error(),
		})
	}

	// Expose LLM info to logging middleware via locals.
	c.Locals("llm_provider", string(provider))
	c.Locals("llm_model", model)

	fieldSpecs := make([]llm.FieldSpec, 0, len(reqBody.Fields))
	for _, f := range reqBody.Fields {
		fieldSpecs = append(fieldSpecs, llm.FieldSpec{
			Name:        f.Name,
			Description: f.Description,
			Type:        f.Type,
		})
	}

	llmTimeout := time.Duration(timeoutMs) * time.Millisecond
	llmCtx, llmCancel := context.WithTimeout(c.Context(), llmTimeout)
	defer llmCancel()

	llmRes, err := client.ExtractFields(llmCtx, llm.ExtractRequest{
		URL:      reqBody.URL,
		Markdown: scrapeRes.Markdown,
		Fields:   fieldSpecs,
		Prompt:   reqBody.Prompt,
		Timeout:  llmTimeout,
		Strict:   reqBody.Strict,
	})
	if err != nil {
		status := fiber.StatusBadGateway
		if errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		metrics.RecordLLMExtract(string(provider), model, false)
		return c.Status(status).JSON(ExtractResponse{
			Success: false,
			Code:    "EXTRACT_FAILED",
			Error:   err.Error(),
		})
	}

	metrics.RecordLLMExtract(string(provider), model, true)

	// Filter fields to only those requested by name.
	requested := make(map[string]struct{}, len(reqBody.Fields))
	for _, f := range reqBody.Fields {
		requested[f.Name] = struct{}{}
	}

	filtered := make(map[string]interface{}, len(requested))
	for k, v := range llmRes.Fields {
		if _, ok := requested[k]; ok {
			filtered[k] = v
		}
	}

	if reqBody.Strict && len(filtered) != len(requested) {
		return c.Status(fiber.StatusBadGateway).JSON(ExtractResponse{
			Success: false,
			Code:    "EXTRACT_INCOMPLETE_FIELDS",
			Error:   "LLM did not return all requested fields",
		})
	}

	result := ExtractResult{
		URL:    scrapeRes.URL,
		Fields: filtered,
		Raw:    rawDoc,
	}

	return c.Status(http.StatusOK).JSON(ExtractResponse{
		Success: true,
		Data:    []ExtractResult{result},
	})
}

func crawlHandler(c *fiber.Ctx) error {
	var reqBody CrawlRequest
	if err := c.BodyParser(&reqBody); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(CrawlResponse{
			Success: false,
			Code:    "BAD_REQUEST_INVALID_JSON",
			Error:   "Bad request, malformed JSON",
		})
	}

	if reqBody.URL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(CrawlResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "Missing required field 'url'",
		})
	}

	_ = c.Locals("config").(*config.Config)
	st := c.Locals("store").(*store.Store)

	// Generate a crawl job ID (uuidv7 preferred)
	id := func() uuid.UUID {
		if id, err := uuid.NewV7(); err == nil {
			return id
		}
		return uuid.New()
	}()

	// Persist job definition. We store the full CrawlRequest so the background
	// worker can reconstruct options when it picks up the job.
	if _, err := st.CreateCrawlJob(c.Context(), id, reqBody.URL, reqBody); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(CrawlResponse{
			Success: false,
			Code:    "CRAWL_JOB_CREATE_FAILED",
			Error:   err.Error(),
		})
	}

	protocol := c.Protocol()
	host := c.Hostname()

	return c.Status(http.StatusOK).JSON(CrawlResponse{
		Success: true,
		ID:      id.String(),
		URL:     protocol + "://" + host + "/v1/crawl/" + id.String(),
	})
}

func crawlStatusHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)

	idParam := c.Params("id")
	jobID, err := uuid.Parse(idParam)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(CrawlResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "invalid crawl id",
		})
	}

	job, docs, err := st.GetCrawlJobAndDocuments(c.Context(), jobID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.Status(http.StatusNotFound).JSON(CrawlResponse{
				Success: false,
				Code:    "NOT_FOUND",
				Error:   "crawl job not found",
			})
		}
		return c.Status(http.StatusInternalServerError).JSON(CrawlResponse{
			Success: false,
			Code:    "CRAWL_JOB_LOOKUP_FAILED",
			Error:   err.Error(),
		})
	}

	resp := CrawlResponse{
		Success: true,
		ID:      job.ID.String(),
		Status:  CrawlStatus(job.Status),
		Total:   len(docs),
	}

	// Map DB documents into API documents only when completed
	if job.Status == "completed" {
		outDocs := make([]Document, 0, len(docs))
		for _, d := range docs {
			var md model.Metadata
			if err := json.Unmarshal(d.Metadata, &md); err != nil {
				continue
			}

			var markdown, html, raw string
			if d.Markdown.Valid {
				markdown = d.Markdown.String
			}
			if d.Html.Valid {
				html = d.Html.String
			}
			if d.RawHtml.Valid {
				raw = d.RawHtml.String
			}

			outDocs = append(outDocs, Document{
				Markdown: markdown,
				HTML:     html,
				RawHTML:  raw,
				Links:    []string{},
				Engine:   "http",
				Metadata: md,
			})
		}
		resp.Data = outDocs
	}

	if job.Error.Valid {
		resp.Error = job.Error.String
	}

	return c.Status(http.StatusOK).JSON(resp)
}

// toString is a small helper to safely convert metadata values to string.
func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
