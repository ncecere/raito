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
	UseBrowser          *bool             `json:"useBrowser,omitempty"`

	// Advanced scrape options (Phase 10)
	Location    *LocationOptions `json:"location,omitempty"`
	Integration string           `json:"integration,omitempty"`
}

// LocationOptions describes geo-related options for scraping.
type LocationOptions struct {
	Country   string   `json:"country,omitempty"`
	Languages []string `json:"languages,omitempty"`
}

// Re-export shared types from the model package.
type Metadata = model.Metadata

type Document = model.Document

type LinkMetadata = model.LinkMetadata

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
// For now, formats are provided at the top level and control which
// fields are included in crawl documents when retrieved.
type CrawlRequest struct {
	URL                string        `json:"url"`
	Origin             string        `json:"origin,omitempty"`
	IncludePaths       []string      `json:"includePaths,omitempty"`
	ExcludePaths       []string      `json:"excludePaths,omitempty"`
	Limit              *int          `json:"limit,omitempty"`
	MaxDiscoveryDepth  *int          `json:"maxDiscoveryDepth,omitempty"`
	AllowExternalLinks *bool         `json:"allowExternalLinks,omitempty"`
	AllowSubdomains    *bool         `json:"allowSubdomains,omitempty"`
	IgnoreRobotsTxt    *bool         `json:"ignoreRobotsTxt,omitempty"`
	Sitemap            string        `json:"sitemap,omitempty"`
	DeduplicateSimilar bool          `json:"deduplicateSimilarURLs,omitempty"`
	IgnoreQueryParams  *bool         `json:"ignoreQueryParameters,omitempty"`
	RegexOnFullURL     *bool         `json:"regexOnFullURL,omitempty"`
	Delay              *int          `json:"delay,omitempty"`
	Webhook            string        `json:"webhook,omitempty"`
	Formats            []interface{} `json:"formats,omitempty"`

	// Advanced crawl options (Phase 10)
	CrawlEntireDomain *bool          `json:"crawlEntireDomain,omitempty"`
	MaxConcurrency    *int           `json:"maxConcurrency,omitempty"`
	ScrapeOptions     *ScrapeOptions `json:"scrapeOptions,omitempty"`
}

// ScrapeOptions captures per-page scrape configuration that can be
// passed through from crawl-level options.
type ScrapeOptions struct {
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
	UseBrowser          *bool             `json:"useBrowser,omitempty"`
	Location            *LocationOptions  `json:"location,omitempty"`
	Integration         string            `json:"integration,omitempty"`

	MaxAge  *int64   `json:"maxAge,omitempty"`
	Parsers []string `json:"parsers,omitempty"`
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
// v2 focuses on a list of URLs plus a JSON schema. Provider/model
// are optional and fall back to server configuration.
//
// Legacy `url` and `fields` modes have been removed from the public
// API; requests must provide `urls` and a `schema`.
type ExtractRequest struct {
	URLs               []string               `json:"urls"`
	Schema             map[string]interface{} `json:"schema,omitempty"`
	Prompt             string                 `json:"prompt,omitempty"`
	SystemPrompt       string                 `json:"systemPrompt,omitempty"`
	Provider           string                 `json:"provider,omitempty"` // openai, anthropic, google
	Model              string                 `json:"model,omitempty"`
	Strict             bool                   `json:"strict,omitempty"`
	IgnoreInvalidURLs  *bool                  `json:"ignoreInvalidURLs,omitempty"`
	EnableWebSearch    *bool                  `json:"enableWebSearch,omitempty"`
	AllowExternalLinks *bool                  `json:"allowExternalLinks,omitempty"`
	ShowSources        *bool                  `json:"showSources,omitempty"`
	ScrapeOptions      *ScrapeOptions         `json:"scrapeOptions,omitempty"`
	Integration        string                 `json:"integration,omitempty"`
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

type ExtractJobStatus string

const (
	ExtractStatusPending   ExtractJobStatus = "pending"
	ExtractStatusRunning   ExtractJobStatus = "running"
	ExtractStatusCompleted ExtractJobStatus = "completed"
	ExtractStatusFailed    ExtractJobStatus = "failed"
)

type ExtractStatusResponse struct {
	Success     bool                   `json:"success"`
	Data        map[string]interface{} `json:"data,omitempty"`
	Status      ExtractJobStatus       `json:"status"`
	ExpiresAt   string                 `json:"expiresAt,omitempty"`
	TokensUsed  int                    `json:"tokensUsed,omitempty"`
	CreditsUsed int                    `json:"creditsUsed,omitempty"`
	Code        string                 `json:"code,omitempty"`
	Error       string                 `json:"error,omitempty"`
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

type BatchScrapeRequest struct {
	URLs    []string      `json:"urls"`
	Formats []interface{} `json:"formats,omitempty"`
}

type BatchScrapeStatus string

const (
	BatchStatusPending   BatchScrapeStatus = "pending"
	BatchStatusRunning   BatchScrapeStatus = "running"
	BatchStatusCompleted BatchScrapeStatus = "completed"
	BatchStatusFailed    BatchScrapeStatus = "failed"
)

type BatchScrapeResponse struct {
	Success bool              `json:"success"`
	ID      string            `json:"id,omitempty"`
	URL     string            `json:"url,omitempty"`
	Status  BatchScrapeStatus `json:"status,omitempty"`
	Total   int               `json:"total,omitempty"`
	Data    []Document        `json:"data,omitempty"`
	Code    string            `json:"code,omitempty"`
	Error   string            `json:"error,omitempty"`
	Warning string            `json:"warning,omitempty"`
}

// SearchRequest defines the payload for POST /v1/search.
// It mirrors a subset of Firecrawl's search options while
// remaining forward-compatible with additional sources/categories.
type SearchRequest struct {
	Query             string         `json:"query"`
	Sources           []string       `json:"sources,omitempty"`
	Categories        []string       `json:"categories,omitempty"`
	Limit             *int           `json:"limit,omitempty"`
	Country           string         `json:"country,omitempty"`
	Location          string         `json:"location,omitempty"`
	TBS               string         `json:"tbs,omitempty"`
	Timeout           *int           `json:"timeout,omitempty"`
	IgnoreInvalidURLs *bool          `json:"ignoreInvalidURLs,omitempty"`
	ScrapeOptions     *ScrapeOptions `json:"scrapeOptions,omitempty"`
	Integration       string         `json:"integration,omitempty"`
}

// SearchWebResult represents a single web search result which may
// optionally include a scraped Document when scrapeOptions are used.
type SearchWebResult struct {
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	URL         string    `json:"url"`
	Document    *Document `json:"document,omitempty"`

	// Lightweight metadata about the scraped page is
	// exposed at the top level for convenience.
	Metadata Metadata `json:"metadata,omitempty"`
	Engine   string   `json:"engine,omitempty"`
}

// SearchData groups results per source type. v1 only populates
// the Web slice; News and Images are reserved for future use.
type SearchData struct {
	Web    []SearchWebResult `json:"web,omitempty"`
	News   []SearchWebResult `json:"news,omitempty"`
	Images []SearchWebResult `json:"images,omitempty"`
}

// SearchResponse wraps search results in a Firecrawl-like envelope.
type SearchResponse struct {
	Success bool        `json:"success"`
	Data    *SearchData `json:"data,omitempty"`
	Code    string      `json:"code,omitempty"`
	Error   string      `json:"error,omitempty"`
	Warning string      `json:"warning,omitempty"`
}
