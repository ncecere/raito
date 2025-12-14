package http

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"raito/internal/services"
	"raito/internal/store"
)

// extractHandler implements a minimal POST /v1/extract endpoint that:
// - Enqueues an async extract job
func extractHandler(c *fiber.Ctx) error {
	var reqBody ExtractRequest
	if err := c.BodyParser(&reqBody); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ExtractResponse{
			Success: false,
			Code:    "BAD_REQUEST_INVALID_JSON",
			Error:   "Bad request, malformed JSON",
		})
	}

	// Require at least one URL via urls
	urls := reqBody.URLs
	if len(urls) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ExtractResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "Missing required field 'urls'",
		})
	}

	// Normalize and validate URLs early to avoid enqueuing obviously
	// invalid work. Only http/https URLs with a host are accepted.
	for i, raw := range urls {
		u := strings.TrimSpace(raw)
		if u == "" {
			return c.Status(fiber.StatusBadRequest).JSON(ExtractResponse{
				Success: false,
				Code:    "BAD_REQUEST_INVALID_URL",
				Error:   fmt.Sprintf("Invalid URL at index %d", i),
			})
		}

		parsed, err := url.Parse(u)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return c.Status(fiber.StatusBadRequest).JSON(ExtractResponse{
				Success: false,
				Code:    "BAD_REQUEST_INVALID_URL",
				Error:   fmt.Sprintf("Invalid URL at index %d", i),
			})
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return c.Status(fiber.StatusBadRequest).JSON(ExtractResponse{
				Success: false,
				Code:    "BAD_REQUEST_INVALID_URL",
				Error:   fmt.Sprintf("Unsupported URL scheme at index %d", i),
			})
		}

		urls[i] = u
	}
	reqBody.URLs = urls

	// Require a JSON schema; legacy fields mode is no longer supported.
	if len(reqBody.Schema) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ExtractResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "Missing required field 'schema'",
		})
	}

	if code, msg := validateExtractSchema(reqBody.Schema); code != "" {
		return c.Status(fiber.StatusBadRequest).JSON(ExtractResponse{
			Success: false,
			Code:    code,
			Error:   msg,
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

	svc := services.NewExtractService(st)

	var tenantID *uuid.UUID
	if val := c.Locals("principal"); val != nil {
		if p, ok := val.(Principal); ok && p.TenantID != nil {
			tenantID = p.TenantID
		}
	}

	if err := svc.Enqueue(c.Context(), &services.ExtractRequest{
		ID:         id,
		Body:       reqBody,
		PrimaryURL: primaryURL,
		TenantID:   tenantID,
	}); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(ExtractResponse{
			Success: false,
			Code:    "EXTRACT_JOB_CREATE_FAILED",
			Error:   err.Error(),
		})
	}

	if loggerVal := c.Locals("logger"); loggerVal != nil {
		if lg, ok := loggerVal.(interface{ Info(msg string, args ...any) }); ok {
			attrs := []any{
				"extract_id", id.String(),
				"primary_url", primaryURL,
				"urls_count", len(urls),
				"provider", reqBody.Provider,
				"model", reqBody.Model,
				"ignore_invalid_urls", reqBody.IgnoreInvalidURLs,
				"show_sources", reqBody.ShowSources,
			}
			lg.Info("extract_enqueued", attrs...)
		}
	}

	protocol := c.Protocol()
	host := c.Hostname()

	return c.Status(http.StatusOK).JSON(fiber.Map{
		"success": true,
		"id":      id.String(),
		"url":     protocol + "://" + host + "/v1/extract/" + id.String(),
	})
}

func validateExtractSchema(schema map[string]interface{}) (string, string) {
	if len(schema) == 0 {
		return "INVALID_SCHEMA", "Schema must be a non-empty JSON object"
	}

	// Guard against excessively large or complex top-level schemas. This is a
	// coarse limit on the number of top-level keys; nested complexity is still
	// allowed but bounded at the root.
	const maxTopLevelKeys = 256
	if len(schema) > maxTopLevelKeys {
		return "SCHEMA_TOO_COMPLEX", fmt.Sprintf("Schema has too many top-level keys (max %d)", maxTopLevelKeys)
	}

	if t, ok := schema["type"]; ok {
		if ts, ok := t.(string); ok && ts != "" {
			// Today we only meaningfully support object/array-shaped outputs at
			// the top level. Reject clearly incompatible primitives while
			// allowing object/array and omitting "type" entirely.
			switch ts {
			case "object", "array":
				// allowed
			default:
				return "INVALID_SCHEMA", "Schema 'type' must be 'object' or 'array' when provided"
			}
		}
	}

	return "", ""
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

	// Enforce tenant scoping: if the job is associated with a tenant,
	// non-admin callers must match that tenant.
	if val := c.Locals("principal"); val != nil {
		if p, ok := val.(Principal); ok && !p.IsSystemAdmin && job.TenantID.Valid && p.TenantID != nil && job.TenantID.UUID != *p.TenantID {
			return c.Status(fiber.StatusNotFound).JSON(ExtractStatusResponse{
				Success: false,
				Status:  ExtractStatusFailed,
				Code:    "NOT_FOUND",
				Error:   "extract job not found",
			})
		}
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
