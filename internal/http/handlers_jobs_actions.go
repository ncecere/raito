package http

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"raito/internal/db"
	"raito/internal/store"
)

type JobDeleteResponse struct {
	Success bool   `json:"success"`
	Code    string `json:"code,omitempty"`
	Error   string `json:"error,omitempty"`
}

func jobDeleteHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)

	val := c.Locals("principal")
	p, ok := val.(Principal)
	if !ok || p.UserID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(JobDeleteResponse{
			Success: false,
			Code:    "UNAUTHENTICATED",
			Error:   "User context is not available for this request",
		})
	}

	rawID := c.Params("id")
	jobID, err := uuid.Parse(rawID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(JobDeleteResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "invalid job id",
		})
	}

	job, err := st.GetJobByID(c.Context(), jobID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(JobDeleteResponse{
			Success: false,
			Code:    "NOT_FOUND",
			Error:   "job not found",
		})
	}

	// Enforce tenant scoping for non-admin callers when the job has a tenant.
	if !p.IsSystemAdmin && job.TenantID.Valid {
		if p.TenantID == nil || job.TenantID.UUID != *p.TenantID {
			return c.Status(fiber.StatusNotFound).JSON(JobDeleteResponse{
				Success: false,
				Code:    "NOT_FOUND",
				Error:   "job not found",
			})
		}
	}

	deleted, err := st.DeleteJobByID(c.Context(), jobID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(JobDeleteResponse{
			Success: false,
			Code:    "JOB_DELETE_FAILED",
			Error:   err.Error(),
		})
	}
	if !deleted {
		return c.Status(fiber.StatusNotFound).JSON(JobDeleteResponse{
			Success: false,
			Code:    "NOT_FOUND",
			Error:   "job not found",
		})
	}

	return c.Status(fiber.StatusOK).JSON(JobDeleteResponse{Success: true})
}

func jobDownloadHandler(c *fiber.Ctx) error {
	st := c.Locals("store").(*store.Store)

	val := c.Locals("principal")
	p, ok := val.(Principal)
	if !ok || p.UserID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Success: false,
			Code:    "UNAUTHENTICATED",
			Error:   "User context is not available for this request",
		})
	}

	rawID := c.Params("id")
	jobID, err := uuid.Parse(rawID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Success: false,
			Code:    "BAD_REQUEST",
			Error:   "invalid job id",
		})
	}

	job, docs, err := st.GetCrawlJobAndDocuments(c.Context(), jobID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Success: false,
				Code:    "NOT_FOUND",
				Error:   "job not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "JOB_LOOKUP_FAILED",
			Error:   err.Error(),
		})
	}

	// Enforce tenant scoping for non-admin callers when the job has a tenant.
	if !p.IsSystemAdmin && job.TenantID.Valid {
		if p.TenantID == nil || job.TenantID.UUID != *p.TenantID {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Success: false,
				Code:    "NOT_FOUND",
				Error:   "job not found",
			})
		}
	}

	if job.Status != "completed" {
		return c.Status(fiber.StatusConflict).JSON(ErrorResponse{
			Success: false,
			Code:    "JOB_NOT_COMPLETED",
			Error:   "job is not completed yet",
		})
	}

	apiKeyLabel := ""
	if job.ApiKeyID.Valid {
		q := db.New(st.DB)
		rows, err := q.GetAPIKeyLabelsByIDs(c.Context(), []uuid.UUID{job.ApiKeyID.UUID})
		if err == nil && len(rows) > 0 {
			apiKeyLabel = rows[0].Label
		}
	}

	filenameBase := buildDownloadBaseName(job.Type, job.Url, job.CreatedAt, job.ID)

	switch job.Type {
	case "scrape":
		return sendScrapeDownload(c, filenameBase, job, docs, apiKeyLabel)
	case "batch_scrape", "batch":
		return sendDocumentsDownload(c, filenameBase, job, docs, true)
	default:
		// For crawl/map/extract (and anything else): zip when documents exist,
		// otherwise fall back to job output JSON if present.
		if len(docs) > 0 {
			return sendDocumentsDownload(c, filenameBase, job, docs, false)
		}
		if job.Output.Valid && len(job.Output.RawMessage) > 0 {
			return sendJSONDownload(c, filenameBase+".json", job.Output.RawMessage)
		}
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Success: false,
			Code:    "NO_DOWNLOAD_AVAILABLE",
			Error:   "no downloadable output is available for this job",
		})
	}
}

func sendJSONDownload(c *fiber.Ctx, filename string, raw json.RawMessage) error {
	c.Set(fiber.HeaderContentType, "application/json")
	c.Set(fiber.HeaderContentDisposition, contentDisposition(filename))
	return c.Send(raw)
}

