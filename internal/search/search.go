package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"raito/internal/config"
)

// Request represents a provider-agnostic search request.
type Request struct {
	Query            string
	Sources          []string
	Limit            int
	Country          string
	Location         string
	TBS              string
	Timeout          time.Duration
	IgnoreInvalidURL bool
}

// Result represents a single search hit from a provider.
type Result struct {
	Title       string
	Description string
	URL         string
}

// Results groups provider results per logical source.
type Results struct {
	Web    []Result
	News   []Result
	Images []Result
}

// Provider defines the contract for pluggable search providers.
// Implementations are responsible for mapping a provider-agnostic
// Request into provider-specific API calls and normalizing results
// back into the shared Results shape. Providers should:
//   - respect the Limit and Timeout fields where possible,
//   - treat IgnoreInvalidURL as a hint for filtering malformed URLs,
//   - avoid returning sensitive configuration details in errors.
type Provider interface {
	Search(ctx context.Context, req *Request) (*Results, error)
}

// NewProviderFromConfig constructs a search Provider based on configuration.
// Today this supports only a SearxNG-backed provider, but the Provider
// interface is intentionally narrow so additional providers (e.g. direct
// web search APIs) can be added without touching callers.
func NewProviderFromConfig(cfg *config.Config) (Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil config")
	}
	if !cfg.Search.Enabled {
		return nil, fmt.Errorf("search disabled in configuration")
	}

	providerName := strings.ToLower(strings.TrimSpace(cfg.Search.Provider))
	if providerName == "" {
		providerName = "searxng"
	}

	switch providerName {
	case "searxng":
		return NewSearxngProvider(cfg.Search)
	default:
		return nil, fmt.Errorf("unsupported search provider: %s", providerName)
	}
}

// SearxngProvider implements Provider using a SearxNG instance with JSON API enabled.
type SearxngProvider struct {
	baseURL      string
	client       *http.Client
	defaultLimit int
	timeout      time.Duration
}

// NewSearxngProvider creates a new SearxngProvider from SearchConfig.
func NewSearxngProvider(cfg config.SearchConfig) (*SearxngProvider, error) {
	base := strings.TrimRight(cfg.Searxng.BaseURL, "/")
	if base == "" {
		return nil, fmt.Errorf("searxng.baseURL is required when search is enabled")
	}

	// Prefer provider-specific timeout, then generic search timeout, with a
	// conservative fallback.
	timeoutMs := cfg.Searxng.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = cfg.TimeoutMs
	}
	if timeoutMs <= 0 {
		timeoutMs = 10000
	}

	defaultLimit := cfg.Searxng.DefaultLimit
	if defaultLimit <= 0 {
		defaultLimit = 5
	}

	return &SearxngProvider{
		baseURL:      base,
		client:       &http.Client{Timeout: time.Duration(timeoutMs) * time.Millisecond},
		defaultLimit: defaultLimit,
		timeout:      time.Duration(timeoutMs) * time.Millisecond,
	}, nil
}

// searxngResponse models only the subset of the SearxNG JSON response
// that we care about for basic web search.
type searxngResponse struct {
	Results []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Content string `json:"content"`
	} `json:"results"`
}

// Search executes a search query against the configured SearxNG instance.
func (p *SearxngProvider) Search(ctx context.Context, req *Request) (*Results, error) {
	if req == nil {
		return nil, fmt.Errorf("nil search request")
	}
	if strings.TrimSpace(req.Query) == "" {
		return nil, fmt.Errorf("empty search query")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = p.defaultLimit
	}
	if limit <= 0 {
		limit = 5
	}

	// Build SearxNG query parameters. We assume the instance has the JSON
	// API enabled and use the standard `q`, `format`, `categories`, and
	// `language` parameters. This is intentionally minimal.
	values := url.Values{}
	values.Set("q", req.Query)
	values.Set("format", "json")
	values.Set("limit", strconv.Itoa(limit))

	// Map logical sources into SearxNG categories. For now we focus on
	// web/general results, but keep the mapping flexible for future use.
	categories := []string{}
	for _, s := range req.Sources {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "images":
			categories = append(categories, "images")
		case "news":
			categories = append(categories, "news")
		default:
			categories = append(categories, "general")
		}
	}
	if len(categories) == 0 {
		categories = []string{"general"}
	}
	values.Set("categories", strings.Join(categories, ","))

	// Use country/location as a best-effort hint for language/region.
	if req.Country != "" {
		values.Set("language", strings.ToLower(req.Country))
	} else if req.Location != "" {
		values.Set("language", req.Location)
	}

	// Time-based search parameter, if provided. SearxNG supports a
	// `time_range` parameter with values like "day", "week", etc.
	if req.TBS != "" {
		values.Set("time_range", req.TBS)
	}

	// SearxNG exposes its search API on /search and, by default,
	// expects POST requests. To align with that and avoid 403s from
	// method restrictions, we send a form-encoded POST.
	endpoint := p.baseURL + "/search"

	encoded := values.Encode()

	// Apply a request-scoped timeout on top of the client's own timeout.
	timeout := p.timeout
	if req.Timeout > 0 {
		timeout = req.Timeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("searxng search failed with status %d", resp.StatusCode)
	}

	var payload searxngResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	out := &Results{Web: make([]Result, 0, len(payload.Results))}
	for _, r := range payload.Results {
		if strings.TrimSpace(r.URL) == "" {
			// Optionally drop invalid URLs when requested; otherwise include
			// them as plain results without a usable URL.
			if req.IgnoreInvalidURL {
				continue
			}
		}
		out.Web = append(out.Web, Result{
			Title:       r.Title,
			Description: r.Content,
			URL:         r.URL,
		})
	}

	return out, nil
}
