package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"raito/internal/config"
	"raito/internal/crawler"
	"raito/internal/db"
	"raito/internal/jobs"
	"raito/internal/llm"
	"raito/internal/metrics"
	"raito/internal/model"
	"raito/internal/scraper"
	"raito/internal/scrapeutil"
	"raito/internal/services"
	"raito/internal/store"
)

// StartCrawlWorker launches a background worker that periodically polls the
// database for pending jobs and processes them using the shared jobs.Runner.
func StartCrawlWorker(ctx context.Context, cfg *config.Config, st *store.Store) {
	// Wire up the job executors that know how to handle each job type.
	execs := jobs.Executors{
		Map:         NewMapJobExecutor(cfg, st),
		Crawl:       NewCrawlJobExecutor(cfg, st),
		Extract:     NewExtractJobExecutor(cfg, st),
		BatchScrape: NewBatchScrapeJobExecutor(cfg, st),
		Scrape:      NewScrapeJobExecutor(cfg, st),
	}

	runner := jobs.NewRunner(cfg, st, execs)
	go runner.Start(ctx)
}

// crawlJobExecutor implements jobs.CrawlJobExecutor using the existing
// crawl job implementation in this package.
type crawlJobExecutor struct {
	cfg *config.Config
	st  *store.Store
}

func NewCrawlJobExecutor(cfg *config.Config, st *store.Store) jobs.CrawlJobExecutor {
	return &crawlJobExecutor{cfg: cfg, st: st}
}

func (e *crawlJobExecutor) ExecuteCrawlJob(ctx context.Context, job db.Job) {
	var req CrawlRequest
	if err := json.Unmarshal(job.Input, &req); err != nil {
		msg := "invalid crawl job input: " + err.Error()
		_ = e.st.UpdateCrawlJobStatus(context.Background(), job.ID, string(jobs.StatusFailed), &msg)
		return
	}

	// Fallback: ensure URL is set even if the stored input was minimal.
	if req.URL == "" {
		req.URL = job.Url
	}

	// Mark job running before we start work.
	_ = e.st.UpdateCrawlJobStatus(context.Background(), job.ID, string(jobs.StatusRunning), nil)

	// Let the job inherit the worker context; per-request timeouts are
	// applied inside runCrawlJob for HTTP and LLM.
	runCrawlJob(ctx, e.cfg, e.st, job.ID, req)
}

// scrapeJobExecutor implements jobs.ScrapeJobExecutor using the existing
// scrape job implementation in this package.
type scrapeJobExecutor struct {
	cfg *config.Config
	st  *store.Store
}

func NewScrapeJobExecutor(cfg *config.Config, st *store.Store) jobs.ScrapeJobExecutor {
	return &scrapeJobExecutor{cfg: cfg, st: st}
}

func (e *scrapeJobExecutor) ExecuteScrapeJob(ctx context.Context, job db.Job) {
	var req ScrapeRequest
	if err := json.Unmarshal(job.Input, &req); err != nil {
		msg := "SCRAPE_FAILED: invalid scrape job input: " + err.Error()
		_ = e.st.UpdateCrawlJobStatus(context.Background(), job.ID, string(jobs.StatusFailed), &msg)
		return
	}

	if req.URL == "" {
		req.URL = job.Url
	}

	_ = e.st.UpdateCrawlJobStatus(context.Background(), job.ID, string(jobs.StatusRunning), nil)

	runScrapeJob(ctx, e.cfg, e.st, job.ID, req)
}

// mapJobExecutor implements jobs.MapJobExecutor using the existing
// map job implementation in this package.
type mapJobExecutor struct {
	cfg *config.Config
	st  *store.Store
}

func NewMapJobExecutor(cfg *config.Config, st *store.Store) jobs.MapJobExecutor {
	return &mapJobExecutor{cfg: cfg, st: st}
}