func sendScrapeDownload(c *fiber.Ctx, filenameBase string, job db.Job, docs []db.Document, apiKeyLabel string) error {
	formats := scrapeFormatNamesFromJob(job)
	markdownOnly := len(formats) == 0 || (len(formats) == 1 && formats[0] == "markdown")

	var outputDoc *Document
	if job.Output.Valid && len(job.Output.RawMessage) > 0 {
		// Depending on the execution path, jobs.output may store either a full
		// ScrapeResponse envelope or the raw Document directly.
		var sr ScrapeResponse
		if err := json.Unmarshal(job.Output.RawMessage, &sr); err == nil && sr.Data != nil {
			outputDoc = sr.Data
		} else {
			var d Document
			if err := json.Unmarshal(job.Output.RawMessage, &d); err == nil {
				// Treat any non-empty document payload as a valid scrape output.
				if d.Markdown != "" || d.HTML != "" || d.RawHTML != "" || d.Summary != "" || len(d.JSON) > 0 || len(d.Branding) > 0 || d.Screenshot != "" {
					outputDoc = &d
				}
			}
		}
	}

	// Prefer a single markdown file when the job only requested markdown.
	if markdownOnly {
		markdown := ""
		if outputDoc != nil && outputDoc.Markdown != "" {
			markdown = outputDoc.Markdown
		} else if len(docs) > 0 && docs[0].Markdown.Valid {
			markdown = docs[0].Markdown.String
		}

		if markdown == "" {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Success: false,
				Code:    "NO_DOWNLOAD_AVAILABLE",
				Error:   "no markdown output is available for this job",
			})
		}

		filename := filenameBase + ".md"
		c.Set(fiber.HeaderContentType, "text/markdown; charset=utf-8")
		c.Set(fiber.HeaderContentDisposition, contentDisposition(filename))
		return c.SendString(markdown)
	}

	return sendScrapeZipDownload(c, filenameBase+".zip", job, docs, outputDoc, formats)
}

func sendDocumentsDownload(c *fiber.Ctx, filenameBase string, job db.Job, docs []db.Document, alwaysZip bool) error {
	formats := formatsFromJobInput(job.Type, job.Input)
	if len(formats) == 0 {
		formats = []string{"markdown"}
	}

	// If there's only a single document and only markdown was requested, prefer a single file.
	if !alwaysZip && len(docs) == 1 && len(formats) == 1 && formats[0] == "markdown" && docs[0].Markdown.Valid {
		filename := filenameBase + ".md"
		c.Set(fiber.HeaderContentType, "text/markdown; charset=utf-8")
		c.Set(fiber.HeaderContentDisposition, contentDisposition(filename))
		return c.SendString(docs[0].Markdown.String)
	}

	filename := filenameBase + ".zip"

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	wrote := false

	for i, doc := range docs {
		prefix := fmt.Sprintf("docs/%03d-%s", i+1, buildDocSlug(doc.Url))
		for _, f := range formats {
			switch strings.ToLower(f) {
			case "markdown":
				if doc.Markdown.Valid {
					_ = zipWriteFile(zw, prefix+".md", []byte(doc.Markdown.String))
					wrote = true
				}
			case "html":
				if doc.Html.Valid {
					_ = zipWriteFile(zw, prefix+".html", []byte(doc.Html.String))
					wrote = true
				}
			case "rawhtml":
				if doc.RawHtml.Valid {
					_ = zipWriteFile(zw, prefix+".raw.html", []byte(doc.RawHtml.String))
					wrote = true
				}
			default:
				// other formats aren't currently persisted per-document in the DB
			}
		}
	}

	if err := zw.Close(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "ZIP_BUILD_FAILED",
			Error:   err.Error(),
		})
	}

	if !wrote {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Success: false,
			Code:    "NO_DOWNLOAD_AVAILABLE",
			Error:   "no formatted output is available for this job",
		})
	}

	c.Set(fiber.HeaderContentType, "application/zip")
	c.Set(fiber.HeaderContentDisposition, contentDisposition(filename))
	return c.Send(buf.Bytes())
}

