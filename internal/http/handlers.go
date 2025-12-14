package http

import (
	"context"
	"strings"
	"time"

	"raito/internal/config"
	"raito/internal/scraper"
	"raito/internal/scrapeutil"
)

// scrapeURLForSearch scrapes a URL for the search endpoint, applying
// ScrapeOptions and returning a lightweight Document.
func scrapeURLForSearch(ctx context.Context, cfg *config.Config, url string, opts *ScrapeOptions, timeoutMs int) (*Document, error) {
	if timeoutMs <= 0 {
		timeoutMs = cfg.Scraper.TimeoutMs
	}
	if timeoutMs <= 0 {
		timeoutMs = 30000
	}

	var engine scraper.Scraper
	useBrowser := false
	if opts != nil && opts.UseBrowser != nil {
		useBrowser = *opts.UseBrowser
	}
	if useBrowser && cfg.Rod.Enabled {
		engine = scraper.NewRodScraper(time.Duration(timeoutMs) * time.Millisecond)
	} else {
		engine = scraper.NewHTTPScraper(time.Duration(timeoutMs) * time.Millisecond)
	}

	headers := map[string]string{}
	if opts != nil {
		for k, v := range opts.Headers {
			headers[k] = v
		}
		if opts.Location != nil {
			if len(opts.Location.Languages) > 0 {
				headers["Accept-Language"] = strings.Join(opts.Location.Languages, ",")
			} else if opts.Location.Country != "" {
				headers["Accept-Language"] = opts.Location.Country
			}
		}
	}

	sReq := scraper.Request{
		URL:       url,
		Headers:   headers,
		Timeout:   time.Duration(timeoutMs) * time.Millisecond,
		UserAgent: cfg.Scraper.UserAgent,
	}

	res, err := engine.Scrape(ctx, sReq)
	if err != nil {
		return nil, err
	}

	md := Metadata{
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
		links = scrapeutil.FilterLinks(links, res.URL, cfg.Scraper.LinksSameDomainOnly, cfg.Scraper.LinksMaxPerDocument)
	}

	linkSet := make(map[string]struct{}, len(links))
	for _, l := range links {
		linkSet[l] = struct{}{}
	}

	linkMetadata := make([]LinkMetadata, 0, len(links))
	for _, lm := range res.LinkMetadata {
		if _, ok := linkSet[lm.URL]; !ok {
			continue
		}
		linkMetadata = append(linkMetadata, LinkMetadata{
			URL:  lm.URL,
			Text: lm.Text,
			Rel:  lm.Rel,
		})
	}

	images := scraper.ExtractImages(res.HTML, res.URL)

	formats := []interface{}{}
	if opts != nil {
		formats = opts.Formats
	}
	hasFormats := len(formats) > 0

	// For /v1/search, keep documents lightweight by default:
	// - When no formats are provided, include only markdown + metadata.
	// - When formats are provided, honor them explicitly.
	includeMarkdown := true
	includeHTML := false
	includeRawHTML := false
	includeLinks := false
	includeImages := false

	if hasFormats {
		includeMarkdown = scrapeutil.WantsFormat(formats, "markdown")
		includeHTML = scrapeutil.WantsFormat(formats, "html")
		includeRawHTML = scrapeutil.WantsFormat(formats, "rawHtml")
		includeLinks = scrapeutil.WantsFormat(formats, "links")
		includeImages = scrapeutil.WantsFormat(formats, "images")
	}

	doc := &Document{
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

	return doc, nil
}

// getScreenshotFormatConfig scans formats for a screenshot entry and returns
// whether it was requested along with a fullPage flag. It supports both simple
// string formats ("screenshot") and object formats ({type: "screenshot", ...}).
func getScreenshotFormatConfig(formats []interface{}) (bool, bool) {
	if len(formats) == 0 {
		return false, false
	}

	for _, f := range formats {
		switch v := f.(type) {
		case string:
			if strings.ToLower(v) == "screenshot" {
				// Default to full-page screenshots for better parity with Firecrawl.
				return true, true
			}
		case map[string]interface{}:
			rawType, ok := v["type"].(string)
			if !ok || strings.ToLower(rawType) != "screenshot" {
				continue
			}

			fullPage := true
			if fp, ok := v["fullPage"].(bool); ok {
				fullPage = fp
			}

			return true, fullPage
		}
	}

	return false, false
}
