package services

import (
	"encoding/json"

	"raito/internal/db"
	"raito/internal/model"
	"raito/internal/scraper"
	"raito/internal/scrapeutil"
)

// JobDocumentFormatOptions controls how stored job documents are projected
// into API-facing Document values for status endpoints.
type JobDocumentFormatOptions struct {
	// Formats is the original formats array from the request (e.g. crawl or
	// batch scrape). When empty, markdown/html/rawHtml/images default to
	// being included, matching existing v1 behavior.
	Formats []interface{}

	// IncludeSummary and IncludeJSON indicate whether these fields are even
	// considered for this job type. For example, crawl jobs may surface
	// summary/json stored in metadata, while batch scrape currently does not.
	IncludeSummary bool
	IncludeJSON    bool
}

// JobDocumentService maps stored db.Document rows into model.Document
// values suitable for HTTP responses.
type JobDocumentService struct{}

func NewJobDocumentService() *JobDocumentService {
	return &JobDocumentService{}
}

// BuildDocuments converts stored documents into API documents, honoring
// the requested formats and job-type-specific options.
func (s *JobDocumentService) BuildDocuments(docs []db.Document, opts JobDocumentFormatOptions) []model.Document {
	formats := opts.Formats
	hasFormats := len(formats) > 0

	includeMarkdown := !hasFormats || scrapeutil.WantsFormat(formats, "markdown")
	includeHTML := !hasFormats || scrapeutil.WantsFormat(formats, "html")
	includeRawHTML := !hasFormats || scrapeutil.WantsFormat(formats, "rawHtml")
	includeImages := !hasFormats || scrapeutil.WantsFormat(formats, "images")

	includeSummary := false
	includeJSON := false

	if opts.IncludeSummary {
		includeSummary = scrapeutil.WantsFormat(formats, "summary")
	}
	if opts.IncludeJSON {
		includeJSON = scrapeutil.WantsFormat(formats, "json")
	}

	out := make([]model.Document, 0, len(docs))
	for _, d := range docs {
		var md model.Metadata
		if err := json.Unmarshal(d.Metadata, &md); err != nil {
			// Skip documents with invalid metadata payloads.
			continue
		}

		var markdown, html, raw, engine string
		if d.Markdown.Valid {
			markdown = d.Markdown.String
		}
		if d.Html.Valid {
			html = d.Html.String
		}
		if d.RawHtml.Valid {
			raw = d.RawHtml.String
		}
		if d.Engine.Valid {
			engine = d.Engine.String
		}
		if engine == "" {
			engine = "http"
		}

		images := scraper.ExtractImages(html, md.SourceURL)
		if len(images) == 0 && raw != "" {
			images = scraper.ExtractImages(raw, md.SourceURL)
		}

		doc := model.Document{
			Engine:   engine,
			Metadata: md,
		}

		if includeMarkdown {
			doc.Markdown = markdown
		}
		if includeHTML {
			doc.HTML = html
		}
		if includeRawHTML {
			doc.RawHTML = raw
		}
		if includeImages {
			doc.Images = images
		}
		if includeSummary && md.Summary != "" {
			doc.Summary = md.Summary
		}
		if includeJSON && md.JSON != nil {
			doc.JSON = md.JSON
		}

		out = append(out, doc)
	}

	return out
}