func (e *mapJobExecutor) ExecuteMapJob(ctx context.Context, job db.Job) {
	var req MapRequest
	if err := json.Unmarshal(job.Input, &req); err != nil {
		msg := "MAP_FAILED: invalid map job input: " + err.Error()
		_ = e.st.UpdateCrawlJobStatus(context.Background(), job.ID, string(jobs.StatusFailed), &msg)
		return
	}

	if req.URL == "" {
		req.URL = job.Url
	}

	_ = e.st.UpdateCrawlJobStatus(context.Background(), job.ID, string(jobs.StatusRunning), nil)

	runMapJob(ctx, e.cfg, e.st, job.ID, req)
}

// extractJobExecutor implements jobs.ExtractJobExecutor using the existing
// extract job implementation in this package.
type extractJobExecutor struct {
	cfg *config.Config
	st  *store.Store
}

func NewExtractJobExecutor(cfg *config.Config, st *store.Store) jobs.ExtractJobExecutor {
	return &extractJobExecutor{cfg: cfg, st: st}
}

func (e *extractJobExecutor) ExecuteExtractJob(ctx context.Context, job db.Job) {
	var req ExtractRequest
	if err := json.Unmarshal(job.Input, &req); err != nil {
		msg := "EXTRACT_FAILED: invalid extract job input: " + err.Error()
		_ = e.st.UpdateCrawlJobStatus(context.Background(), job.ID, string(jobs.StatusFailed), &msg)
		return
	}

	if len(req.URLs) == 0 && job.Url != "" {
		req.URLs = []string{job.Url}
	}

	_ = e.st.UpdateCrawlJobStatus(context.Background(), job.ID, string(jobs.StatusRunning), nil)

	runExtractJob(ctx, e.cfg, e.st, job.ID, req)
}

// batchScrapeJobExecutor implements jobs.BatchScrapeJobExecutor using the
// existing batch scrape job implementation in this package.
type batchScrapeJobExecutor struct {
	cfg *config.Config
	st  *store.Store
}

func NewBatchScrapeJobExecutor(cfg *config.Config, st *store.Store) jobs.BatchScrapeJobExecutor {
	return &batchScrapeJobExecutor{cfg: cfg, st: st}
}

func (e *batchScrapeJobExecutor) ExecuteBatchScrapeJob(ctx context.Context, job db.Job) {
	var req BatchScrapeRequest
	if err := json.Unmarshal(job.Input, &req); err != nil {
		msg := "BATCH_SCRAPE_FAILED: invalid batch scrape job input: " + err.Error()
		_ = e.st.UpdateCrawlJobStatus(context.Background(), job.ID, string(jobs.StatusFailed), &msg)
		return
	}

	_ = e.st.UpdateCrawlJobStatus(context.Background(), job.ID, string(jobs.StatusRunning), nil)

	runBatchScrapeJob(ctx, e.cfg, e.st, job.ID, req)
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
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
		return
	}

	urls := make([]string, 0, len(mapRes.Links)+1)
	urls = append(urls, req.URL)
	for _, l := range mapRes.Links {
		urls = append(urls, l.URL)
	}

	// Determine whether we should compute summaries and/or json/branding for this crawl.
	wantSummary := scrapeutil.WantsFormat(req.Formats, "summary")
	hasJSON, jsonPrompt, jsonSchema := scrapeutil.GetJSONFormatConfig(req.Formats)
	wantBranding, brandingPrompt := scrapeutil.GetBrandingFormatConfig(req.Formats)
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
			_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
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
					Title:       scrapeutil.ToString(res.Metadata["title"]),
					Description: scrapeutil.ToString(res.Metadata["description"]),
					SourceURL:   scrapeutil.ToString(res.Metadata["sourceURL"]),
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
							if m, ok := v.(map[string]interface{}); ok {
								scrapeutil.NormalizeBrandingImages(m)
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
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
		return
	case <-doneCh:
	}

	if atomic.LoadInt32(&successCount) == 0 {
		msg := "no pages successfully scraped"
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
		return
	}

	_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusCompleted), nil)
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
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
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
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
		return
	}

	if err := st.SetJobOutput(context.Background(), jobID, output); err != nil {
		msg := "MAP_FAILED: failed to persist job output: " + err.Error()
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
		return
	}

	_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusCompleted), nil)
}

