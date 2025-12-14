package http

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"raito/internal/services"
	"raito/internal/store"
)

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

	svc := services.NewBatchScrapeService(st)

	var tenantID *uuid.UUID
	if val := c.Locals("principal"); val != nil {
		if p, ok := val.(Principal); ok && p.TenantID != nil {
			tenantID = p.TenantID
		}
	}

	if err := svc.Enqueue(c.Context(), &services.BatchScrapeEnqueueRequest{
		ID:         id,
		PrimaryURL: primaryURL,
		Body:       reqBody,
		TenantID:   tenantID,
	}); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(BatchScrapeResponse{
			Success: false,
			Code:    "BATCH_SCRAPE_JOB_CREATE_FAILED",
			Error:   err.Error(),
		})
	}

	// Structured log event for batch scrape job enqueue.
	if loggerVal := c.Locals("logger"); loggerVal != nil {
		if lg, ok := loggerVal.(interface{ Info(msg string, args ...any) }); ok {
			attrs := []any{
				"batch_scrape_id", id.String(),
				"primary_url", primaryURL,
				"urls_count", len(reqBody.URLs),
				"has_formats", len(reqBody.Formats) > 0,
			}
			lg.Info("batch_scrape_enqueued", attrs...)
		}
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

	// Enforce tenant scoping for non-admin callers.
	if val := c.Locals("principal"); val != nil {
		if p, ok := val.(Principal); ok && !p.IsSystemAdmin && job.TenantID.Valid && p.TenantID != nil && job.TenantID.UUID != *p.TenantID {
			return c.Status(fiber.StatusNotFound).JSON(BatchScrapeResponse{
				Success: false,
				Code:    "NOT_FOUND",
				Error:   "batch scrape job not found",
			})
		}
	}

	resp := BatchScrapeResponse{
		Success: true,
		ID:      job.ID.String(),
		Status:  BatchScrapeStatus(job.Status),
		Total:   len(docs),
	}

	// Job-level logs for batch scrape completion/failure.
	if loggerVal := c.Locals("logger"); loggerVal != nil {
		if lg, ok := loggerVal.(interface{ Info(msg string, args ...any) }); ok {
			event := "batch_scrape_completed"
			if job.Status != "completed" {
				event = "batch_scrape_failed"
			}
			attrs := []any{
				"batch_scrape_id", job.ID.String(),
				"status", job.Status,
				"total_documents", len(docs),
			}
			if job.Error.Valid {
				attrs = append(attrs, "error", job.Error.String)
			}
			lg.Info(event, attrs...)
		}
	}

	// Map DB documents into API documents only when completed
	if job.Status == "completed" {
		// Decode the original batch request to determine requested formats.
		var originalReq BatchScrapeRequest
		_ = json.Unmarshal(job.Input, &originalReq)

		docSvc := services.NewJobDocumentService()
		mapped := docSvc.BuildDocuments(docs, services.JobDocumentFormatOptions{
			Formats:        originalReq.Formats,
			IncludeSummary: false,
			IncludeJSON:    false,
		})

		outDocs := make([]Document, 0, len(mapped))
		for _, d := range mapped {
			outDocs = append(outDocs, Document(d))
		}
		resp.Data = outDocs
	}

	if job.Error.Valid {
		resp.Error = job.Error.String
	}

	return c.Status(http.StatusOK).JSON(resp)
}
