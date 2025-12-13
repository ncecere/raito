package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"raito/internal/config"
	"raito/internal/crawler"
	"raito/internal/llm"
	"raito/internal/metrics"
	"raito/internal/model"
	"raito/internal/scraper"
	"raito/internal/store"
)

// RetentionStats captures the number of records deleted by TTL cleanup.
type RetentionStats struct {
	DocumentsDeleted int64            `json:"documentsDeleted"`
	JobsDeleted      map[string]int64 `json:"jobsDeleted"`
}

// cleanupExpiredData deletes old jobs and documents based on retention
// settings so that the database does not grow without bound.
func cleanupExpiredData(ctx context.Context, cfg *config.Config, st *store.Store) RetentionStats {
	now := time.Now().UTC()
	stats := RetentionStats{JobsDeleted: make(map[string]int64)}

	// Documents TTL (currently crawl documents only)
	if cfg.Retention.Documents.DefaultDays > 0 {
		cutoff := now.AddDate(0, 0, -cfg.Retention.Documents.DefaultDays)
		if n, err := st.DeleteExpiredDocuments(ctx, cutoff); err == nil && n > 0 {
			stats.DocumentsDeleted += n
			metrics.RecordRetentionDocuments(n)
		}
	}

	// Jobs TTL per job type, falling back to defaultDays when specific
	// values are not provided.
	jobTTL := cfg.Retention.Jobs

	applyJobTTL := func(jobType string, days int) {
		if days <= 0 {
			return
		}
		cutoff := now.AddDate(0, 0, -days)
		if n, err := st.DeleteExpiredJobsByType(ctx, jobType, cutoff); err == nil && n > 0 {
			stats.JobsDeleted[jobType] += n
			metrics.RecordRetentionJobs(jobType, n)
		}
	}

	// Helper to compute effective TTL for each known job type.
	effectiveDays := func(specific int) int {
		if specific > 0 {
			return specific
		}
		return jobTTL.DefaultDays
	}

	applyJobTTL("scrape", effectiveDays(jobTTL.ScrapeDays))
	applyJobTTL("map", effectiveDays(jobTTL.MapDays))
	applyJobTTL("extract", effectiveDays(jobTTL.ExtractDays))
	applyJobTTL("crawl", effectiveDays(jobTTL.CrawlDays))
	applyJobTTL("batch_scrape", effectiveDays(0))

	return stats
}

