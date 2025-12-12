package http

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"raito/internal/config"
	"raito/internal/crawler"
	"raito/internal/model"
	"raito/internal/scraper"
	"raito/internal/store"
)

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

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

			// Determine how many new jobs we can start based on current concurrency.
			capacity := maxJobs - len(sem)
			if capacity <= 0 {
				continue
			}

			jobs, err := st.ListPendingCrawlJobs(ctx, int32(capacity))
			if err != nil {
				// TODO: add logging once logging is in place
				continue
			}

			for _, job := range jobs {
				job := job
				sem <- struct{}{}
				go func() {
					defer func() { <-sem }()

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

					// Derive a per-job timeout from scraper config.
					timeout := time.Duration(cfg.Scraper.TimeoutMs) * time.Millisecond
					jobCtx, cancel := context.WithTimeout(ctx, timeout)
					defer cancel()

					runCrawlJob(jobCtx, cfg, st, job.ID, req)
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

	s := scraper.NewHTTPScraper(timeout)

	maxPerJob := cfg.Worker.MaxConcurrentURLsPerJob
	if maxPerJob <= 0 {
		maxPerJob = 1
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

				res, err := s.Scrape(ctx, scraper.Request{
					URL:       u,
					Headers:   map[string]string{},
					Timeout:   timeout,
					UserAgent: cfg.Scraper.UserAgent,
				})
				if err != nil {
					return
				}

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

				_ = st.AddDocument(ctx, jobID, res.URL, &markdown, &html, &raw, metaBytes, &statusCode)
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
