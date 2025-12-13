package http

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"raito/internal/config"
	"raito/internal/crawler"
	"raito/internal/llm"
	"raito/internal/metrics"
	"raito/internal/model"
	"raito/internal/scraper"
	"raito/internal/search"
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

	// When a job queue-backed executor is available, delegate the heavy
	// scrape work to it so that API-only nodes can remain lightweight and
	// workers perform the browser/LLM work.
	if execVal := c.Locals("executor"); execVal != nil {
		if exec, ok := execVal.(WorkExecutor); ok && exec != nil {
			ctx, cancel := context.WithTimeout(c.Context(), time.Duration(timeoutMs)*time.Millisecond)
			defer cancel()

			res, err := exec.Scrape(ctx, &reqBody)
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
			if res == nil {
				return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
					Success: false,
					Code:    "SCRAPE_FAILED",
					Error:   "empty scrape response",
				})
			}

			status := http.StatusOK
			if !res.Success {
				// Job-level failures are treated as upstream errors.
				status = http.StatusBadGateway
				if res.Code == "SCRAPE_TIMEOUT" || res.Code == "JOB_NOT_STARTED" {
					status = http.StatusGatewayTimeout
				}
			}

			return c.Status(status).JSON(res)
		}
	}

	hasFormats := len(reqBody.Formats) > 0

	// Determine whether screenshot format was requested and its options.
	hasScreenshot, screenshotFullPage := getScreenshotFormatConfig(reqBody.Formats)

	// Choose scraper engine: HTTP by default, rod when requested and enabled.
	useBrowser := false
	if reqBody.UseBrowser != nil {
		useBrowser = *reqBody.UseBrowser
	}
	if hasScreenshot {
		// Screenshot always uses the browser engine.
		useBrowser = true
	}

	var engine scraper.Scraper
	if useBrowser {
		if !cfg.Rod.Enabled {
			if hasScreenshot {
				return c.Status(http.StatusInternalServerError).JSON(ErrorResponse{
					Success: false,
					Code:    "SCREENSHOT_NOT_AVAILABLE",
					Error:   "screenshot format requires browser scraping, but rod is disabled in server configuration",
				})
			}
			engine = scraper.NewHTTPScraper(time.Duration(timeoutMs) * time.Millisecond)
		} else {
			// When rod is enabled, always use a locally managed headless browser
			// via RodScraper. The browser pool / BrowserURL support has been
			// removed for now to simplify deployment.
			engine = scraper.NewRodScraper(time.Duration(timeoutMs) * time.Millisecond)
		}
	} else {
		engine = scraper.NewHTTPScraper(time.Duration(timeoutMs) * time.Millisecond)
	}

	headers := map[string]string{}
	for k, v := range reqBody.Headers {
		headers[k] = v
	}
	// Apply location settings to Accept-Language when provided.
	if reqBody.Location != nil {
		if len(reqBody.Location.Languages) > 0 {
			headers["Accept-Language"] = strings.Join(reqBody.Location.Languages, ",")
		} else if reqBody.Location.Country != "" {
			headers["Accept-Language"] = reqBody.Location.Country
		}
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

	links := res.Links
	if len(links) > 0 {
		links = filterLinks(links, res.URL, cfg.Scraper.LinksSameDomainOnly, cfg.Scraper.LinksMaxPerDocument)
	}

	// Map link metadata to the API model, keeping it aligned with the filtered links.
	linkSet := make(map[string]struct{}, len(links))
	for _, l := range links {
		linkSet[l] = struct{}{}
	}
	linkMetadata := make([]LinkMetadata, 0, len(links))
	for _, lm := range res.LinkMetadata {
		if _, ok := linkSet[lm.URL]; !ok {
			continue
		}
		linkMetadata = append(linkMetadata, LinkMetadata{
			URL:  lm.URL,
			Text: lm.Text,
			Rel:  lm.Rel,
		})
	}

	images := scraper.ExtractImages(res.HTML, res.URL)

	// When formats are provided, only include the requested fields
	// (plus minimal metadata/engine) to better match Firecrawl semantics.

	includeMarkdown := !hasFormats || wantsFormat(reqBody.Formats, "markdown")
	includeHTML := !hasFormats || wantsFormat(reqBody.Formats, "html")
	includeRawHTML := !hasFormats || wantsFormat(reqBody.Formats, "rawHtml")
	includeLinks := !hasFormats || wantsFormat(reqBody.Formats, "links")
	includeImages := !hasFormats || wantsFormat(reqBody.Formats, "images")

	doc := &Document{
		Engine:   res.Engine,
		Metadata: md,
	}

	if includeMarkdown {
		doc.Markdown = res.Markdown
	}
	if includeHTML {
		doc.HTML = res.HTML
	}
	if includeRawHTML {
		doc.RawHTML = res.RawHTML
	}
	if includeLinks {
		doc.Links = links
		doc.LinkMetadata = linkMetadata
	}
	if includeImages {
		doc.Images = images
	}

	// Optional screenshot format using the browser engine when requested.
	if hasScreenshot {
		screenshotCtx, screenshotCancel := context.WithTimeout(c.Context(), time.Duration(timeoutMs)*time.Millisecond)
		defer screenshotCancel()

		shot, err := scraper.CaptureScreenshot(screenshotCtx, res.URL, time.Duration(timeoutMs)*time.Millisecond, screenshotFullPage)
		if err != nil {
			status := fiber.StatusBadGateway
			if errors.Is(err, context.DeadlineExceeded) {
				status = http.StatusGatewayTimeout
			}
			return c.Status(status).JSON(ErrorResponse{
				Success: false,
				Code:    "SCREENSHOT_FAILED",
				Error:   err.Error(),
			})
		}

		doc.Screenshot = base64.StdEncoding.EncodeToString(shot)
	}

	if includeMarkdown {
		doc.Markdown = res.Markdown
	}
	if includeHTML {
		doc.HTML = res.HTML
	}
	if includeRawHTML {
		doc.RawHTML = res.RawHTML
	}
	if includeLinks {
		doc.Links = links
		doc.LinkMetadata = linkMetadata
	}
	if includeImages {
		doc.Images = images
	}

	// Optional summary format using the configured LLM provider when requested.
	if wantsFormat(reqBody.Formats, "summary") {
		client, provider, modelName, err := llm.NewClientFromConfig(cfg, "", "")
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(ErrorResponse{
				Success: false,
				Code:    "LLM_NOT_CONFIGURED",
				Error:   err.Error(),
			})
		}

		// Expose LLM info to logging middleware via locals.
		c.Locals("llm_provider", string(provider))
		c.Locals("llm_model", modelName)

		fieldSpecs := []llm.FieldSpec{
			{
				Name:        "summary",
				Description: "Short natural-language summary of the page content.",
				Type:        "string",
			},
		}

		llmTimeout := time.Duration(timeoutMs) * time.Millisecond
		llmCtx, llmCancel := context.WithTimeout(c.Context(), llmTimeout)
		defer llmCancel()

		llmRes, err := client.ExtractFields(llmCtx, llm.ExtractRequest{
			URL:      reqBody.URL,
			Markdown: res.Markdown,
			Fields:   fieldSpecs,
			Prompt:   "",
			Timeout:  llmTimeout,
			Strict:   false,
		})
		if err != nil {
			metrics.RecordLLMExtract(string(provider), modelName, false)
			status := fiber.StatusBadGateway
			if errors.Is(err, context.DeadlineExceeded) {
				status = http.StatusGatewayTimeout
			}
			return c.Status(status).JSON(ErrorResponse{
				Success: false,
				Code:    "SUMMARY_FAILED",
				Error:   err.Error(),
			})
		}

		metrics.RecordLLMExtract(string(provider), modelName, true)

		if v, ok := llmRes.Fields["summary"]; ok {
			if s, ok := v.(string); ok {
				doc.Summary = s
			} else {
				doc.Summary = fmt.Sprint(v)
			}
		}
	}

	// Optional json format using the configured LLM provider when requested.
	if hasJSON, jsonPrompt, jsonSchema := getJSONFormatConfig(reqBody.Formats); hasJSON {
		client, provider, modelName, err := llm.NewClientFromConfig(cfg, "", "")
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(ErrorResponse{
				Success: false,
				Code:    "LLM_NOT_CONFIGURED",
				Error:   err.Error(),
			})
		}

		// Expose LLM info to logging middleware via locals (may override previous LLM info).
		c.Locals("llm_provider", string(provider))
		c.Locals("llm_model", modelName)

		desc := "Arbitrary JSON object extracted from the page content."
		if len(jsonSchema) > 0 {
			if schemaBytes, err := json.Marshal(jsonSchema); err == nil {
				desc = desc + " Schema: " + string(schemaBytes)
			}
		}

		fieldSpecs := []llm.FieldSpec{
			{
				Name:        "json",
				Description: desc,
				Type:        "object",
			},
		}

		llmTimeout := time.Duration(timeoutMs) * time.Millisecond
		llmCtx, llmCancel := context.WithTimeout(c.Context(), llmTimeout)
		defer llmCancel()

		llmRes, err := client.ExtractFields(llmCtx, llm.ExtractRequest{
			URL:      reqBody.URL,
			Markdown: res.Markdown,
			Fields:   fieldSpecs,
			Prompt:   jsonPrompt,
			Timeout:  llmTimeout,
			Strict:   false,
		})
		if err != nil {
			metrics.RecordLLMExtract(string(provider), modelName, false)
			status := fiber.StatusBadGateway
			if errors.Is(err, context.DeadlineExceeded) {
				status = http.StatusGatewayTimeout
			}
			return c.Status(status).JSON(ErrorResponse{
				Success: false,
				Code:    "JSON_EXTRACT_FAILED",
				Error:   err.Error(),
			})
		}

		metrics.RecordLLMExtract(string(provider), modelName, true)

		if v, ok := llmRes.Fields["json"]; ok {
			// v is expected to be a nested map[string]interface{} representing structured JSON.
			if m, ok := v.(map[string]interface{}); ok {
				doc.JSON = m
			} else {
				// If the LLM returns a non-object, still expose it as best-effort.
				// The client can decide how to interpret this.
				// We wrap it into a single-field object for consistency.
				doc.JSON = map[string]interface{}{"_value": v}
			}
		}
	}

	// Optional branding format using the configured LLM provider when requested.
	if hasBranding, brandingPrompt := getBrandingFormatConfig(reqBody.Formats); hasBranding {
		client, provider, modelName, err := llm.NewClientFromConfig(cfg, "", "")
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(ErrorResponse{
				Success: false,
				Code:    "LLM_NOT_CONFIGURED",
				Error:   err.Error(),
			})
		}

		// Expose LLM info to logging middleware via locals (may override previous LLM info).
		c.Locals("llm_provider", string(provider))
		c.Locals("llm_model", modelName)

		// Default branding prompt modeled after Firecrawl's BrandingProfile,
		// asking for a structured object with keys like colorScheme, colors,
		// typography, spacing, components, images, fonts, tone, and personality.
		if brandingPrompt == "" {
			brandingPrompt = "You are a brand design expert analyzing a website. Analyze the page and return a single JSON object describing the brand, matching this structure as closely as possible: " +
				"{colorScheme?: 'light'|'dark', colors?: {primary?: string, secondary?: string, accent?: string, background?: string, textPrimary?: string, textSecondary?: string, link?: string, success?: string, warning?: string, error?: string}, " +
				"typography?: {fontFamilies?: {primary?: string, heading?: string, code?: string}, fontStacks?: {primary?: string[], heading?: string[], body?: string[], paragraph?: string[]}, fontSizes?: {h1?: string, h2?: string, h3?: string, body?: string, small?: string}}, " +
				"spacing?: {baseUnit?: number, borderRadius?: string}, components?: {buttonPrimary?: {background?: string, textColor?: string, borderColor?: string, borderRadius?: string}, buttonSecondary?: {...}}, " +
				"images?: {logo?: string|null, favicon?: string|null, ogImage?: string|null}, personality?: {tone?: string, energy?: string, targetAudience?: string}}. " +
				"Only include fields you can infer with reasonable confidence."
		}

		descBranding := "Brand identity and design system information (colors, typography, logo, components, personality, etc.) extracted from the page, following Firecrawl's BrandingProfile conventions."

		fieldSpecs := []llm.FieldSpec{
			{
				Name:        "branding",
				Description: descBranding,
				Type:        "object",
			},
		}

		llmTimeout := time.Duration(timeoutMs) * time.Millisecond
		llmCtx, llmCancel := context.WithTimeout(c.Context(), llmTimeout)
		defer llmCancel()

		llmRes, err := client.ExtractFields(llmCtx, llm.ExtractRequest{
			URL:      reqBody.URL,
			Markdown: res.Markdown,
			Fields:   fieldSpecs,
			Prompt:   brandingPrompt,
			Timeout:  llmTimeout,
			Strict:   false,
		})

		if err != nil {
			metrics.RecordLLMExtract(string(provider), modelName, false)
			status := fiber.StatusBadGateway
			if errors.Is(err, context.DeadlineExceeded) {
				status = http.StatusGatewayTimeout
			}
			return c.Status(status).JSON(ErrorResponse{
				Success: false,
				Code:    "BRANDING_FAILED",
				Error:   err.Error(),
			})
		}

		metrics.RecordLLMExtract(string(provider), modelName, true)

		if v, ok := llmRes.Fields["branding"]; ok {
			if m, ok := v.(map[string]interface{}); ok {
				normalizeBrandingImages(m)
				doc.Branding = m
			} else {
				doc.Branding = map[string]interface{}{"_value": v}
			}
		}
	}

	response := ScrapeResponse{

		Success: true,
		Data:    doc,
	}

	return c.Status(http.StatusOK).JSON(response)
}

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

	hasScrape := reqBody.ScrapeOptions != nil

	web := make([]SearchWebResult, 0, len(results.Web))
	warning := ""
	invalidURLCount := 0
	scrapeErrorCount := 0
	scrapedCount := 0

	if !hasScrape {
		for _, r := range results.Web {
			web = append(web, SearchWebResult{
				Title:       r.Title,
				Description: r.Description,
				URL:         r.URL,
			})
		}
	} else {
		for _, r := range results.Web {
			entry := SearchWebResult{
				Title:       r.Title,
				Description: r.Description,
				URL:         r.URL,
			}

			if strings.TrimSpace(r.URL) == "" {
				invalidURLCount++
				if ignoreInvalid {
					// Drop this result entirely when ignoreInvalidURLs is requested.
					continue
				}
				web = append(web, entry)
				continue
			}

			doc, err := scrapeURLForSearch(ctx, cfg, r.URL, reqBody.ScrapeOptions, timeoutMs)
			if err != nil {
				scrapeErrorCount++
				if ignoreInvalid {
					// Treat failed scrapes as invalid when ignoreInvalidURLs
					// is requested and drop the result.
					continue
				}
				// Keep the plain search result but omit the document.
				web = append(web, entry)
				continue
			}

			if doc != nil {
				entry.Document = doc
				entry.Metadata = doc.Metadata
				entry.Engine = doc.Engine
				scrapedCount++
			}

			web = append(web, entry)
		}
	}

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

	// Require at least one URL (either url or urls)
	urls := reqBody.URLs
	if len(urls) == 0 && reqBody.URL != "" {
		urls = []string{reqBody.URL}
	}
	if len(urls) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ExtractResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "Missing required field 'urls' or 'url'",
		})
	}

	if len(reqBody.Fields) == 0 && len(reqBody.Schema) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ExtractResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "Missing required field 'fields' or 'schema'",
		})
	}

	st := c.Locals("store").(*store.Store)

	// Generate an extract job ID (uuidv7 preferred)
	id := func() uuid.UUID {
		if id, err := uuid.NewV7(); err == nil {
			return id
		}
		return uuid.New()
	}()

	primaryURL := urls[0]

	// Persist job definition. We store the full ExtractRequest so the background
	// worker can reconstruct options when it picks up the job.
	if _, err := st.CreateJob(c.Context(), id, "extract", primaryURL, reqBody, false, 10); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(ExtractResponse{
			Success: false,
			Code:    "EXTRACT_JOB_CREATE_FAILED",
			Error:   err.Error(),
		})
	}

	protocol := c.Protocol()
	host := c.Hostname()

	return c.Status(http.StatusOK).JSON(fiber.Map{
		"success": true,
		"id":      id.String(),
		"url":     protocol + "://" + host + "/v1/extract/" + id.String(),
	})
}

func extractStatusHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)

	idParam := c.Params("id")
	jobID, err := uuid.Parse(idParam)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ExtractStatusResponse{
			Success: false,
			Status:  ExtractStatusFailed,
			Code:    "BAD_REQUEST",
			Error:   "invalid extract id",
		})
	}

	job, err := st.GetJobByID(c.Context(), jobID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.Status(fiber.StatusNotFound).JSON(ExtractStatusResponse{
				Success: false,
				Status:  ExtractStatusFailed,
				Code:    "NOT_FOUND",
				Error:   "extract job not found",
			})
		}
		return c.Status(http.StatusInternalServerError).JSON(ExtractStatusResponse{
			Success: false,
			Status:  ExtractStatusFailed,
			Code:    "EXTRACT_JOB_LOOKUP_FAILED",
			Error:   err.Error(),
		})
	}

	resp := ExtractStatusResponse{
		Success: true,
		Status:  ExtractJobStatus(job.Status),
	}

	switch job.Status {
	case "completed":
		if job.Output.Valid && len(job.Output.RawMessage) > 0 {
			var data map[string]interface{}
			if err := json.Unmarshal(job.Output.RawMessage, &data); err != nil {
				return c.Status(http.StatusInternalServerError).JSON(ExtractStatusResponse{
					Success: false,
					Status:  ExtractStatusFailed,
					Code:    "EXTRACT_RESULT_DECODE_FAILED",
					Error:   err.Error(),
				})
			}
			resp.Data = data
		}
	case "failed":
		code := "EXTRACT_FAILED"
		msg := "extract job failed"
		if job.Error.Valid {
			msg = job.Error.String
			if idx := strings.Index(msg, ":"); idx != -1 {
				maybeCode := strings.TrimSpace(msg[:idx])
				if maybeCode != "" {
					code = maybeCode
				}
				msg = strings.TrimSpace(msg[idx+1:])
			}
		}
		resp.Code = code
		resp.Error = msg
	}

	return c.Status(http.StatusOK).JSON(resp)
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

func batchScrapeHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)

	var reqBody BatchScrapeRequest
	if err := c.BodyParser(&reqBody); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(BatchScrapeResponse{
			Success: false,
			Code:    "BAD_REQUEST_INVALID_JSON",
			Error:   "Bad request, malformed JSON",
		})
	}

	if len(reqBody.URLs) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(BatchScrapeResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "Missing required field 'urls'",
		})
	}

	// Basic sanity limit to avoid huge batches in v1.
	if len(reqBody.URLs) > 1000 {
		return c.Status(fiber.StatusBadRequest).JSON(BatchScrapeResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "Too many urls; maximum is 1000",
		})
	}

	// Generate a batch scrape job ID (uuidv7 preferred)
	id := func() uuid.UUID {
		if id, err := uuid.NewV7(); err == nil {
			return id
		}
		return uuid.New()
	}()

	primaryURL := reqBody.URLs[0]

	// Persist job definition. We store the full BatchScrapeRequest so the background
	// worker can reconstruct options when it picks up the job.
	if _, err := st.CreateJob(c.Context(), id, "batch_scrape", primaryURL, reqBody, false, 10); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(BatchScrapeResponse{
			Success: false,
			Code:    "BATCH_SCRAPE_JOB_CREATE_FAILED",
			Error:   err.Error(),
		})
	}

	protocol := c.Protocol()
	host := c.Hostname()

	return c.Status(http.StatusOK).JSON(BatchScrapeResponse{
		Success: true,
		ID:      id.String(),
		URL:     protocol + "://" + host + "/v1/batch/scrape/" + id.String(),
	})
}

func batchScrapeStatusHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)

	idParam := c.Params("id")
	jobID, err := uuid.Parse(idParam)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(BatchScrapeResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "invalid batch scrape id",
		})
	}

	job, docs, err := st.GetCrawlJobAndDocuments(c.Context(), jobID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.Status(fiber.StatusNotFound).JSON(BatchScrapeResponse{
				Success: false,
				Code:    "NOT_FOUND",
				Error:   "batch scrape job not found",
			})
		}
		return c.Status(http.StatusInternalServerError).JSON(BatchScrapeResponse{
			Success: false,
			Code:    "BATCH_SCRAPE_JOB_LOOKUP_FAILED",
			Error:   err.Error(),
		})
	}

	resp := BatchScrapeResponse{
		Success: true,
		ID:      job.ID.String(),
		Status:  BatchScrapeStatus(job.Status),
		Total:   len(docs),
	}

	// Map DB documents into API documents only when completed
	if job.Status == "completed" {
		// Decode the original batch request to determine requested formats.
		var originalReq BatchScrapeRequest
		_ = json.Unmarshal(job.Input, &originalReq)
		formats := originalReq.Formats
		hasFormats := len(formats) > 0

		includeMarkdown := !hasFormats || wantsFormat(formats, "markdown")
		includeHTML := !hasFormats || wantsFormat(formats, "html")
		includeRawHTML := !hasFormats || wantsFormat(formats, "rawHtml")
		includeImages := !hasFormats || wantsFormat(formats, "images")

		outDocs := make([]Document, 0, len(docs))
		for _, d := range docs {
			var md model.Metadata
			if err := json.Unmarshal(d.Metadata, &md); err != nil {
				continue
			}

			var markdown, html, raw, engine string
			if d.Markdown.Valid {
				markdown = d.Markdown.String
			}
			if d.Html.Valid {
				html = d.Html.String
			}
			if d.RawHtml.Valid {
				raw = d.RawHtml.String
			}
			if d.Engine.Valid {
				engine = d.Engine.String
			}
			if engine == "" {
				engine = "http"
			}

			images := scraper.ExtractImages(html, md.SourceURL)
			if len(images) == 0 && raw != "" {
				images = scraper.ExtractImages(raw, md.SourceURL)
			}

			doc := Document{
				Engine:   engine,
				Metadata: md,
			}

			if includeMarkdown {
				doc.Markdown = markdown
			}
			if includeHTML {
				doc.HTML = html
			}
			if includeRawHTML {
				doc.RawHTML = raw
			}
			if includeImages {
				doc.Images = images
			}

			outDocs = append(outDocs, doc)
		}
		resp.Data = outDocs
	}

	if job.Error.Valid {
		resp.Error = job.Error.String
	}

	return c.Status(http.StatusOK).JSON(resp)
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
			return c.Status(fiber.StatusNotFound).JSON(CrawlResponse{
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
		// Decode the original crawl request to determine requested formats.
		var originalReq CrawlRequest
		_ = json.Unmarshal(job.Input, &originalReq)
		formats := originalReq.Formats
		hasFormats := len(formats) > 0

		includeMarkdown := !hasFormats || wantsFormat(formats, "markdown")
		includeHTML := !hasFormats || wantsFormat(formats, "html")
		includeRawHTML := !hasFormats || wantsFormat(formats, "rawHtml")
		includeImages := !hasFormats || wantsFormat(formats, "images")
		includeSummary := wantsFormat(formats, "summary")
		includeJSON := wantsFormat(formats, "json")

		outDocs := make([]Document, 0, len(docs))
		for _, d := range docs {
			var md model.Metadata
			if err := json.Unmarshal(d.Metadata, &md); err != nil {
				continue
			}

			var markdown, html, raw, engine string
			if d.Markdown.Valid {
				markdown = d.Markdown.String
			}
			if d.Html.Valid {
				html = d.Html.String
			}
			if d.RawHtml.Valid {
				raw = d.RawHtml.String
			}
			if d.Engine.Valid {
				engine = d.Engine.String
			}
			if engine == "" {
				engine = "http"
			}

			images := scraper.ExtractImages(html, md.SourceURL)
			if len(images) == 0 && raw != "" {
				images = scraper.ExtractImages(raw, md.SourceURL)
			}

			doc := Document{
				Engine:   engine,
				Metadata: md,
			}

			if includeMarkdown {
				doc.Markdown = markdown
			}
			if includeHTML {
				doc.HTML = html
			}
			if includeRawHTML {
				doc.RawHTML = raw
			}
			if includeImages {
				doc.Images = images
			}
			if includeSummary && md.Summary != "" {
				doc.Summary = md.Summary
			}
			if includeJSON && md.JSON != nil {
				doc.JSON = md.JSON
			}

			outDocs = append(outDocs, doc)
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

// filterLinks applies basic link filters based on configuration.
// sameDomainOnly restricts links to those matching the base URL's host.
// maxPerDocument > 0 limits the number of links returned.
func filterLinks(links []string, baseURL string, sameDomainOnly bool, maxPerDocument int) []string {
	if len(links) == 0 {
		return links
	}

	filtered := make([]string, 0, len(links))

	var baseHost string
	if sameDomainOnly {
		if u, err := url.Parse(baseURL); err == nil {
			baseHost = strings.ToLower(u.Hostname())
		} else {
			// If base URL is invalid, skip same-domain filtering but still apply maxPerDocument.
			sameDomainOnly = false
		}
	}

	for _, link := range links {
		if link == "" {
			continue
		}

		if sameDomainOnly {
			lu, err := url.Parse(link)
			if err != nil {
				continue
			}
			if strings.ToLower(lu.Hostname()) != baseHost {
				continue
			}
		}

		filtered = append(filtered, link)
		if maxPerDocument > 0 && len(filtered) >= maxPerDocument {
			break
		}
	}

	return filtered
}

// wantsFormat inspects a Firecrawl-style formats array to determine
// whether a given format type (e.g., "summary") was requested.
func wantsFormat(formats []interface{}, name string) bool {
	if len(formats) == 0 {
		return false
	}

	target := strings.ToLower(name)
	for _, f := range formats {
		switch v := f.(type) {
		case string:
			if strings.ToLower(v) == target {
				return true
			}
		case map[string]interface{}:
			if t, ok := v["type"]; ok {
				if s, ok := t.(string); ok && strings.ToLower(s) == target {
					return true
				}
			}
		}
	}

	return false
}

// getJSONFormatConfig scans a Firecrawl-style formats array and returns
// the first json format configuration, if present. It supports both
// simple string formats ("json") and object formats ({type: "json", ...}).
func getJSONFormatConfig(formats []interface{}) (bool, string, map[string]interface{}) {
	if len(formats) == 0 {
		return false, "", nil
	}

	for _, f := range formats {
		switch v := f.(type) {
		case string:
			if strings.ToLower(v) == "json" {
				return true, "", nil
			}
		case map[string]interface{}:
			rawType, ok := v["type"].(string)
			if !ok || strings.ToLower(rawType) != "json" {
				continue
			}

			prompt := ""
			if p, ok := v["prompt"].(string); ok {
				prompt = p
			}

			var schema map[string]interface{}
			if s, ok := v["schema"].(map[string]interface{}); ok {
				schema = s
			}

			return true, prompt, schema
		}
	}

	return false, "", nil
}

// getBrandingFormatConfig scans formats for a branding entry and returns
// whether it was requested along with an optional custom prompt.
func getBrandingFormatConfig(formats []interface{}) (bool, string) {
	if len(formats) == 0 {
		return false, ""
	}

	for _, f := range formats {
		switch v := f.(type) {
		case string:
			if strings.ToLower(v) == "branding" {
				return true, ""
			}
		case map[string]interface{}:
			rawType, ok := v["type"].(string)
			if !ok || strings.ToLower(rawType) != "branding" {
				continue
			}

			prompt := ""
			if p, ok := v["prompt"].(string); ok {
				prompt = p
			}

			return true, prompt
		}
	}

	return false, ""
}

// normalizeBrandingImages prunes nil values from the images sub-object
// of a branding profile so that fields like favicon and ogImage are
// omitted rather than returned as explicit nulls.
func normalizeBrandingImages(branding map[string]interface{}) {
	if branding == nil {
		return
	}

	imagesVal, ok := branding["images"]
	if !ok {
		return
	}

	imagesMap, ok := imagesVal.(map[string]interface{})
	if !ok {
		return
	}

	for k, v := range imagesMap {
		if v == nil {
			delete(imagesMap, k)
		}
	}

	if len(imagesMap) == 0 {
		delete(branding, "images")
	} else {
		branding["images"] = imagesMap
	}
}

// getScreenshotFormatConfig scans formats for a screenshot entry and returns
// whether it was requested along with a fullPage flag. It supports both simple
// string formats ("screenshot") and object formats ({type: "screenshot", ...}).
func scrapeURLForSearch(ctx context.Context, cfg *config.Config, url string, opts *ScrapeOptions, timeoutMs int) (*Document, error) {
	if timeoutMs <= 0 {
		timeoutMs = cfg.Scraper.TimeoutMs
	}
	if timeoutMs <= 0 {
		timeoutMs = 30000
	}

	var engine scraper.Scraper
	useBrowser := false
	if opts != nil && opts.UseBrowser != nil {
		useBrowser = *opts.UseBrowser
	}
	if useBrowser && cfg.Rod.Enabled {
		engine = scraper.NewRodScraper(time.Duration(timeoutMs) * time.Millisecond)
	} else {
		engine = scraper.NewHTTPScraper(time.Duration(timeoutMs) * time.Millisecond)
	}

	headers := map[string]string{}
	if opts != nil {
		for k, v := range opts.Headers {
			headers[k] = v
		}
		if opts.Location != nil {
			if len(opts.Location.Languages) > 0 {
				headers["Accept-Language"] = strings.Join(opts.Location.Languages, ",")
			} else if opts.Location.Country != "" {
				headers["Accept-Language"] = opts.Location.Country
			}
		}
	}

	sReq := scraper.Request{
		URL:       url,
		Headers:   headers,
		Timeout:   time.Duration(timeoutMs) * time.Millisecond,
		UserAgent: cfg.Scraper.UserAgent,
	}

	res, err := engine.Scrape(ctx, sReq)
	if err != nil {
		return nil, err
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

	links := res.Links
	if len(links) > 0 {
		links = filterLinks(links, res.URL, cfg.Scraper.LinksSameDomainOnly, cfg.Scraper.LinksMaxPerDocument)
	}

	linkSet := make(map[string]struct{}, len(links))
	for _, l := range links {
		linkSet[l] = struct{}{}
	}

	linkMetadata := make([]LinkMetadata, 0, len(links))
	for _, lm := range res.LinkMetadata {
		if _, ok := linkSet[lm.URL]; !ok {
			continue
		}
		linkMetadata = append(linkMetadata, LinkMetadata{
			URL:  lm.URL,
			Text: lm.Text,
			Rel:  lm.Rel,
		})
	}

	images := scraper.ExtractImages(res.HTML, res.URL)

	formats := []interface{}{}
	if opts != nil {
		formats = opts.Formats
	}
	hasFormats := len(formats) > 0

	// For /v1/search, keep documents lightweight by default:
	// - When no formats are provided, include only markdown + metadata.
	// - When formats are provided, honor them explicitly.
	includeMarkdown := true
	includeHTML := false
	includeRawHTML := false
	includeLinks := false
	includeImages := false

	if hasFormats {
		includeMarkdown = wantsFormat(formats, "markdown")
		includeHTML = wantsFormat(formats, "html")
		includeRawHTML = wantsFormat(formats, "rawHtml")
		includeLinks = wantsFormat(formats, "links")
		includeImages = wantsFormat(formats, "images")
	}

	doc := &Document{
		Engine:   res.Engine,
		Metadata: md,
	}

	if includeMarkdown {
		doc.Markdown = res.Markdown
	}
	if includeHTML {
		doc.HTML = res.HTML
	}
	if includeRawHTML {
		doc.RawHTML = res.RawHTML
	}
	if includeLinks {
		doc.Links = links
		doc.LinkMetadata = linkMetadata
	}
	if includeImages {
		doc.Images = images
	}

	return doc, nil
}

func getScreenshotFormatConfig(formats []interface{}) (bool, bool) {
	if len(formats) == 0 {
		return false, false
	}

	for _, f := range formats {
		switch v := f.(type) {
		case string:
			if strings.ToLower(v) == "screenshot" {
				// Default to full-page screenshots for better parity with Firecrawl.
				return true, true
			}
		case map[string]interface{}:
			rawType, ok := v["type"].(string)
			if !ok || strings.ToLower(rawType) != "screenshot" {
				continue
			}

			fullPage := true
			if fp, ok := v["fullPage"].(bool); ok {
				fullPage = fp
			}

			return true, fullPage
		}
	}

	return false, false
}