// StartCrawlWorker launches a background worker that periodically polls the
// database for pending crawl jobs and processes them.
func StartCrawlWorker(ctx context.Context, cfg *config.Config, st *store.Store) {
	go func() {
		pollInterval := time.Duration(cfg.Worker.PollIntervalMs) * time.Millisecond
		if pollInterval <= 0 {
			pollInterval = 2 * time.Second
		}

		maxJobs := cfg.Worker.MaxConcurrentJobs
		if maxJobs <= 0 {
			maxJobs = 4
		}

		sem := make(chan struct{}, maxJobs)
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		var lastCleanup time.Time
		cleanupInterval := time.Duration(cfg.Retention.CleanupIntervalMinutes) * time.Minute
		if cleanupInterval <= 0 {
			cleanupInterval = time.Hour
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

			// Periodically run TTL cleanup for jobs/documents.
			if cfg.Retention.Enabled {
				now := time.Now().UTC()
				if lastCleanup.IsZero() || now.Sub(lastCleanup) >= cleanupInterval {
					_ = cleanupExpiredData(ctx, cfg, st)
					lastCleanup = now
				}
			}

			// Determine how many new jobs we can start based on current concurrency.
			capacity := maxJobs - len(sem)
			if capacity <= 0 {
				continue
			}

			jobs, err := st.ListPendingJobs(ctx, int32(capacity))
			if err != nil {
				// TODO: add logging once logging is in place
				continue
			}

			for _, job := range jobs {
				job := job
				sem <- struct{}{}
				go func() {
					defer func() { <-sem }()

					switch job.Type {
					case "crawl":
						// Decode the original crawl request from the job input.
						var req CrawlRequest
						if err := json.Unmarshal(job.Input, &req); err != nil {
							msg := "invalid crawl job input: " + err.Error()
							_ = st.UpdateCrawlJobStatus(context.Background(), job.ID, "failed", &msg)
							return
						}

						// Fallback: ensure URL is set even if the stored input was minimal.
						if req.URL == "" {
							req.URL = job.Url
						}

						// Mark job running before we start work.
						_ = st.UpdateCrawlJobStatus(context.Background(), job.ID, "running", nil)

						// Let the job inherit the worker context; per-request
						// timeouts are applied inside runCrawlJob for HTTP and LLM.
						runCrawlJob(ctx, cfg, st, job.ID, req)
					case "scrape":
						// Decode the original scrape request from the job input.
						var req ScrapeRequest
						if err := json.Unmarshal(job.Input, &req); err != nil {
							msg := "SCRAPE_FAILED: invalid scrape job input: " + err.Error()
							_ = st.UpdateCrawlJobStatus(context.Background(), job.ID, "failed", &msg)
							return
						}

						// Fallback: ensure URL is set even if the stored input was minimal.
						if req.URL == "" {
							req.URL = job.Url
						}

						// Mark job running before we start work.
						_ = st.UpdateCrawlJobStatus(context.Background(), job.ID, "running", nil)

						// Execute the scrape using the worker context and
						// persist the resulting document into job.output.
						runScrapeJob(ctx, cfg, st, job.ID, req)
					case "map":
						// Decode the original map request from the job input.
						var req MapRequest
						if err := json.Unmarshal(job.Input, &req); err != nil {
							msg := "MAP_FAILED: invalid map job input: " + err.Error()
							_ = st.UpdateCrawlJobStatus(context.Background(), job.ID, "failed", &msg)
							return
						}

						if req.URL == "" {
							req.URL = job.Url
						}

						_ = st.UpdateCrawlJobStatus(context.Background(), job.ID, "running", nil)

						runMapJob(ctx, cfg, st, job.ID, req)
					case "extract":
						// Decode the original extract request from the job input.
						var req ExtractRequest
						if err := json.Unmarshal(job.Input, &req); err != nil {
							msg := "EXTRACT_FAILED: invalid extract job input: " + err.Error()
							_ = st.UpdateCrawlJobStatus(context.Background(), job.ID, "failed", &msg)
							return
						}

						if req.URL == "" {
							req.URL = job.Url
						}

						_ = st.UpdateCrawlJobStatus(context.Background(), job.ID, "running", nil)

						runExtractJob(ctx, cfg, st, job.ID, req)
					case "batch_scrape":
						var req BatchScrapeRequest
						if err := json.Unmarshal(job.Input, &req); err != nil {
							msg := "BATCH_SCRAPE_FAILED: invalid batch scrape job input: " + err.Error()
							_ = st.UpdateCrawlJobStatus(context.Background(), job.ID, "failed", &msg)
							return
						}

						_ = st.UpdateCrawlJobStatus(context.Background(), job.ID, "running", nil)

						runBatchScrapeJob(ctx, cfg, st, job.ID, req)
					default:
						msg := "UNKNOWN_JOB_TYPE: " + job.Type
						_ = st.UpdateCrawlJobStatus(context.Background(), job.ID, "failed", &msg)
					}

				}()

			}
		}
	}()
}

