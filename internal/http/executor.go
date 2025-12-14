package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"raito/internal/config"
	"raito/internal/store"
)

// WorkExecutor defines an abstraction for executing heavy operations
// like scrape/map/extract/crawl, backed by the jobs table.
type WorkExecutor interface {
	Scrape(ctx context.Context, req *ScrapeRequest) (*ScrapeResponse, error)
	Map(ctx context.Context, req *MapRequest) (*MapResponse, error)
	Extract(ctx context.Context, req *ExtractRequest) (*ExtractResponse, error)
}

// JobQueueExecutor is a WorkExecutor implementation that enqueues jobs
// into the database and, for synchronous operations like Scrape, waits
// for completion before returning the result.
type JobQueueExecutor struct {
	cfg    *config.Config
	st     *store.Store
	logger *slog.Logger
}

func NewJobQueueExecutor(cfg *config.Config, st *store.Store, logger *slog.Logger) *JobQueueExecutor {
	return &JobQueueExecutor{cfg: cfg, st: st, logger: logger}
}

func (e *JobQueueExecutor) logInfo(msg string, args ...any) {
	if e.logger != nil {
		e.logger.Info(msg, args...)
	}
}

// Scrape enqueues a scrape job and waits for completion, returning
// a ScrapeResponse that mirrors the direct HTTP implementation but
// executes the heavy work on a worker via the jobs table.
func (e *JobQueueExecutor) Scrape(ctx context.Context, req *ScrapeRequest) (*ScrapeResponse, error) {
	if req == nil {
		return &ScrapeResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "nil scrape request",
		}, nil
	}

	// Derive the underlying work timeout from config and request override.
	workTimeoutMs := e.cfg.Scraper.TimeoutMs
	if req.Timeout != nil && *req.Timeout > 0 {
		workTimeoutMs = *req.Timeout
	}

	// Use a separate timeout for how long the API waits for the job.
	waitTimeoutMs := e.cfg.Worker.SyncJobWaitTimeoutMs
	if waitTimeoutMs <= 0 {
		waitTimeoutMs = workTimeoutMs
	}

	waitCtx := ctx
	var cancel context.CancelFunc
	if waitTimeoutMs > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, time.Duration(waitTimeoutMs)*time.Millisecond)
		defer cancel()
	}

	// Generate a job ID (prefer uuidv7 when available).
	jobID := func() uuid.UUID {
		if id, err := uuid.NewV7(); err == nil {
			return id
		}
		return uuid.New()
	}()

	// Enqueue the scrape job as a high-priority, synchronous job.
	var tenantID *uuid.UUID
	if val := ctx.Value("tenant_id"); val != nil {
		if tid, ok := val.(uuid.UUID); ok {
			tenantID = &tid
		}
	}
	if _, err := e.st.CreateJob(waitCtx, jobID, "scrape", req.URL, req, true, 100, tenantID); err != nil {
		return nil, err
	}

	e.logInfo("scrape_enqueued",
		"scrape_id", jobID.String(),
		"url", req.URL,
		"has_formats", len(req.Formats) > 0,
	)

	// Poll for job completion until it is completed/failed or the
	// context times out.
	pollInterval := 100 * time.Millisecond
	lastStatus := ""

	for {
		select {
		case <-waitCtx.Done():
			// Distinguish between timeout-before-start and timeout-during-run.
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				if lastStatus == "" || lastStatus == "pending" {
					e.logInfo("scrape_failed",
						"scrape_id", jobID.String(),
						"status", lastStatus,
						"code", "JOB_NOT_STARTED",
						"error", "scrape job did not start before timeout",
					)
					return &ScrapeResponse{
						Success: false,
						Code:    "JOB_NOT_STARTED",
						Error:   "scrape job did not start before timeout",
					}, nil
				}
				e.logInfo("scrape_failed",
					"scrape_id", jobID.String(),
					"status", lastStatus,
					"code", "SCRAPE_TIMEOUT",
					"error", "scrape job did not complete before timeout",
				)
				return &ScrapeResponse{
					Success: false,
					Code:    "SCRAPE_TIMEOUT",
					Error:   "scrape job did not complete before timeout",
				}, nil
			}
			return nil, waitCtx.Err()
		case <-time.After(pollInterval):
		}

		job, err := e.st.GetJobByID(waitCtx, jobID)
		if err != nil {
			return nil, err
		}

		lastStatus = job.Status

		switch job.Status {
		case "pending", "running":
			// Still in progress; continue polling.
			continue
		case "completed":
			if !job.Output.Valid || len(job.Output.RawMessage) == 0 {
				return nil, fmt.Errorf("scrape job %s completed with empty output", jobID.String())
			}

			var doc Document
			if err := json.Unmarshal(job.Output.RawMessage, &doc); err != nil {
				return nil, err
			}

			e.logInfo("scrape_completed",
				"scrape_id", jobID.String(),
				"status", job.Status,
			)

			return &ScrapeResponse{
				Success: true,
				Data:    &doc,
			}, nil
		case "failed":
			code := "SCRAPE_FAILED"
			msg := "scrape job failed"
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

			e.logInfo("scrape_failed",
				"scrape_id", jobID.String(),
				"status", job.Status,
				"code", code,
				"error", msg,
			)

			return &ScrapeResponse{
				Success: false,
				Code:    code,
				Error:   msg,
			}, nil
		default:
			return nil, fmt.Errorf("unexpected scrape job status %q", job.Status)
		}
	}
}