// runExtractJob performs a multi-URL extract for an extract job and
// stores the resulting JSON object into the job's output field.
func runExtractJob(ctx context.Context, cfg *config.Config, st *store.Store, jobID uuid.UUID, req ExtractRequest) {
	urls := req.URLs
	if len(urls) == 0 {
		msg := "EXTRACT_FAILED: no urls provided for extract job"
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
		return
	}

	if len(req.Schema) == 0 {
		msg := "EXTRACT_FAILED: no schema provided for extract job"
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
		return
	}

	// Use the scraper timeout for both scraping and LLM operations.
	timeoutMs := cfg.Scraper.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 30000
	}

	ignoreInvalid := false
	if req.IgnoreInvalidURLs != nil {
		ignoreInvalid = *req.IgnoreInvalidURLs
	}

	showSources := false
	if req.ShowSources != nil {
		showSources = *req.ShowSources
	}

	// Prepare HTTP scraper (no browser for extract).
	s := scraper.NewHTTPScraper(time.Duration(timeoutMs) * time.Millisecond)

	// Prepare LLM client.
	client, provider, modelName, err := llm.NewClientFromConfig(cfg, req.Provider, req.Model)
	if err != nil {
		metrics.RecordExtractJob("", "", "failed")
		msg := "LLM_NOT_CONFIGURED: " + err.Error()
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
		return
	}

	llmTimeout := time.Duration(timeoutMs) * time.Millisecond

	// Build a combined prompt including an optional system prompt.
	buildPrompt := func(userPrompt string) string {
		if req.SystemPrompt == "" {
			return userPrompt
		}
		if userPrompt == "" {
			return req.SystemPrompt
		}
		return req.SystemPrompt + "\n\n" + userPrompt
	}

	results := make([]map[string]interface{}, 0, len(urls))
	sources := make([]map[string]interface{}, 0, len(urls))
	var successCount int

	// Derive shared scrape headers from scrapeOptions when provided.
	baseHeaders := map[string]string{}
	if req.ScrapeOptions != nil {
		for k, v := range req.ScrapeOptions.Headers {
			baseHeaders[k] = v
		}
		if req.ScrapeOptions.Location != nil {
			loc := req.ScrapeOptions.Location
			if len(loc.Languages) > 0 {
				baseHeaders["Accept-Language"] = strings.Join(loc.Languages, ",")
			} else if loc.Country != "" {
				baseHeaders["Accept-Language"] = loc.Country
			}
		}
	}

	for _, u := range urls {
		// Scrape the URL first.
		headers := map[string]string{}
		for k, v := range baseHeaders {
			headers[k] = v
		}

		res, err := s.Scrape(ctx, scraper.Request{
			URL:       u,
			Headers:   headers,
			Timeout:   time.Duration(timeoutMs) * time.Millisecond,
			UserAgent: cfg.Scraper.UserAgent,
		})
		if err != nil {
			if ignoreInvalid {
				results = append(results, map[string]interface{}{
					"url":     u,
					"success": false,
					"error":   "SCRAPE_FAILED: " + err.Error(),
				})
				if showSources {
					sources = append(sources, map[string]interface{}{
						"url":        u,
						"statusCode": 0,
						"error":      err.Error(),
					})
				}
				continue
			}
			msg := "SCRAPE_FAILED: " + err.Error()
			_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
			return
		}

		// Firecrawl-style JSON mode using a JSON Schema, one LLM call per URL.
		desc := "Arbitrary JSON object extracted from the page content."
		if schemaBytes, err := json.Marshal(req.Schema); err == nil {
			desc = desc + " Schema: " + string(schemaBytes)
		}

		fieldSpecs := []llm.FieldSpec{{
			Name:        "json",
			Description: desc,
			Type:        "object",
		}}

		llmCtx, llmCancel := context.WithTimeout(ctx, llmTimeout)
		llmRes, err := client.ExtractFields(llmCtx, llm.ExtractRequest{
			URL:      u,
			Markdown: res.Markdown,
			Fields:   fieldSpecs,
			Prompt:   buildPrompt(req.Prompt),
			Timeout:  llmTimeout,
			Strict:   false,
		})
		llmCancel()
		if err != nil {
			metrics.RecordLLMExtract(string(provider), modelName, false)
			if ignoreInvalid {
				results = append(results, map[string]interface{}{
					"url":     u,
					"success": false,
					"error":   "EXTRACT_FAILED: " + err.Error(),
				})
				if showSources {
					sources = append(sources, map[string]interface{}{
						"url":        u,
						"statusCode": res.Status,
						"error":      err.Error(),
					})
				}
				continue
			}

			msg := "EXTRACT_FAILED: " + err.Error()
			_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
			return
		}

		metrics.RecordLLMExtract(string(provider), modelName, true)

		var jsonValue map[string]interface{}
		if v, ok := llmRes.Fields["json"]; ok {
			if m, ok := v.(map[string]interface{}); ok {
				jsonValue = m
			} else {
				jsonValue = map[string]interface{}{"_value": v}
			}
		} else if len(llmRes.Fields) > 0 {
			// Fallback: ensure we still return something useful.
			jsonValue = llmRes.Fields
		}

		if jsonValue == nil || len(jsonValue) == 0 {
			if ignoreInvalid {
				results = append(results, map[string]interface{}{
					"url":     u,
					"success": false,
					"error":   "EXTRACT_EMPTY_RESULT: LLM did not return any fields",
				})
				if showSources {
					sources = append(sources, map[string]interface{}{
						"url":        u,
						"statusCode": res.Status,
						"error":      "LLM empty result",
					})
				}
				continue
			}
			msg := "EXTRACT_EMPTY_RESULT: LLM did not return any fields"
			_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
			return
		}

		results = append(results, map[string]interface{}{
			"url":     u,
			"success": true,
			"json":    jsonValue,
		})
		if showSources {
			sources = append(sources, map[string]interface{}{
				"url":        u,
				"statusCode": res.Status,
				"error":      "",
			})
		}
		successCount++
	}

	if successCount == 0 {
		metrics.RecordExtractJob(string(provider), modelName, "failed")
		if len(results) > 0 {
			metrics.RecordExtractResults(string(provider), 0, len(results))
		}
		msg := "EXTRACT_EMPTY_RESULT: no URLs produced extracted JSON"
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
		return
	}

	payload := map[string]interface{}{
		"results": results,
	}
	if showSources && len(sources) > 0 {
		payload["sources"] = sources
	}

	output, err := json.Marshal(payload)
	if err != nil {
		metrics.RecordExtractJob(string(provider), modelName, "failed")
		metrics.RecordExtractResults(string(provider), successCount, len(results)-successCount)
		msg := "EXTRACT_FAILED: failed to marshal extract result: " + err.Error()
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
		return
	}

	if err := st.SetJobOutput(context.Background(), jobID, output); err != nil {
		metrics.RecordExtractJob(string(provider), modelName, "failed")
		metrics.RecordExtractResults(string(provider), successCount, len(results)-successCount)
		msg := "EXTRACT_FAILED: failed to persist extract result: " + err.Error()
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
		return
	}

	metrics.RecordExtractJob(string(provider), modelName, "completed")
	metrics.RecordExtractResults(string(provider), successCount, len(results)-successCount)

	_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusCompleted), nil)
}