// runCrawlJob performs the actual crawl for a single job ID using the
// provided crawl request options.
func runCrawlJob(ctx context.Context, cfg *config.Config, st *store.Store, jobID uuid.UUID, req CrawlRequest) {
	// Derive discovery options from request and config.
	limit := cfg.Crawler.MaxPagesDefault
	if req.Limit != nil && *req.Limit > 0 {
		limit = *req.Limit
	}

	includeSubdomains := false
	if req.AllowSubdomains != nil {
		includeSubdomains = *req.AllowSubdomains
	}
	// When crawlEntireDomain is requested, always include subdomains for
	// broader coverage of the site.
	if req.CrawlEntireDomain != nil && *req.CrawlEntireDomain {
		includeSubdomains = true
	}

	ignoreQueryParams := true
	if req.IgnoreQueryParams != nil {
		ignoreQueryParams = *req.IgnoreQueryParams
	}

	allowExternal := false
	if req.AllowExternalLinks != nil {
		allowExternal = *req.AllowExternalLinks
	}

	sitemapMode := req.Sitemap
	if sitemapMode == "" {
		sitemapMode = "include"
	}

	timeout := time.Duration(cfg.Scraper.TimeoutMs) * time.Millisecond

	// Discover URLs
	mapRes, err := crawler.Map(ctx, crawler.MapOptions{
		URL:               req.URL,
		Limit:             limit,
		Search:            "",
		IncludeSubdomains: includeSubdomains,
		IgnoreQueryParams: ignoreQueryParams,
		AllowExternal:     allowExternal,
		SitemapMode:       sitemapMode,
		Timeout:           timeout,
		RespectRobots:     cfg.Robots.Respect,
		UserAgent:         cfg.Scraper.UserAgent,
	})
	if err != nil {
		msg := err.Error()
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
		return
	}

	urls := make([]string, 0, len(mapRes.Links)+1)
	urls = append(urls, req.URL)
	for _, l := range mapRes.Links {
		urls = append(urls, l.URL)
	}

	// Determine whether we should compute summaries and/or json/branding for this crawl.
	wantSummary := wantsFormat(req.Formats, "summary")
	hasJSON, jsonPrompt, jsonSchema := getJSONFormatConfig(req.Formats)
	wantBranding, brandingPrompt := getBrandingFormatConfig(req.Formats)
	wantLLM := wantSummary || hasJSON || wantBranding

	var (
		llmClient  llm.Client
		provider   llm.Provider
		modelName  string
		llmTimeout time.Duration
	)
	if wantLLM {
		var err error
		llmClient, provider, modelName, err = llm.NewClientFromConfig(cfg, "", "")
		if err != nil {
			msg := err.Error()
			_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
			return
		}
		llmTimeout = timeout
	}

	s := scraper.NewHTTPScraper(timeout)

	// Derive per-page scrape headers if provided at the crawl level.
	scrapeHeaders := map[string]string{}
	if req.ScrapeOptions != nil {
		for k, v := range req.ScrapeOptions.Headers {
			scrapeHeaders[k] = v
		}
	}

	maxPerJob := cfg.Worker.MaxConcurrentURLsPerJob

	if maxPerJob <= 0 {
		maxPerJob = 1
	}
	// Allow per-crawl overrides of URL concurrency, but never exceed the
	// global worker limit.
	if req.MaxConcurrency != nil && *req.MaxConcurrency > 0 && *req.MaxConcurrency < maxPerJob {
		maxPerJob = *req.MaxConcurrency
	}

	var successCount int32
	sem := make(chan struct{}, maxPerJob)
	// Use a channel to wait for all URL scrapes to finish.
	doneCh := make(chan struct{})

	go func() {
		for _, u := range urls {
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}

			u := u
			go func() {
				defer func() { <-sem }()

				select {
				case <-ctx.Done():
					return
				default:
				}

				// Build per-request headers, including crawl-level scrapeOptions
				// and any location-derived Accept-Language settings.
				headers := map[string]string{}
				for k, v := range scrapeHeaders {
					headers[k] = v
				}
				if req.ScrapeOptions != nil && req.ScrapeOptions.Location != nil {
					loc := req.ScrapeOptions.Location
					if len(loc.Languages) > 0 {
						headers["Accept-Language"] = strings.Join(loc.Languages, ",")
					} else if loc.Country != "" {
						headers["Accept-Language"] = loc.Country
					}
				}

				res, err := s.Scrape(ctx, scraper.Request{
					URL:       u,
					Headers:   headers,
					Timeout:   timeout,
					UserAgent: cfg.Scraper.UserAgent,
				})

				if err != nil {
					return
				}

				engine := res.Engine
				md := model.Metadata{
					Title:       toString(res.Metadata["title"]),
					Description: toString(res.Metadata["description"]),
					SourceURL:   toString(res.Metadata["sourceURL"]),
					StatusCode:  res.Status,
				}

				if wantSummary {
					fieldSpecs := []llm.FieldSpec{{
						Name:        "summary",
						Description: "Short natural-language summary of the page content.",
						Type:        "string",
					}}

					llmCtx, llmCancel := context.WithTimeout(ctx, llmTimeout)
					llmRes, err := llmClient.ExtractFields(llmCtx, llm.ExtractRequest{
						URL:      md.SourceURL,
						Markdown: res.Markdown,
						Fields:   fieldSpecs,
						Prompt:   "",
						Timeout:  llmTimeout,
						Strict:   false,
					})
					llmCancel()
					if err != nil {
						metrics.RecordLLMExtract(string(provider), modelName, false)
					} else {
						metrics.RecordLLMExtract(string(provider), modelName, true)
						if v, ok := llmRes.Fields["summary"]; ok {
							if s, ok2 := v.(string); ok2 {
								md.Summary = s
							}
						}
					}
				}

				if hasJSON {
					desc := "Arbitrary JSON object extracted from the page content."
					if len(jsonSchema) > 0 {
						if schemaBytes, err := json.Marshal(jsonSchema); err == nil {
							desc = desc + " Schema: " + string(schemaBytes)
						}
					}

					fieldSpecs := []llm.FieldSpec{{
						Name:        "json",
						Description: desc,
						Type:        "object",
					}}

					llmCtx, llmCancel := context.WithTimeout(ctx, llmTimeout)
					llmRes, err := llmClient.ExtractFields(llmCtx, llm.ExtractRequest{
						URL:      md.SourceURL,
						Markdown: res.Markdown,
						Fields:   fieldSpecs,
						Prompt:   jsonPrompt,
						Timeout:  llmTimeout,
						Strict:   false,
					})
					llmCancel()
					if err != nil {
						metrics.RecordLLMExtract(string(provider), modelName, false)
					} else {
						metrics.RecordLLMExtract(string(provider), modelName, true)
						if v, ok := llmRes.Fields["json"]; ok {
							if m, ok2 := v.(map[string]interface{}); ok2 {
								md.JSON = m
							} else {
								md.JSON = map[string]interface{}{"_value": v}
							}
						}
					}
				}

				if wantBranding {
					// Use a Firecrawl-style default prompt if the user did not
					// provide one in the formats array.
					if brandingPrompt == "" {
						brandingPrompt = "You are a brand design expert analyzing a website. Analyze the page and return a single JSON object describing the brand, matching this structure as closely as possible: " +
							"{colorScheme?: 'light'|'dark', colors?: {primary?: string, secondary?: string, accent?: string, background?: string, textPrimary?: string, textSecondary?: string, link?: string, success?: string, warning?: string, error?: string}, " +
							"typography?: {fontFamilies?: {primary?: string, heading?: string, code?: string}, fontStacks?: {primary?: string[], heading?: string[], body?: string[], paragraph?: string[]}, fontSizes?: {h1?: string, h2?: string, h3?: string, body?: string, small?: string}}, " +
							"spacing?: {baseUnit?: number, borderRadius?: string}, components?: {buttonPrimary?: {background?: string, textColor?: string, borderColor?: string, borderRadius?: string}, buttonSecondary?: {...}}, " +
							"images?: {logo?: string|null, favicon?: string|null, ogImage?: string|null}, personality?: {tone?: string, energy?: string, targetAudience?: string}}. " +
							"Only include fields you can infer with reasonable confidence."
					}

					descBranding := "Brand identity and design system information (colors, typography, logo, components, personality, etc.) extracted from the page, following Firecrawl's BrandingProfile conventions."

					fieldSpecs := []llm.FieldSpec{{
						Name:        "branding",
						Description: descBranding,
						Type:        "object",
					}}

					llmCtx, llmCancel := context.WithTimeout(ctx, llmTimeout)
					llmRes, err := llmClient.ExtractFields(llmCtx, llm.ExtractRequest{
						URL:      md.SourceURL,
						Markdown: res.Markdown,
						Fields:   fieldSpecs,
						Prompt:   brandingPrompt,
						Timeout:  llmTimeout,
						Strict:   false,
					})
					llmCancel()
					if err != nil {
						metrics.RecordLLMExtract(string(provider), modelName, false)
					} else {
						metrics.RecordLLMExtract(string(provider), modelName, true)
						if v, ok := llmRes.Fields["branding"]; ok {
							if m, ok2 := v.(map[string]interface{}); ok2 {
								normalizeBrandingImages(m)
								md.Branding = m
							} else {
								md.Branding = map[string]interface{}{"_value": v}
							}
						}
					}
				}

				metaBytes, err := json.Marshal(md)
				if err != nil {
					return
				}

				statusCode := int32(res.Status)
				markdown := res.Markdown
				html := res.HTML
				raw := res.RawHTML

				_ = st.AddDocument(ctx, jobID, res.URL, &markdown, &html, &raw, metaBytes, &statusCode, &engine)
				atomic.AddInt32(&successCount, 1)
			}()
		}

		// Wait for all goroutines to drain the semaphore.
		for i := 0; i < maxPerJob; i++ {
			sem <- struct{}{}
		}
		close(doneCh)
	}()

	select {
	case <-ctx.Done():
		msg := ctx.Err().Error()
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
		return
	case <-doneCh:
	}

	if atomic.LoadInt32(&successCount) == 0 {
		msg := "no pages successfully scraped"
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
		return
	}

	_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "completed", nil)
}