// Map enqueues a map job and waits for completion, returning a
// MapResponse that mirrors the direct HTTP implementation.
func (e *JobQueueExecutor) Map(ctx context.Context, req *MapRequest) (*MapResponse, error) {
	if req == nil {
		return &MapResponse{
			Success: false,
			Links:   []MapLink{},
			Code:    "BAD_REQUEST",
			Error:   "nil map request",
		}, nil
	}

	workTimeoutMs := e.cfg.Scraper.TimeoutMs
	if req.Timeout != nil && *req.Timeout > 0 {
		workTimeoutMs = *req.Timeout
	}

	waitTimeoutMs := e.cfg.Worker.SyncJobWaitTimeoutMs
	if waitTimeoutMs <= 0 {
		waitTimeoutMs = workTimeoutMs
	}

	waitCtx := ctx
	var cancel context.CancelFunc
	if waitTimeoutMs > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, time.Duration(waitTimeoutMs)*time.Millisecond)
		defer cancel()
	}

	jobID := func() uuid.UUID {
		if id, err := uuid.NewV7(); err == nil {
			return id
		}
		return uuid.New()
	}()

	var tenantID *uuid.UUID
	if val := ctx.Value("tenant_id"); val != nil {
		if tid, ok := val.(uuid.UUID); ok {
			tenantID = &tid
		}
	}

	if _, err := e.st.CreateJob(waitCtx, jobID, "map", req.URL, req, true, 100, tenantID); err != nil {
		return nil, err
	}

	e.logInfo("map_enqueued",
		"map_id", jobID.String(),
		"url", req.URL,
		"limit", req.Limit,
	)

	pollInterval := 100 * time.Millisecond
	lastStatus := ""

	for {
		select {
		case <-waitCtx.Done():
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				if lastStatus == "" || lastStatus == "pending" {
					e.logInfo("map_failed",
						"map_id", jobID.String(),
						"status", lastStatus,
						"code", "JOB_NOT_STARTED",
						"error", "map job did not start before timeout",
					)
					return &MapResponse{
						Success: false,
						Links:   []MapLink{},
						Code:    "JOB_NOT_STARTED",
						Error:   "map job did not start before timeout",
					}, nil
				}
				e.logInfo("map_failed",
					"map_id", jobID.String(),
					"status", lastStatus,
					"code", "MAP_TIMEOUT",
					"error", "map job did not complete before timeout",
				)
				return &MapResponse{
					Success: false,
					Links:   []MapLink{},
					Code:    "MAP_TIMEOUT",
					Error:   "map job did not complete before timeout",
				}, nil
			}
			return nil, waitCtx.Err()
		case <-time.After(pollInterval):
		}

		job, err := e.st.GetJobByID(waitCtx, jobID)
		if err != nil {
			return nil, err
		}

		lastStatus = job.Status

		switch job.Status {
		case "pending", "running":
			continue
		case "completed":
			if !job.Output.Valid || len(job.Output.RawMessage) == 0 {
				return nil, fmt.Errorf("map job %s completed with empty output", jobID.String())
			}

			var res MapResponse
			if err := json.Unmarshal(job.Output.RawMessage, &res); err != nil {
				return nil, err
			}

			e.logInfo("map_completed",
				"map_id", jobID.String(),
				"status", job.Status,
			)

			return &res, nil
		case "failed":
			code := "MAP_FAILED"
			msg := "map job failed"
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

			e.logInfo("map_failed",
				"map_id", jobID.String(),
				"status", job.Status,
				"code", code,
				"error", msg,
			)

			return &MapResponse{
				Success: false,
				Links:   []MapLink{},
				Code:    code,
				Error:   msg,
			}, nil
		default:
			return nil, fmt.Errorf("unexpected map job status %q", job.Status)
		}
	}
}

