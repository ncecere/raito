package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"raito/internal/config"
	"raito/internal/model"
	"raito/internal/scraper"
	"raito/internal/search"
)

// SearchRequest is the internal representation of a search request
// used by SearchService. It mirrors the HTTP search payload but is
// decoupled from Fiber and JSON tags.
type SearchRequest struct {
	Query             string
	Sources           []string
	Limit             int
	Country           string
	Location          string
	TBS               string
	TimeoutMs         int
	IgnoreInvalidURLs bool
}

// SearchWebResult is a provider-agnostic representation of a single
// search hit. It intentionally mirrors the HTTP layer's shape but
// lives in the services package for reuse.
type SearchWebResult struct {
	Title       string
	Description string
	URL         string
}

// SearchResult groups web results and basic metadata about the
// provider used. Scraping and document enrichment are handled in the
// HTTP layer for now.
type SearchResult struct {
	Web          []SearchWebResult
	ProviderName string
}

// SearchService encapsulates the provider selection and execution of
// search queries. For scraped search results, it reuses ScrapeService
// to build Documents while the HTTP layer handles error mapping and
// response shaping.
type SearchService interface {
	Search(ctx context.Context, req *SearchRequest) (*SearchResult, error)
	ScrapeResults(ctx context.Context, base []search.Result, opts *SearchScrapeOptions, ignoreInvalid bool) (*SearchScrapeResult, error)
}

type SearchScrapeOptions struct {
	Formats    []interface{}
	Headers    map[string]string
	UseBrowser *bool
	Location   *LocationOptions
	TimeoutMs  int
}

type LocationOptions struct {
	Country   string
	Languages []string
}

type ScrapedWebResult struct {
	Title       string
	Description string
	URL         string
	Document    *model.Document
}

type SearchScrapeResult struct {
	Web              []ScrapedWebResult
	ScrapedCount     int
	InvalidURLCount  int
	ScrapeErrorCount int
}

type searchService struct {
	cfg     *config.Config
	scraper ScrapeService
}

// NewSearchService constructs a SearchService backed by the provided
// configuration.
func NewSearchService(cfg *config.Config) SearchService {
	return &searchService{
		cfg:     cfg,
		scraper: NewScrapeService(cfg),
	}
}

func (s *searchService) Search(ctx context.Context, req *SearchRequest) (*SearchResult, error) {
	if req == nil {
		return nil, fmt.Errorf("nil search request")
	}
	if strings.TrimSpace(req.Query) == "" {
		return nil, fmt.Errorf("empty search query")
	}

	// Build sources; v1 only supports "web", but we keep it flexible.
	sources := req.Sources
	if len(sources) == 0 {
		sources = []string{"web"}
	}

	// Provider selection is delegated to the internal search package.
	provider, err := search.NewProviderFromConfig(s.cfg)
	if err != nil {
		return nil, err
	}

	// Derive timeout from request and configuration.
	timeoutMs := req.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = s.cfg.Search.TimeoutMs
	}
	if timeoutMs <= 0 {
		timeoutMs = s.cfg.Scraper.TimeoutMs
	}
	if timeoutMs <= 0 {
		timeoutMs = 60000
	}

	// Derive limit; enforce a conservative default when unset.
	limit := req.Limit
	if limit <= 0 {
		limit = s.cfg.Search.MaxResults
	}
	if limit <= 0 {
		limit = 5
	}
	if s.cfg.Search.MaxResults > 0 && limit > s.cfg.Search.MaxResults {
		limit = s.cfg.Search.MaxResults
	}

	searchReq := &search.Request{
		Query:            req.Query,
		Sources:          sources,
		Limit:            limit,
		Country:          req.Country,
		Location:         req.Location,
		TBS:              req.TBS,
		Timeout:          time.Duration(timeoutMs) * time.Millisecond,
		IgnoreInvalidURL: req.IgnoreInvalidURLs,
	}

	results, err := provider.Search(ctx, searchReq)
	if err != nil {
		return nil, err
	}

	// Enforce the effective limit as a defensive measure.
	if limit > 0 && len(results.Web) > limit {
		results.Web = results.Web[:limit]
	}

	web := make([]SearchWebResult, 0, len(results.Web))
	for _, r := range results.Web {
		web = append(web, SearchWebResult{
			Title:       r.Title,
			Description: r.Description,
			URL:         r.URL,
		})
	}

	providerName := strings.ToLower(strings.TrimSpace(s.cfg.Search.Provider))
	if providerName == "" {
		providerName = "searxng"
	}

	return &SearchResult{
		Web:          web,
		ProviderName: providerName,
	}, nil
}