// runMapJob performs a map operation for a map job and stores the
// resulting MapResponse into the job's output field.
func runMapJob(ctx context.Context, cfg *config.Config, st *store.Store, jobID uuid.UUID, req MapRequest) {
	// Derive options from request and config
	limit := cfg.Crawler.MaxPagesDefault
	if req.Limit != nil && *req.Limit > 0 {
		limit = *req.Limit
	}

	includeSubdomains := false
	if req.IncludeSubdomains != nil {
		includeSubdomains = *req.IncludeSubdomains
	}

	ignoreQueryParams := true
	if req.IgnoreQueryParams != nil {
		ignoreQueryParams = *req.IgnoreQueryParams
	}

	allowExternal := false
	if req.AllowExternal != nil {
		allowExternal = *req.AllowExternal
	}

	sitemapMode := req.Sitemap
	if sitemapMode == "" {
		sitemapMode = "include"
	}

	timeoutMs := cfg.Scraper.TimeoutMs
	if req.Timeout != nil && *req.Timeout > 0 {
		timeoutMs = *req.Timeout
	}

	mapCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	res, err := crawler.Map(mapCtx, crawler.MapOptions{
		URL:               req.URL,
		Limit:             limit,
		Search:            req.Search,
		IncludeSubdomains: includeSubdomains,
		IgnoreQueryParams: ignoreQueryParams,
		AllowExternal:     allowExternal,
		SitemapMode:       sitemapMode,
		Timeout:           time.Duration(timeoutMs) * time.Millisecond,
		RespectRobots:     cfg.Robots.Respect,
		UserAgent:         cfg.Scraper.UserAgent,
	})
	if err != nil {
		msg := "MAP_FAILED: " + err.Error()
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
		return
	}

	linksResp := make([]MapLink, 0, len(res.Links))
	for _, l := range res.Links {
		linksResp = append(linksResp, MapLink{
			URL:         l.URL,
			Title:       l.Title,
			Description: l.Description,
		})
	}

	out := MapResponse{
		Success: true,
		Links:   linksResp,
		Warning: res.Warning,
	}

	output, err := json.Marshal(out)
	if err != nil {
		msg := "MAP_FAILED: failed to marshal map response: " + err.Error()
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
		return
	}

	if err := st.SetJobOutput(context.Background(), jobID, output); err != nil {
		msg := "MAP_FAILED: failed to persist job output: " + err.Error()
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
		return
	}

	_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "completed", nil)
}

