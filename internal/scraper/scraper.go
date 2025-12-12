package scraper

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	htmlmd "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/PuerkitoBio/goquery"
)

// Request represents a simplified scrape request used by the scraper package.
type Request struct {
	URL       string
	Headers   map[string]string
	Timeout   time.Duration
	UserAgent string
}

// Result represents the core scrape output independent of the HTTP layer.
type Result struct {
	URL      string
	Markdown string
	HTML     string
	RawHTML  string
	Links    []string
	Metadata map[string]interface{}
	Status   int
	Engine   string
}

// Scraper defines the interface for URL scrapers.
type Scraper interface {
	Scrape(ctx context.Context, req Request) (*Result, error)
}

// HTTPScraper is a basic implementation using net/http and goquery.
type HTTPScraper struct {
	client *http.Client
}

func NewHTTPScraper(timeout time.Duration) *HTTPScraper {
	return &HTTPScraper{
		client: &http.Client{Timeout: timeout},
	}
}

func (s *HTTPScraper) Scrape(ctx context.Context, req Request) (*Result, error) {
	u, err := url.Parse(req.URL)
	if err != nil {
		return nil, err
	}

	if u.Scheme == "" {
		u.Scheme = "http"
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	if req.UserAgent != "" {
		httpReq.Header.Set("User-Agent", req.UserAgent)
	}

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	htmlStr := string(bodyBytes)

	// First, attempt HTML -> Markdown conversion (CommonMark-enabled)
	converter := htmlmd.NewConverter(u.Hostname(), true, nil)
	markdown, mdErr := converter.ConvertString(htmlStr)

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(bodyBytes))
	if err != nil {
		// If parsing fails, still return raw HTML, status, and best-effort markdown
		if mdErr != nil {
			markdown = ""
		}
		return &Result{
			URL:      u.String(),
			Markdown: markdown,
			HTML:     htmlStr,
			RawHTML:  htmlStr,
			Status:   resp.StatusCode,
			Engine:   "http",
			Metadata: map[string]interface{}{
				"statusCode": resp.StatusCode,
				"sourceURL":  u.String(),
			},
		}, nil
	}

	// Extract links and fallback plain-text markdown if converter failed
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

	if mdErr != nil {
		markdown = doc.Text()
	}

	// Build richer metadata
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

	// Canonical URL
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
		"statusCode":    resp.StatusCode,
		"sourceURL":     sourceURL,
	}

	return &Result{
		URL:      u.String(),
		Markdown: markdown,
		HTML:     htmlStr,
		RawHTML:  htmlStr,
		Links:    links,
		Metadata: metadata,
		Status:   resp.StatusCode,
		Engine:   "http",
	}, nil
}
