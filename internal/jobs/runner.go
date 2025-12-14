package jobs

import (
	"context"
	"time"

	"raito/internal/config"
	"raito/internal/db"
	"raito/internal/store"
)

// MapJobExecutor executes a single map job.
type MapJobExecutor interface {
	ExecuteMapJob(ctx context.Context, job db.Job)
}

// CrawlJobExecutor executes a single crawl job.
type CrawlJobExecutor interface {
	ExecuteCrawlJob(ctx context.Context, job db.Job)
}

// ExtractJobExecutor executes a single extract job.
type ExtractJobExecutor interface {
	ExecuteExtractJob(ctx context.Context, job db.Job)
}

// BatchScrapeJobExecutor executes a single batch scrape job.
type BatchScrapeJobExecutor interface {
	ExecuteBatchScrapeJob(ctx context.Context, job db.Job)
}

// ScrapeJobExecutor executes a single scrape job (used by the
// job-queue backed /v1/scrape executor).
type ScrapeJobExecutor interface {
	ExecuteScrapeJob(ctx context.Context, job db.Job)
}

// Executors groups the concrete executors for each job type.
type Executors struct {
	Map         MapJobExecutor
	Crawl       CrawlJobExecutor
	Extract     ExtractJobExecutor
	BatchScrape BatchScrapeJobExecutor
	Scrape      ScrapeJobExecutor
}

// Runner is responsible for polling the jobs table and dispatching
// work to job-type-specific executors. It encapsulates concurrency
// limits, polling intervals, and periodic retention cleanup.
type Runner struct {
	cfg       *config.Config
	store     *store.Store
	executors Executors
}

// NewRunner constructs a Runner with the given configuration, store,
// and job executors. Any missing executor will cause jobs of that
// type to be marked as failed with an UNKNOWN_JOB_TYPE error.
func NewRunner(cfg *config.Config, st *store.Store, execs Executors) *Runner {
	return &Runner{
		cfg:       cfg,
		store:     st,
		executors: execs,
	}
}

// Start launches the worker loop in the current goroutine. Callers
// typically run this in its own goroutine and keep the process alive.
func (r *Runner) Start(ctx context.Context) {
	pollInterval := time.Duration(r.cfg.Worker.PollIntervalMs) * time.Millisecond
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}

	maxJobs := r.cfg.Worker.MaxConcurrentJobs
	if maxJobs <= 0 {
		maxJobs = 4
	}

	sem := make(chan struct{}, maxJobs)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	var lastCleanup time.Time
	cleanupInterval := time.Duration(r.cfg.Retention.CleanupIntervalMinutes) * time.Minute
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
		if r.cfg.Retention.Enabled {
			now := time.Now().UTC()
			if lastCleanup.IsZero() || now.Sub(lastCleanup) >= cleanupInterval {
				_ = CleanupExpiredData(ctx, r.cfg, r.store)
				lastCleanup = now
			}
		}

		// Determine how many new jobs we can start based on current concurrency.
		capacity := maxJobs - len(sem)
		if capacity <= 0 {
			continue
		}

		jobs, err := r.store.ListPendingJobs(ctx, int32(capacity))
		if err != nil {
			// TODO: add logging once structured logging is available here.
			continue
		}

		for _, job := range jobs {
			job := job
			sem <- struct{}{}
			go func() {
				defer func() { <-sem }()
				r.dispatchJob(ctx, job)
			}()
		}
	}
}

func (r *Runner) dispatchJob(ctx context.Context, job db.Job) {
	// Delegate to the appropriate executor based on the job type.
	switch job.Type {
	case "crawl":
		if r.executors.Crawl != nil {
			r.executors.Crawl.ExecuteCrawlJob(ctx, job)
			return
		}
	case "scrape":
		if r.executors.Scrape != nil {
			r.executors.Scrape.ExecuteScrapeJob(ctx, job)
			return
		}
	case "map":
		if r.executors.Map != nil {
			r.executors.Map.ExecuteMapJob(ctx, job)
			return
		}
	case "extract":
		if r.executors.Extract != nil {
			r.executors.Extract.ExecuteExtractJob(ctx, job)
			return
		}
	case "batch_scrape":
		if r.executors.BatchScrape != nil {
			r.executors.BatchScrape.ExecuteBatchScrapeJob(ctx, job)
			return
		}
	}

	// Unknown or unconfigured job type; mark as failed.
	msg := "UNKNOWN_JOB_TYPE: " + job.Type
	_ = r.store.UpdateCrawlJobStatus(context.Background(), job.ID, string(StatusFailed), &msg)
}