// runExtractJob performs a multi-URL extract for an extract job and
// stores the resulting JSON object into the job's output field.
func runExtractJob(ctx context.Context, cfg *config.Config, st *store.Store, jobID uuid.UUID, req ExtractRequest) {
	// Determine URLs: prefer req.URLs, fall back to single URL if present.
	urls := req.URLs
	if len(urls) == 0 && req.URL != "" {
		urls = []string{req.URL}
	}
	if len(urls) == 0 {
		msg := "EXTRACT_FAILED: no urls provided for extract job"
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
		return
	}

	// Use the scraper timeout for both scraping and LLM operations.
	timeoutMs := cfg.Scraper.TimeoutMs

	// Scrape all URLs using the HTTP scraper (no browser for extract).
	s := scraper.NewHTTPScraper(time.Duration(timeoutMs) * time.Millisecond)

	scrapeCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	var combinedMarkdown strings.Builder
	for i, u := range urls {
		res, err := s.Scrape(scrapeCtx, scraper.Request{
			URL:       u,
			Headers:   map[string]string{},
			Timeout:   time.Duration(timeoutMs) * time.Millisecond,
			UserAgent: cfg.Scraper.UserAgent,
		})
		if err != nil {
			msg := "SCRAPE_FAILED: " + err.Error()
			_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
			return
		}

		if i > 0 {
			combinedMarkdown.WriteString("\n\n---\n\n")
		}
		combinedMarkdown.WriteString(fmt.Sprintf("URL: %s\n\n", u))
		combinedMarkdown.WriteString(res.Markdown)
	}

	markdown := combinedMarkdown.String()

	// Prepare LLM client.
	client, provider, modelName, err := llm.NewClientFromConfig(cfg, req.Provider, req.Model)
	if err != nil {
		msg := "LLM_NOT_CONFIGURED: " + err.Error()
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
		return
	}

	llmTimeout := time.Duration(timeoutMs) * time.Millisecond
	llmCtx, llmCancel := context.WithTimeout(ctx, llmTimeout)
	defer llmCancel()

	var filtered map[string]interface{}

	if len(req.Schema) > 0 {
		// Firecrawl-style JSON mode using a JSON Schema.
		desc := "Arbitrary JSON object extracted from the page content."
		if schemaBytes, err := json.Marshal(req.Schema); err == nil {
			desc = desc + " Schema: " + string(schemaBytes)
		}

		fieldSpecs := []llm.FieldSpec{{
			Name:        "json",
			Description: desc,
			Type:        "object",
		}}

		llmRes, err := client.ExtractFields(llmCtx, llm.ExtractRequest{
			URL:      urls[0],
			Markdown: markdown,
			Fields:   fieldSpecs,
			Prompt:   req.Prompt,
			Timeout:  llmTimeout,
			Strict:   false,
		})
		if err != nil {
			statusCode := "EXTRACT_FAILED"
			metrics.RecordLLMExtract(string(provider), modelName, false)
			msg := statusCode + ": " + err.Error()
			_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
			return
		}

		metrics.RecordLLMExtract(string(provider), modelName, true)

		// Prefer the "json" field when present, otherwise fall back to
		// the full fields map so we still return useful data when the
		// model omits the outer "json" key.
		if v, ok := llmRes.Fields["json"]; ok {
			if m, ok := v.(map[string]interface{}); ok {
				filtered = m
			} else {
				filtered = map[string]interface{}{"_value": v}
			}
		} else if len(llmRes.Fields) > 0 {
			filtered = llmRes.Fields
		}
	} else {
		// Legacy field-based extract mode.
		fieldSpecs := make([]llm.FieldSpec, 0, len(req.Fields))
		for _, f := range req.Fields {
			fieldSpecs = append(fieldSpecs, llm.FieldSpec{
				Name:        f.Name,
				Description: f.Description,
				Type:        f.Type,
			})
		}

		llmRes, err := client.ExtractFields(llmCtx, llm.ExtractRequest{
			URL:      urls[0],
			Markdown: markdown,
			Fields:   fieldSpecs,
			Prompt:   req.Prompt,
			Timeout:  llmTimeout,
			Strict:   req.Strict,
		})
		if err != nil {
			statusCode := "EXTRACT_FAILED"
			metrics.RecordLLMExtract(string(provider), modelName, false)
			msg := statusCode + ": " + err.Error()
			_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
			return
		}

		metrics.RecordLLMExtract(string(provider), modelName, true)

		// Filter fields to only those requested by name.
		requested := make(map[string]struct{}, len(req.Fields))
		for _, f := range req.Fields {
			requested[f.Name] = struct{}{}
		}

		filtered = make(map[string]interface{}, len(requested))
		for k, v := range llmRes.Fields {
			if _, ok := requested[k]; ok {
				filtered[k] = v
			}
		}

		if req.Strict && len(filtered) != len(requested) {
			msg := "EXTRACT_INCOMPLETE_FIELDS: LLM did not return all requested fields"
			_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
			return
		}
	}

	if filtered == nil || len(filtered) == 0 {
		msg := "EXTRACT_EMPTY_RESULT: LLM did not return any fields"
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
		return
	}

	// Persist only the structured JSON object (filtered) into job output.
	output, err := json.Marshal(filtered)
	if err != nil {
		msg := "EXTRACT_FAILED: failed to marshal extract result: " + err.Error()
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
		return
	}

	if err := st.SetJobOutput(context.Background(), jobID, output); err != nil {
		msg := "EXTRACT_FAILED: failed to persist job output: " + err.Error()
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
		return
	}

	_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "completed", nil)
}

