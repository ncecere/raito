package services

import (
	"context"
	"time"

	"raito/internal/config"
	"raito/internal/crawler"
)

// MapRequest is the internal representation of a map request
// used by the MapService. It is derived from the HTTP DTO but
// only contains the normalized values needed by the crawler.
type MapRequest struct {
	URL               string
	Limit             int
	Search            string
	IncludeSubdomains bool
	IgnoreQueryParams bool
	AllowExternal     bool
	SitemapMode       string
	TimeoutMs         int
}

// MapLink represents a discovered URL with basic metadata
// returned by the MapService.
type MapLink struct {
	URL         string
	Title       string
	Description string
}

// MapResult is the internal result type returned by MapService.
type MapResult struct {
	Links   []MapLink
	Warning string
}

// MapService encapsulates the business logic for the /v1/map
// endpoint, delegating to the crawler package.
type MapService interface {
	Map(ctx context.Context, req *MapRequest) (*MapResult, error)
}

type mapService struct {
	cfg *config.Config
}

// NewMapService constructs a MapService backed by the crawler
// package and the provided configuration.
func NewMapService(cfg *config.Config) MapService {
	return &mapService{cfg: cfg}
}

func (s *mapService) Map(ctx context.Context, req *MapRequest) (*MapResult, error) {
	// Derive timeout for the underlying crawl/map operation.
	timeoutMs := req.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = s.cfg.Scraper.TimeoutMs
	}

	// Apply a context timeout when a positive timeout is configured.
	if timeoutMs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
		defer cancel()
	}

	res, err := crawler.Map(ctx, crawler.MapOptions{
		URL:               req.URL,
		Limit:             req.Limit,
		Search:            req.Search,
		IncludeSubdomains: req.IncludeSubdomains,
		IgnoreQueryParams: req.IgnoreQueryParams,
		AllowExternal:     req.AllowExternal,
		SitemapMode:       req.SitemapMode,
		Timeout:           time.Duration(timeoutMs) * time.Millisecond,
		RespectRobots:     s.cfg.Robots.Respect,
		UserAgent:         s.cfg.Scraper.UserAgent,
	})
	if err != nil {
		return nil, err
	}

	links := make([]MapLink, 0, len(res.Links))
	for _, l := range res.Links {
		links = append(links, MapLink{
			URL:         l.URL,
			Title:       l.Title,
			Description: l.Description,
		})
	}

	return &MapResult{
		Links:   links,
		Warning: res.Warning,
	}, nil
}
