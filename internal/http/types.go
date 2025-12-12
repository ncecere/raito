package http

import "raito/internal/model"

// Firecrawl v2-compatible types (subset)

// ScrapeFormat represents a simplified version of Firecrawl's FormatObject
// We focus on the common string formats for now (markdown, html, rawHtml, links, images, metadata).
type ScrapeFormat struct {
	Type string `json:"type"`
}

// ScrapeRequest mirrors the Firecrawl v2 scrapeRequest input shape
// but only includes the most relevant fields for Raito v1.
type ScrapeRequest struct {
	URL                 string            `json:"url"`
	Formats             []interface{}     `json:"formats,omitempty"`
	Headers             map[string]string `json:"headers,omitempty"`
	IncludeTags         []string          `json:"includeTags,omitempty"`
	ExcludeTags         []string          `json:"excludeTags,omitempty"`
	OnlyMainContent     *bool             `json:"onlyMainContent,omitempty"`
	Timeout             *int              `json:"timeout,omitempty"`
	WaitFor             *int              `json:"waitFor,omitempty"`
	Mobile              *bool             `json:"mobile,omitempty"`
	SkipTLSVerification *bool             `json:"skipTlsVerification,omitempty"`
	RemoveBase64Images  *bool             `json:"removeBase64Images,omitempty"`
	FastMode            *bool             `json:"fastMode,omitempty"`
	BlockAds            *bool             `json:"blockAds,omitempty"`
	Proxy               string            `json:"proxy,omitempty"`
	Origin              string            `json:"origin,omitempty"`
	ZeroDataRetention   *bool             `json:"zeroDataRetention,omitempty"`
	UseBrowser          *bool             `json:"useBrowser,omitempty"`
}

// Re-export shared types from the model package.
type Metadata = model.Metadata

type Document = model.Document

// ErrorResponse matches Firecrawl's error envelope shape.
type ErrorResponse struct {
	Success bool        `json:"success"`
	Code    string      `json:"code,omitempty"`
	Error   string      `json:"error"`
	Details interface{} `json:"details,omitempty"`
}

// ScrapeResponse matches Firecrawl v2's ScrapeResponse union shape.
type ScrapeResponse struct {
	Success  bool      `json:"success"`
	Warning  string    `json:"warning,omitempty"`
	Data     *Document `json:"data,omitempty"`
	ScrapeID string    `json:"scrape_id,omitempty"`
	Code     string    `json:"code,omitempty"`
	Error    string    `json:"error,omitempty"`
}

// MapRequest shape is based on Firecrawl's MapRequest.
type MapRequest struct {
	URL               string `json:"url"`
	Origin            string `json:"origin,omitempty"`
	Search            string `json:"search,omitempty"`
	IncludeSubdomains *bool  `json:"includeSubdomains,omitempty"`
	IgnoreQueryParams *bool  `json:"ignoreQueryParameters,omitempty"`
	AllowExternal     *bool  `json:"allowExternalLinks,omitempty"`
	Sitemap           string `json:"sitemap,omitempty"`
	Limit             *int   `json:"limit,omitempty"`
	Timeout           *int   `json:"timeout,omitempty"`
}

type MapLink struct {
	URL         string `json:"url"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

type MapResponse struct {
	Success bool      `json:"success"`
	Links   []MapLink `json:"links"`
	Warning string    `json:"warning,omitempty"`
	Code    string    `json:"code,omitempty"`
	Error   string    `json:"error,omitempty"`
}

// CrawlRequest is a simplified subset of Firecrawl's CrawlRequest.
type CrawlRequest struct {
	URL                string   `json:"url"`
	Origin             string   `json:"origin,omitempty"`
	IncludePaths       []string `json:"includePaths,omitempty"`
	ExcludePaths       []string `json:"excludePaths,omitempty"`
	Limit              *int     `json:"limit,omitempty"`
	MaxDiscoveryDepth  *int     `json:"maxDiscoveryDepth,omitempty"`
	AllowExternalLinks *bool    `json:"allowExternalLinks,omitempty"`
	AllowSubdomains    *bool    `json:"allowSubdomains,omitempty"`
	IgnoreRobotsTxt    *bool    `json:"ignoreRobotsTxt,omitempty"`
	Sitemap            string   `json:"sitemap,omitempty"`
	DeduplicateSimilar bool     `json:"deduplicateSimilarURLs,omitempty"`
	IgnoreQueryParams  *bool    `json:"ignoreQueryParameters,omitempty"`
	RegexOnFullURL     *bool    `json:"regexOnFullURL,omitempty"`
	Delay              *int     `json:"delay,omitempty"`
	Webhook            string   `json:"webhook,omitempty"`
}

type CrawlStatus string

// ExtractField describes a single field to be extracted
// from a scraped document.
type ExtractField struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"` // optional hint: string, number, boolean
}

// ExtractRequest defines the payload for POST /v1/extract.
// v1 focuses on a single URL plus a set of fields. Provider/model
// are optional and fall back to server configuration.
type ExtractRequest struct {
	URL      string         `json:"url"`
	Fields   []ExtractField `json:"fields"`
	Prompt   string         `json:"prompt,omitempty"`
	Provider string         `json:"provider,omitempty"` // openai, anthropic, google
	Model    string         `json:"model,omitempty"`
	Strict   bool           `json:"strict,omitempty"`
}

type ExtractResult struct {
	URL    string                 `json:"url"`
	Fields map[string]interface{} `json:"fields"`
	Raw    *Document              `json:"raw,omitempty"`
}

type ExtractResponse struct {
	Success bool            `json:"success"`
	Data    []ExtractResult `json:"data,omitempty"`
	Code    string          `json:"code,omitempty"`
	Error   string          `json:"error,omitempty"`
}

const (
	CrawlStatusPending   CrawlStatus = "pending"
	CrawlStatusRunning   CrawlStatus = "running"
	CrawlStatusCompleted CrawlStatus = "completed"
	CrawlStatusFailed    CrawlStatus = "failed"
)

type CrawlResponse struct {
	Success     bool        `json:"success"`
	ID          string      `json:"id,omitempty"`
	URL         string      `json:"url,omitempty"`
	Status      CrawlStatus `json:"status,omitempty"`
	Total       int         `json:"total,omitempty"`
	CreditsUsed int         `json:"creditsUsed,omitempty"`
	ExpiresAt   string      `json:"expiresAt,omitempty"`
	Data        []Document  `json:"data,omitempty"`
	Code        string      `json:"code,omitempty"`
	Error       string      `json:"error,omitempty"`
	Warning     string      `json:"warning,omitempty"`
}