// runBatchScrapeJob performs a batch scrape for a fixed list of URLs and
// stores each scraped page as a document associated with the job.
func runBatchScrapeJob(ctx context.Context, cfg *config.Config, st *store.Store, jobID uuid.UUID, req BatchScrapeRequest) {
	if len(req.URLs) == 0 {
		msg := "BATCH_SCRAPE_FAILED: no urls provided for batch scrape job"
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
		return
	}

	timeout := time.Duration(cfg.Scraper.TimeoutMs) * time.Millisecond
	s := scraper.NewHTTPScraper(timeout)

	maxPerJob := cfg.Worker.MaxConcurrentURLsPerJob
	if maxPerJob <= 0 {
		maxPerJob = 1
	}

	var successCount int32
	sem := make(chan struct{}, maxPerJob)
	doneCh := make(chan struct{})

	go func() {
		for _, u := range req.URLs {
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}

			u := u
			go func() {
				defer func() { <-sem }()

				select {
				case <-ctx.Done():
					return
				default:
				}

				res, err := s.Scrape(ctx, scraper.Request{
					URL:       u,
					Headers:   map[string]string{},
					Timeout:   timeout,
					UserAgent: cfg.Scraper.UserAgent,
				})
				if err != nil {
					return
				}

				engine := res.Engine
				md := model.Metadata{
					Title:       toString(res.Metadata["title"]),
					Description: toString(res.Metadata["description"]),
					SourceURL:   toString(res.Metadata["sourceURL"]),
					StatusCode:  res.Status,
				}

				metaBytes, err := json.Marshal(md)
				if err != nil {
					return
				}

				statusCode := int32(res.Status)
				markdown := res.Markdown
				html := res.HTML
				raw := res.RawHTML

				_ = st.AddDocument(ctx, jobID, res.URL, &markdown, &html, &raw, metaBytes, &statusCode, &engine)
				atomic.AddInt32(&successCount, 1)
			}()
		}

		for i := 0; i < maxPerJob; i++ {
			sem <- struct{}{}
		}
		close(doneCh)
	}()

	select {
	case <-ctx.Done():
		msg := ctx.Err().Error()
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
		return
	case <-doneCh:
	}

	if atomic.LoadInt32(&successCount) == 0 {
		msg := "BATCH_SCRAPE_FAILED: no pages successfully scraped"
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
		return
	}

	_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "completed", nil)
}

