package scraper

import (
	"context"
	"net/url"
	"strings"
	"time"

	htmlmd "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/PuerkitoBio/goquery"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// RodScraper uses a real browser (via rod) to render JS-heavy pages
// before extracting HTML, markdown, links, and metadata.
type RodScraper struct {
	BrowserURL string
	Timeout    time.Duration
}

func NewRodScraper(browserURL string, timeout time.Duration) *RodScraper {
	return &RodScraper{BrowserURL: browserURL, Timeout: timeout}
}

func (r *RodScraper) Scrape(ctx context.Context, req Request) (*Result, error) {
	u, err := url.Parse(req.URL)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" {
		u.Scheme = "http"
	}

	// Prepare browser with context and timeout
	browser := rod.New().Context(ctx).Timeout(r.Timeout)
	if r.BrowserURL != "" {
		browser = browser.ControlURL(r.BrowserURL)
	}

	if err := browser.Connect(); err != nil {
		return nil, err
	}
	defer browser.MustClose()

	page, err := browser.Page(proto.TargetCreateTarget{URL: u.String()})
	if err != nil {
		return nil, err
	}
	defer page.MustClose()

	if err := page.WaitLoad(); err != nil {
		return nil, err
	}

	htmlStr, err := page.HTML()
	if err != nil {
		return nil, err
	}

	// First, attempt HTML -> Markdown conversion (CommonMark-enabled)
	converter := htmlmd.NewConverter(u.Hostname(), true, nil)
	markdown, mdErr := converter.ConvertString(htmlStr)

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		// If parsing fails, still return raw HTML and status, with best-effort markdown
		if mdErr != nil {
			markdown = ""
		}
		return &Result{
			URL:      u.String(),
			Markdown: markdown,
			HTML:     htmlStr,
			RawHTML:  htmlStr,
			Status:   200,
			Engine:   "browser",
			Metadata: map[string]interface{}{
				"statusCode": 200,
				"sourceURL":  u.String(),
			},
		}, nil
	}

	// Extract links
	links := make([]string, 0)
	doc.Find("a[href]").Each(func(_ int, sel *goquery.Selection) {
		if href, ok := sel.Attr("href"); ok {
			href = strings.TrimSpace(href)
			if href == "" || strings.HasPrefix(href, "#") {
				return
			}
			linkURL, err := url.Parse(href)
			if err != nil {
				return
			}
			if !linkURL.IsAbs() {
				linkURL = u.ResolveReference(linkURL)
			}
			if linkURL.Scheme != "http" && linkURL.Scheme != "https" {
				return
			}
			linkURL.Fragment = ""
			links = append(links, linkURL.String())
		}
	})

	// Fallback markdown if converter failed
	if mdErr != nil {
		markdown = doc.Text()
	}

	// Build richer metadata (same as HTTPScraper)
	title := strings.TrimSpace(doc.Find("title").First().Text())
	desc := doc.Find("meta[name=description]").AttrOr("content", "")
	keywords := doc.Find("meta[name=keywords]").AttrOr("content", "")
	robots := doc.Find("meta[name=robots]").AttrOr("content", "")
	lang, _ := doc.Find("html").First().Attr("lang")

	ogTitle := doc.Find("meta[property=og:title]").AttrOr("content", "")
	ogDesc := doc.Find("meta[property=og:description]").AttrOr("content", "")
	ogURL := doc.Find("meta[property=og:url]").AttrOr("content", "")
	ogImage := doc.Find("meta[property=og:image]").AttrOr("content", "")
	ogSiteName := doc.Find("meta[property=og:site_name]").AttrOr("content", "")

	canonical := doc.Find("link[rel=canonical]").AttrOr("href", "")
	sourceURL := u.String()
	if canonical != "" {
		if cu, err := url.Parse(canonical); err == nil {
			if cu.Scheme == "" {
				cu = u.ResolveReference(cu)
			}
			sourceURL = cu.String()
		}
	}

	metadata := map[string]interface{}{
		"title":         title,
		"description":   desc,
		"language":      lang,
		"keywords":      keywords,
		"robots":        robots,
		"ogTitle":       ogTitle,
		"ogDescription": ogDesc,
		"ogUrl":         ogURL,
		"ogImage":       ogImage,
		"ogSiteName":    ogSiteName,
		"statusCode":    200,
		"sourceURL":     sourceURL,
	}

	return &Result{
		URL:      u.String(),
		Markdown: markdown,
		HTML:     htmlStr,
		RawHTML:  htmlStr,
		Links:    links,
		Metadata: metadata,
		Status:   200,
		Engine:   "browser",
	}, nil
}