// runBatchScrapeJob performs a batch scrape for a fixed list of URLs and
// stores each scraped page as a document associated with the job.
func runBatchScrapeJob(ctx context.Context, cfg *config.Config, st *store.Store, jobID uuid.UUID, req BatchScrapeRequest) {
	if len(req.URLs) == 0 {
		msg := "BATCH_SCRAPE_FAILED: no urls provided for batch scrape job"
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
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
					Title:       scrapeutil.ToString(res.Metadata["title"]),
					Description: scrapeutil.ToString(res.Metadata["description"]),
					SourceURL:   scrapeutil.ToString(res.Metadata["sourceURL"]),
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
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
		return
	case <-doneCh:
	}

	if atomic.LoadInt32(&successCount) == 0 {
		msg := "BATCH_SCRAPE_FAILED: no pages successfully scraped"
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
		return
	}

	_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusCompleted), nil)
}

// runScrapeJob performs a single-page scrape for a scrape job and stores
// the resulting Document into the job's output field.
func runScrapeJob(ctx context.Context, cfg *config.Config, st *store.Store, jobID uuid.UUID, req ScrapeRequest) {
	// Derive timeout from request and config.
	timeoutMs := cfg.Scraper.TimeoutMs
	if req.Timeout != nil && *req.Timeout > 0 {
		timeoutMs = *req.Timeout
	}

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
				_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
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
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
		return
	}

	svc := services.NewScrapeService(cfg)
	svcRes, err := svc.Scrape(scrapeCtx, &services.ScrapeRequest{
		Result:  res,
		Formats: req.Formats,
	})
	if err != nil {
		msg := "SCRAPE_FAILED: " + err.Error()
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
		return
	}
	if svcRes == nil || svcRes.Document == nil {
		msg := "SCRAPE_FAILED: empty scrape document"
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
		return
	}

	doc := (*Document)(svcRes.Document)
	md := doc.Metadata

	// Optional screenshot format using the browser engine when requested.
	if hasScreenshot {
		screenshotCtx, screenshotCancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
		defer screenshotCancel()

		shot, err := scraper.CaptureScreenshot(screenshotCtx, res.URL, time.Duration(timeoutMs)*time.Millisecond, screenshotFullPage)
		if err != nil {
			msg := "SCREENSHOT_FAILED: " + err.Error()
			_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
			return
		}

		doc.Screenshot = base64.StdEncoding.EncodeToString(shot)
	}

	// Optional summary format using the configured LLM provider when requested.
	if scrapeutil.WantsFormat(req.Formats, "summary") {
		client, provider, modelName, err := llm.NewClientFromConfig(cfg, "", "")
		if err != nil {
			msg := "LLM_NOT_CONFIGURED: " + err.Error()
			_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
			return
		}

		fieldSpecs := []llm.FieldSpec{{
			Name:        "summary",
			Description: "Short natural-language summary of the page content.",
			Type:        "string",
		}}

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
			_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
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
	if hasJSON, jsonPrompt, jsonSchema := scrapeutil.GetJSONFormatConfig(req.Formats); hasJSON {
		client, provider, modelName, err := llm.NewClientFromConfig(cfg, "", "")
		if err != nil {
			msg := "LLM_NOT_CONFIGURED: " + err.Error()
			_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
			return
		}

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
			_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
			return
		}

		metrics.RecordLLMExtract(string(provider), modelName, true)

		if v, ok := llmRes.Fields["json"]; ok {
			if m, ok := v.(map[string]interface{}); ok {
				doc.JSON = m
			} else {
				doc.JSON = map[string]interface{}{"_value": v}
			}
		}
	}

	// Optional branding format using the configured LLM provider when requested.
	if hasBranding, brandingPrompt := scrapeutil.GetBrandingFormatConfig(req.Formats); hasBranding {
		client, provider, modelName, err := llm.NewClientFromConfig(cfg, "", "")
		if err != nil {
			msg := "LLM_NOT_CONFIGURED: " + err.Error()
			_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
			return
		}

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
			_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
			return
		}

		metrics.RecordLLMExtract(string(provider), modelName, true)

		if v, ok := llmRes.Fields["branding"]; ok {
			if m, ok := v.(map[string]interface{}); ok {
				scrapeutil.NormalizeBrandingImages(m)
				doc.Branding = m
			} else {
				doc.Branding = map[string]interface{}{"_value": v}
			}
		}
	}

	output, err := json.Marshal(doc)
	if err != nil {
		msg := "SCRAPE_FAILED: failed to marshal document: " + err.Error()
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
		return
	}

	if err := st.SetJobOutput(context.Background(), jobID, output); err != nil {
		msg := "SCRAPE_FAILED: failed to persist job output: " + err.Error()
		_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusFailed), &msg)
		return
	}

	_ = st.UpdateCrawlJobStatus(context.Background(), jobID, string(jobs.StatusCompleted), nil)
}
