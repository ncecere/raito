package services

import (
	"context"

	"raito/internal/config"
	"raito/internal/model"
	"raito/internal/scraper"
	"raito/internal/scrapeutil"
)

// ScrapeRequest is the internal representation of a scrape request
// used by ScrapeService. For now it focuses on transforming a
// scraper.Result into a Document while respecting Firecrawl-style
// formats.
type ScrapeRequest struct {
	Result  *scraper.Result
	Formats []interface{}
}

// ScrapeResult wraps the constructed Document so the service API
// remains extensible as we add more fields later.
type ScrapeResult struct {
	Document *model.Document
}

// ScrapeService encapsulates the core, non-HTTP scraping logic: it
// takes a low-level scraper.Result and produces a Firecrawl-like
// Document with metadata, links, and images filled in.
type ScrapeService interface {
	Scrape(ctx context.Context, req *ScrapeRequest) (*ScrapeResult, error)
}

type scrapeService struct {
	cfg *config.Config
}

// NewScrapeService constructs a ScrapeService backed by the provided
// configuration. The current implementation does not perform any
// network I/O; it only transforms an existing scraper.Result.
func NewScrapeService(cfg *config.Config) ScrapeService {
	return &scrapeService{cfg: cfg}
}

func (s *scrapeService) Scrape(_ context.Context, req *ScrapeRequest) (*ScrapeResult, error) {
	// For now the context is unused since this service does not perform
	// its own I/O, but we keep it in the signature for future use.
	if req == nil || req.Result == nil {
		return &ScrapeResult{Document: &model.Document{}}, nil
	}

	res := req.Result
	formats := req.Formats
	hasFormats := len(formats) > 0

	md := model.Metadata{
		Title:         scrapeutil.ToString(res.Metadata["title"]),
		Description:   scrapeutil.ToString(res.Metadata["description"]),
		Language:      scrapeutil.ToString(res.Metadata["language"]),
		Keywords:      scrapeutil.ToString(res.Metadata["keywords"]),
		Robots:        scrapeutil.ToString(res.Metadata["robots"]),
		OgTitle:       scrapeutil.ToString(res.Metadata["ogTitle"]),
		OgDescription: scrapeutil.ToString(res.Metadata["ogDescription"]),
		OgURL:         scrapeutil.ToString(res.Metadata["ogUrl"]),
		OgImage:       scrapeutil.ToString(res.Metadata["ogImage"]),
		OgSiteName:    scrapeutil.ToString(res.Metadata["ogSiteName"]),
		SourceURL:     scrapeutil.ToString(res.Metadata["sourceURL"]),
		StatusCode:    res.Status,
	}

	links := res.Links
	if len(links) > 0 {
		links = scrapeutil.FilterLinks(links, res.URL, s.cfg.Scraper.LinksSameDomainOnly, s.cfg.Scraper.LinksMaxPerDocument)
	}

	linkSet := make(map[string]struct{}, len(links))
	for _, l := range links {
		linkSet[l] = struct{}{}
	}

	linkMetadata := make([]model.LinkMetadata, 0, len(links))
	for _, lm := range res.LinkMetadata {
		if _, ok := linkSet[lm.URL]; !ok {
			continue
		}
		linkMetadata = append(linkMetadata, model.LinkMetadata{
			URL:  lm.URL,
			Text: lm.Text,
			Rel:  lm.Rel,
		})
	}

	images := scraper.ExtractImages(res.HTML, res.URL)

	includeMarkdown := !hasFormats || scrapeutil.WantsFormat(formats, "markdown")
	includeHTML := !hasFormats || scrapeutil.WantsFormat(formats, "html")
	includeRawHTML := !hasFormats || scrapeutil.WantsFormat(formats, "rawHtml")
	includeLinks := !hasFormats || scrapeutil.WantsFormat(formats, "links")
	includeImages := !hasFormats || scrapeutil.WantsFormat(formats, "images")

	doc := &model.Document{
		Engine:   res.Engine,
		Metadata: md,
	}

	if includeMarkdown {
		doc.Markdown = res.Markdown
	}
	if includeHTML {
		doc.HTML = res.HTML
	}
	if includeRawHTML {
		doc.RawHTML = res.RawHTML
	}
	if includeLinks {
		doc.Links = links
		doc.LinkMetadata = linkMetadata
	}
	if includeImages {
		doc.Images = images
	}

	return &ScrapeResult{Document: doc}, nil
}