// runScrapeJob performs a single-page scrape for a scrape job and stores
// the resulting Document into the job's output field.
func runScrapeJob(ctx context.Context, cfg *config.Config, st *store.Store, jobID uuid.UUID, req ScrapeRequest) {
	// Derive timeout from request and config.
	timeoutMs := cfg.Scraper.TimeoutMs
	if req.Timeout != nil && *req.Timeout > 0 {
		timeoutMs = *req.Timeout
	}

	hasFormats := len(req.Formats) > 0

	// Determine whether screenshot format was requested and its options.
	hasScreenshot, screenshotFullPage := getScreenshotFormatConfig(req.Formats)

	// Choose scraper engine: HTTP by default, rod when requested and enabled.
	useBrowser := false
	if req.UseBrowser != nil {
		useBrowser = *req.UseBrowser
	}
	if hasScreenshot {
		// Screenshot always uses the browser engine.
		useBrowser = true
	}

	var engine scraper.Scraper
	if useBrowser {
		if !cfg.Rod.Enabled {
			if hasScreenshot {
				msg := "SCREENSHOT_NOT_AVAILABLE: screenshot format requires browser scraping, but rod is disabled in server configuration"
				_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
				return
			}
			engine = scraper.NewHTTPScraper(time.Duration(timeoutMs) * time.Millisecond)
		} else {
			engine = scraper.NewRodScraper(time.Duration(timeoutMs) * time.Millisecond)
		}
	} else {
		engine = scraper.NewHTTPScraper(time.Duration(timeoutMs) * time.Millisecond)
	}

	headers := map[string]string{}
	for k, v := range req.Headers {
		headers[k] = v
	}
	// Apply location settings to Accept-Language when provided.
	if req.Location != nil {
		if len(req.Location.Languages) > 0 {
			headers["Accept-Language"] = strings.Join(req.Location.Languages, ",")
		} else if req.Location.Country != "" {
			headers["Accept-Language"] = req.Location.Country
		}
	}

	scrapeReq := scraper.Request{
		URL:       req.URL,
		Headers:   headers,
		Timeout:   time.Duration(timeoutMs) * time.Millisecond,
		UserAgent: cfg.Scraper.UserAgent,
	}

	scrapeCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	res, err := engine.Scrape(scrapeCtx, scrapeReq)
	if err != nil {
		msg := "SCRAPE_FAILED: " + err.Error()
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
		return
	}

	md := model.Metadata{
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

	includeMarkdown := !hasFormats || wantsFormat(req.Formats, "markdown")
	includeHTML := !hasFormats || wantsFormat(req.Formats, "html")
	includeRawHTML := !hasFormats || wantsFormat(req.Formats, "rawHtml")
	includeLinks := !hasFormats || wantsFormat(req.Formats, "links")
	includeImages := !hasFormats || wantsFormat(req.Formats, "images")

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
		screenshotCtx, screenshotCancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
		defer screenshotCancel()

		shot, err := scraper.CaptureScreenshot(screenshotCtx, res.URL, time.Duration(timeoutMs)*time.Millisecond, screenshotFullPage)
		if err != nil {
			msg := "SCREENSHOT_FAILED: " + err.Error()
			_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
			return
		}

		doc.Screenshot = base64.StdEncoding.EncodeToString(shot)
	}

	// Optional summary format using the configured LLM provider when requested.
	if wantsFormat(req.Formats, "summary") {
		client, provider, modelName, err := llm.NewClientFromConfig(cfg, "", "")
		if err != nil {
			msg := "LLM_NOT_CONFIGURED: " + err.Error()
			_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
			return
		}

		fieldSpecs := []llm.FieldSpec{
			{
				Name:        "summary",
				Description: "Short natural-language summary of the page content.",
				Type:        "string",
			},
		}

		llmTimeout := time.Duration(timeoutMs) * time.Millisecond
		llmCtx, llmCancel := context.WithTimeout(ctx, llmTimeout)
		defer llmCancel()

		llmRes, err := client.ExtractFields(llmCtx, llm.ExtractRequest{
			URL:      req.URL,
			Markdown: res.Markdown,
			Fields:   fieldSpecs,
			Prompt:   "",
			Timeout:  llmTimeout,
			Strict:   false,
		})
		if err != nil {
			metrics.RecordLLMExtract(string(provider), modelName, false)
			msg := "SUMMARY_FAILED: " + err.Error()
			_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
			return
		}

		metrics.RecordLLMExtract(string(provider), modelName, true)

		if v, ok := llmRes.Fields["summary"]; ok {
			if s, ok := v.(string); ok {
				doc.Summary = s
			}
		}
	}

	// Optional json format using the configured LLM provider when requested.
	if hasJSON, jsonPrompt, jsonSchema := getJSONFormatConfig(req.Formats); hasJSON {
		client, provider, modelName, err := llm.NewClientFromConfig(cfg, "", "")
		if err != nil {
			msg := "LLM_NOT_CONFIGURED: " + err.Error()
			_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
			return
		}

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
		llmCtx, llmCancel := context.WithTimeout(ctx, llmTimeout)
		defer llmCancel()

		llmRes, err := client.ExtractFields(llmCtx, llm.ExtractRequest{
			URL:      md.SourceURL,
			Markdown: res.Markdown,
			Fields:   fieldSpecs,
			Prompt:   jsonPrompt,
			Timeout:  llmTimeout,
			Strict:   false,
		})

		if err != nil {
			metrics.RecordLLMExtract(string(provider), modelName, false)
			msg := "JSON_EXTRACT_FAILED: " + err.Error()
			_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
			return
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
	if hasBranding, brandingPrompt := getBrandingFormatConfig(req.Formats); hasBranding {
		client, provider, modelName, err := llm.NewClientFromConfig(cfg, "", "")
		if err != nil {
			msg := "LLM_NOT_CONFIGURED: " + err.Error()
			_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
			return
		}

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
		llmCtx, llmCancel := context.WithTimeout(ctx, llmTimeout)
		defer llmCancel()

		llmRes, err := client.ExtractFields(llmCtx, llm.ExtractRequest{
			URL:      req.URL,
			Markdown: res.Markdown,
			Fields:   fieldSpecs,
			Prompt:   brandingPrompt,
			Timeout:  llmTimeout,
			Strict:   false,
		})
		if err != nil {
			metrics.RecordLLMExtract(string(provider), modelName, false)
			msg := "BRANDING_FAILED: " + err.Error()
			_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
			return
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

	output, err := json.Marshal(doc)
	if err != nil {
		msg := "SCRAPE_FAILED: failed to marshal document: " + err.Error()
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
		return
	}

	if err := st.SetJobOutput(context.Background(), jobID, output); err != nil {
		msg := "SCRAPE_FAILED: failed to persist job output: " + err.Error()
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "failed", &msg)
		return
	}

	_ = st.UpdateCrawlJobStatus(context.Background(), jobID, "completed", nil)
}