func sendScrapeZipDownload(c *fiber.Ctx, filename string, job db.Job, docs []db.Document, outputDoc *Document, formats []string) error {
	var doc Document
	if outputDoc != nil {
		doc = *outputDoc
	} else if len(docs) > 0 {
		if docs[0].Markdown.Valid {
			doc.Markdown = docs[0].Markdown.String
		}
		if docs[0].Html.Valid {
			doc.HTML = docs[0].Html.String
		}
		if docs[0].RawHtml.Valid {
			doc.RawHTML = docs[0].RawHtml.String
		}
		_ = json.Unmarshal(docs[0].Metadata, &doc.Metadata)
		if docs[0].Engine.Valid {
			doc.Engine = docs[0].Engine.String
		}
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	wroteFormatFile := false

	// Include metadata whenever available (it’s useful context and small).
	if metaJSON, err := json.MarshalIndent(doc.Metadata, "", "  "); err == nil && len(metaJSON) > 0 {
		_ = zipWriteFile(zw, "metadata.json", metaJSON)
	}

	// Include selected formats.
	for _, f := range formats {
		switch f {
		case "markdown":
			if doc.Markdown != "" {
				_ = zipWriteFile(zw, "scrape.md", []byte(doc.Markdown))
				wroteFormatFile = true
			}
		case "html":
			if doc.HTML != "" {
				_ = zipWriteFile(zw, "scrape.html", []byte(doc.HTML))
				wroteFormatFile = true
			}
		case "rawhtml":
			if doc.RawHTML != "" {
				_ = zipWriteFile(zw, "scrape.raw.html", []byte(doc.RawHTML))
				wroteFormatFile = true
			}
		case "links":
			if len(doc.Links) > 0 {
				b, _ := json.MarshalIndent(doc.Links, "", "  ")
				_ = zipWriteFile(zw, "links.json", b)
				wroteFormatFile = true
			}
		case "images":
			if len(doc.Images) > 0 {
				b, _ := json.MarshalIndent(doc.Images, "", "  ")
				_ = zipWriteFile(zw, "images.json", b)
				wroteFormatFile = true
			}
		case "summary":
			if doc.Summary != "" {
				_ = zipWriteFile(zw, "summary.txt", []byte(doc.Summary))
				wroteFormatFile = true
			}
		case "json":
			if len(doc.JSON) > 0 {
				b, _ := json.MarshalIndent(doc.JSON, "", "  ")
				_ = zipWriteFile(zw, "json.json", b)
				wroteFormatFile = true
			}
		case "branding":
			if len(doc.Branding) > 0 {
				b, _ := json.MarshalIndent(doc.Branding, "", "  ")
				_ = zipWriteFile(zw, "branding.json", b)
				wroteFormatFile = true
			}
		case "screenshot":
			if doc.Screenshot != "" {
				if raw, err := base64.StdEncoding.DecodeString(doc.Screenshot); err == nil && len(raw) > 0 {
					_ = zipWriteFile(zw, "screenshot.png", raw)
					wroteFormatFile = true
				}
			}
		default:
			// ignore unknown/unsupported formats for download
		}
	}

	if err := zw.Close(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Success: false,
			Code:    "ZIP_BUILD_FAILED",
			Error:   err.Error(),
		})
	}

	if !wroteFormatFile {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Success: false,
			Code:    "NO_DOWNLOAD_AVAILABLE",
			Error:   "no formatted output is available for this job",
		})
	}

	c.Set(fiber.HeaderContentType, "application/zip")
	c.Set(fiber.HeaderContentDisposition, contentDisposition(filename))
	return c.Send(buf.Bytes())
}

func zipWriteFile(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func scrapeFormatNamesFromJob(job db.Job) []string {
	var req ScrapeRequest
	if err := json.Unmarshal(job.Input, &req); err != nil {
		return nil
	}
	return scrapeFormatNames(req.Formats)
}

func scrapeFormatNames(formats []any) []string {
	out := make([]string, 0, len(formats))
	for _, f := range formats {
		switch v := f.(type) {
		case string:
			out = append(out, v)
		case map[string]any:
			if t, ok := v["type"].(string); ok {
				out = append(out, t)
			}
		default:
			// ignore
		}
	}

	uniq := make([]string, 0, len(out))
	seen := make(map[string]struct{}, len(out))
	for _, v := range out {
		v = strings.TrimSpace(strings.ToLower(v))
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		uniq = append(uniq, v)
	}
	return uniq
}

func buildDownloadBaseName(jobType, rawURL string, createdAt time.Time, id uuid.UUID) string {
	host, slug := extractHostAndSlug(rawURL)
	ts := createdAt.UTC().Format("20060102-150405")
	short := strings.Split(id.String(), "-")[0]

	parts := []string{"raito", jobType}
	if host != "" {
		parts = append(parts, host)
	}
	if slug != "" {
		parts = append(parts, slug)
	}
	parts = append(parts, ts, short)

	name := strings.Join(parts, "-")
	name = sanitizeFilename(name)
	if len(name) > 120 {
		name = name[:120]
	}
	return name
}

func extractHostAndSlug(rawURL string) (string, string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "job"
	}
	host := sanitizeFilename(u.Hostname())
	path := strings.Trim(u.Path, "/")
	if path == "" {
		return host, "root"
	}
	parts := strings.Split(path, "/")
	last := parts[len(parts)-1]
	if last == "" {
		last = "root"
	}
	return host, sanitizeFilename(last)
}

func buildDocSlug(rawURL string) string {
	host, slug := extractHostAndSlug(rawURL)
	if host == "" {
		return slug
	}
	if slug == "" {
		return host
	}
	return sanitizeFilename(host + "-" + slug)
}

var invalidFilenameChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitizeFilename(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "item"
	}
	v = invalidFilenameChars.ReplaceAllString(v, "-")
	v = strings.Trim(v, "-._")
	if v == "" {
		return "item"
	}
	return v
}

func contentDisposition(filename string) string {
	filename = strings.ReplaceAll(filename, `"`, "")
	return fmt.Sprintf(`attachment; filename="%s"`, filename)
}
