package jobs

import (
	"context"
	"time"

	"raito/internal/config"
	"raito/internal/metrics"
	"raito/internal/store"
)

// RetentionStats captures the number of records deleted by TTL cleanup.
type RetentionStats struct {
	DocumentsDeleted int64            `json:"documentsDeleted"`
	JobsDeleted      map[string]int64 `json:"jobsDeleted"`
}

// CleanupExpiredData deletes old jobs and documents based on retention
// settings so that the database does not grow without bound.
func CleanupExpiredData(ctx context.Context, cfg *config.Config, st *store.Store) RetentionStats {
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