// Extract enqueues an extract job and waits for completion, returning
// an ExtractResponse that mirrors the direct HTTP implementation.
func (e *JobQueueExecutor) Extract(ctx context.Context, req *ExtractRequest) (*ExtractResponse, error) {
	if req == nil {
		return &ExtractResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "nil extract request",
		}, nil
	}

	workTimeoutMs := e.cfg.Scraper.TimeoutMs

	waitTimeoutMs := e.cfg.Worker.SyncJobWaitTimeoutMs
	if waitTimeoutMs <= 0 {
		waitTimeoutMs = workTimeoutMs
	}

	waitCtx := ctx
	var cancel context.CancelFunc
	if waitTimeoutMs > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, time.Duration(waitTimeoutMs)*time.Millisecond)
		defer cancel()
	}

	jobID := func() uuid.UUID {
		if id, err := uuid.NewV7(); err == nil {
			return id
		}
		return uuid.New()
	}()

	var tenantID *uuid.UUID
	if val := ctx.Value("tenant_id"); val != nil {
		if tid, ok := val.(uuid.UUID); ok {
			tenantID = &tid
		}
	}

	primaryURL := ""
	if len(req.URLs) > 0 {
		primaryURL = req.URLs[0]
	}

	if _, err := e.st.CreateJob(waitCtx, jobID, "extract", primaryURL, req, true, 100, tenantID); err != nil {
		return nil, err
	}

	pollInterval := 100 * time.Millisecond
	lastStatus := ""

	for {
		select {
		case <-waitCtx.Done():
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				if lastStatus == "" || lastStatus == "pending" {
					return &ExtractResponse{
						Success: false,
						Code:    "JOB_NOT_STARTED",
						Error:   "extract job did not start before timeout",
					}, nil
				}
				return &ExtractResponse{
					Success: false,
					Code:    "EXTRACT_TIMEOUT",
					Error:   "extract job did not complete before timeout",
				}, nil
			}
			return nil, waitCtx.Err()
		case <-time.After(pollInterval):
		}

		job, err := e.st.GetJobByID(waitCtx, jobID)
		if err != nil {
			return nil, err
		}

		lastStatus = job.Status

		switch job.Status {
		case "pending", "running":
			continue
		case "completed":
			if !job.Output.Valid || len(job.Output.RawMessage) == 0 {
				return nil, fmt.Errorf("extract job %s completed with empty output", jobID.String())
			}

			var res ExtractResponse
			if err := json.Unmarshal(job.Output.RawMessage, &res); err != nil {
				return nil, err
			}
			return &res, nil
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

			return &ExtractResponse{
				Success: false,
				Code:    code,
				Error:   msg,
			}, nil
		default:
			return nil, fmt.Errorf("unexpected extract job status %q", job.Status)
		}
	}
}
