package http

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"raito/internal/config"
	"raito/internal/services"
	"raito/internal/store"
)

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

	svc := services.NewCrawlService(st)
	if err := svc.Enqueue(c.Context(), &services.CrawlEnqueueRequest{
		ID:   id,
		URL:  reqBody.URL,
		Body: reqBody,
	}); err != nil {
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

		docSvc := services.NewJobDocumentService()
		mapped := docSvc.BuildDocuments(docs, services.JobDocumentFormatOptions{
			Formats:        originalReq.Formats,
			IncludeSummary: true,
			IncludeJSON:    true,
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
