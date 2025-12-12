package crawler

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	robotstxt "github.com/temoto/robotstxt"
)

// MapOptions controls how the map operation discovers URLs for a site.
type MapOptions struct {
	URL               string
	Limit             int
	Search            string
	IncludeSubdomains bool
	IgnoreQueryParams bool
	AllowExternal     bool
	SitemapMode       string // "only", "include", "skip"
	Timeout           time.Duration
	RespectRobots     bool
	UserAgent         string
}

// Link represents a discovered URL with optional metadata.
type Link struct {
	URL         string
	Title       string
	Description string
}

// MapResult is the result of a map operation.
type MapResult struct {
	Links   []Link
	Warning string
}

// Map discovers URLs for the given site based on the provided options.
func Map(ctx context.Context, opts MapOptions) (*MapResult, error) {
	if opts.URL == "" {
		return nil, errors.New("url is required")
	}

	baseURL, err := url.Parse(opts.URL)
	if err != nil {
		return nil, err
	}
	if baseURL.Scheme == "" {
		baseURL.Scheme = "http"
	}

	client := &http.Client{Timeout: opts.Timeout}

	var robotsData *robotstxt.RobotsData
	if opts.RespectRobots {
		robotsData, _ = fetchRobots(ctx, client, baseURL, opts.UserAgent)
	}

	linksSet := make(map[string]Link)

	// Helper to add a URL if it passes filters.
	addLink := func(uStr, title, desc string) {
		if len(linksSet) >= opts.Limit {
			return
		}

		u, err := baseURL.Parse(uStr)
		if err != nil {
			return
		}

		// Enforce same host / subdomain rules unless external links are allowed.
		if !opts.AllowExternal && !sameHostOrSubdomain(baseURL.Hostname(), u.Hostname(), opts.IncludeSubdomains) {
			return
		}

		if opts.IgnoreQueryParams {
			u.RawQuery = ""
		}

		// Respect robots.txt if available.
		if robotsData != nil {
			grp := robotsData.FindGroup(opts.UserAgent)
			if !grp.Test(u.String()) {
				return
			}
		}

		finalURL := u.String()

		// Search filter
		if opts.Search != "" {
			needle := strings.ToLower(opts.Search)
			if !strings.Contains(strings.ToLower(finalURL), needle) &&
				!strings.Contains(strings.ToLower(title), needle) {
				return
			}
		}

		if _, exists := linksSet[finalURL]; exists {
			return
		}

		linksSet[finalURL] = Link{
			URL:         finalURL,
			Title:       strings.TrimSpace(title),
			Description: strings.TrimSpace(desc),
		}
	}

	// Sitemap discovery
	if opts.SitemapMode == "only" || opts.SitemapMode == "include" || opts.SitemapMode == "" {
		if err := collectFromSitemap(ctx, client, baseURL, addLink); err != nil {
			// Non-fatal; we still try HTML discovery
		}
	}

	// HTML discovery from root page
	if opts.SitemapMode == "include" || opts.SitemapMode == "skip" || opts.SitemapMode == "" {
		if err := collectFromHTML(ctx, client, baseURL, addLink); err != nil {
			// Non-fatal
		}
	}

	links := make([]Link, 0, len(linksSet))
	for _, l := range linksSet {
		links = append(links, l)
	}

	warning := ""
	if len(links) <= 1 && opts.Limit != 1 {
		// If user mapped a deep path and got few results, suggest the base domain
		if baseURL.Path != "" && baseURL.Path != "/" {
			root := &url.URL{Scheme: baseURL.Scheme, Host: baseURL.Host}
			warning = "Only " + strconv.Itoa(len(links)) + " result(s) found. For broader coverage, try mapping the base domain: " + root.String()
		}
	}

	return &MapResult{Links: links, Warning: warning}, nil
}

func sameHostOrSubdomain(baseHost, host string, includeSubdomains bool) bool {
	if host == "" {
		return false
	}
	if strings.EqualFold(baseHost, host) {
		return true
	}
	if includeSubdomains {
		// e.g. baseHost=example.com, host=foo.example.com
		if strings.HasSuffix(strings.ToLower(host), "."+strings.ToLower(baseHost)) {
			return true
		}
	}
	return false
}

// fetchRobots fetches and parses robots.txt for a given base URL.
func fetchRobots(ctx context.Context, client *http.Client, base *url.URL, userAgent string) (*robotstxt.RobotsData, error) {
	robotsURL := &url.URL{
		Scheme: base.Scheme,
		Host:   base.Host,
		Path:   "/robots.txt",
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL.String(), nil)
	if err != nil {
		return nil, err
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("non-200 robots.txt")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return robotstxt.FromStatusAndBytes(resp.StatusCode, body)
}

// collectFromSitemap tries the conventional /sitemap.xml location and collects URLs.
func collectFromSitemap(ctx context.Context, client *http.Client, base *url.URL, add func(url, title, desc string)) error {
	sitemapURL := &url.URL{
		Scheme: base.Scheme,
		Host:   base.Host,
		Path:   "/sitemap.xml",
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sitemapURL.String(), nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("non-200 sitemap")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Basic urlset sitemap
	type urlEntry struct {
		Loc string `xml:"loc"`
	}
	type urlSet struct {
		URLs []urlEntry `xml:"url"`
	}

	var us urlSet
	if err := xml.Unmarshal(body, &us); err != nil {
		return err
	}

	for _, ue := range us.URLs {
		add(ue.Loc, "", "")
	}

	return nil
}

// collectFromHTML fetches the base URL HTML and extracts links from anchor tags.
func collectFromHTML(ctx context.Context, client *http.Client, base *url.URL, add func(url, title, desc string)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("non-200 html")
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return err
	}

	doc.Find("a[href]").Each(func(_ int, sel *goquery.Selection) {
		href, ok := sel.Attr("href")
		if !ok {
			return
		}
		title := strings.TrimSpace(sel.Text())
		add(href, title, "")
	})

	return nil
}