func (s *searchService) ScrapeResults(ctx context.Context, base []search.Result, opts *SearchScrapeOptions, ignoreInvalid bool) (*SearchScrapeResult, error) {
	if len(base) == 0 {
		return &SearchScrapeResult{Web: []ScrapedWebResult{}}, nil
	}

	// Derive timeout for scraping.
	timeoutMs := 0
	if opts != nil {
		timeoutMs = opts.TimeoutMs
	}
	if timeoutMs <= 0 {
		timeoutMs = s.cfg.Scraper.TimeoutMs
	}
	if timeoutMs <= 0 {
		timeoutMs = 30000
	}

	dur := time.Duration(timeoutMs) * time.Millisecond

	useBrowser := false
	if opts != nil && opts.UseBrowser != nil {
		useBrowser = *opts.UseBrowser
	}

	var engine scraper.Scraper
	if useBrowser && s.cfg.Rod.Enabled {
		engine = scraper.NewRodScraper(dur)
	} else {
		engine = scraper.NewHTTPScraper(dur)
	}

	out := make([]ScrapedWebResult, 0, len(base))
	invalidURLCount := 0
	scrapeErrorCount := 0
	scrapedCount := 0

	for _, r := range base {
		entry := ScrapedWebResult{
			Title:       r.Title,
			Description: r.Description,
			URL:         r.URL,
		}

		if strings.TrimSpace(r.URL) == "" {
			invalidURLCount++
			if ignoreInvalid {
				continue
			}
			out = append(out, entry)
			continue
		}

		// Build a scraper.Request using shared helpers to keep
		// headers and Accept-Language behavior consistent.
		var locOpts *scraper.LocationOptions
		if opts != nil && opts.Location != nil {
			loc := opts.Location
			locOpts = &scraper.LocationOptions{
				Country:   loc.Country,
				Languages: loc.Languages,
			}
		}

		baseHeaders := map[string]string{}
		if opts != nil && opts.Headers != nil {
			for k, v := range opts.Headers {
				baseHeaders[k] = v
			}
		}

		sReq := scraper.BuildRequestFromOptions(scraper.RequestOptions{
			URL:       r.URL,
			Headers:   baseHeaders,
			TimeoutMs: int(dur.Milliseconds()),
			UserAgent: s.cfg.Scraper.UserAgent,
			Location:  locOpts,
		})

		res, err := engine.Scrape(ctx, sReq)

		if err != nil {
			scrapeErrorCount++
			if ignoreInvalid {
				continue
			}
			out = append(out, entry)
			continue
		}

		formats := []interface{}{}
		if opts != nil {
			formats = opts.Formats
		}
		// For /v1/search, when no formats are provided we only include
		// markdown by default for scraped documents.
		if len(formats) == 0 {
			formats = []interface{}{"markdown"}
		}

		svcRes, err := s.scraper.Scrape(ctx, &ScrapeRequest{
			Result:  res,
			Formats: formats,
		})
		if err != nil {
			scrapeErrorCount++
			if ignoreInvalid {
				continue
			}
			out = append(out, entry)
			continue
		}
		if svcRes != nil && svcRes.Document != nil {
			entry.Document = svcRes.Document
			scrapedCount++
		}

		out = append(out, entry)
	}

	return &SearchScrapeResult{
		Web:              out,
		ScrapedCount:     scrapedCount,
		InvalidURLCount:  invalidURLCount,
		ScrapeErrorCount: scrapeErrorCount,
	}, nil
}
