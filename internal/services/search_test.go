package services

import (
	"context"
	"testing"

	"raito/internal/config"
	"raito/internal/model"
	"raito/internal/search"
)

// fakeScrapeService is a minimal ScrapeService implementation used to
// avoid invoking real scrapers in tests. ScrapeResults tests below are
// constructed so that Scrape is never actually called.
type fakeScrapeService struct{}

func (f *fakeScrapeService) Scrape(ctx context.Context, req *ScrapeRequest) (*ScrapeResult, error) {
	return &ScrapeResult{Document: &model.Document{}}, nil
}

func newSearchServiceWithFakeScraper() *searchService {
	cfg := &config.Config{}
	return &searchService{
		cfg:     cfg,
		scraper: &fakeScrapeService{},
	}
}

func TestSearchScrapeResults_IgnoreInvalidURLsFalse(t *testing.T) {
	svc := newSearchServiceWithFakeScraper()

	base := []search.Result{
		{Title: "a", URL: ""},
		{Title: "b", URL: "   "},
	}

	res, err := svc.ScrapeResults(context.Background(), base, nil, false)
	if err != nil {
		t.Fatalf("ScrapeResults returned error: %v", err)
	}

	if got := res.InvalidURLCount; got != len(base) {
		t.Fatalf("expected InvalidURLCount=%d, got %d", len(base), got)
	}
	if got := len(res.Web); got != len(base) {
		t.Fatalf("expected Web length=%d, got %d", len(base), got)
	}
}

func TestSearchScrapeResults_IgnoreInvalidURLsTrue(t *testing.T) {
	svc := newSearchServiceWithFakeScraper()

	base := []search.Result{
		{Title: "a", URL: ""},
		{Title: "b", URL: "   "},
	}

	res, err := svc.ScrapeResults(context.Background(), base, nil, true)
	if err != nil {
		t.Fatalf("ScrapeResults returned error: %v", err)
	}

	if got := res.InvalidURLCount; got != len(base) {
		t.Fatalf("expected InvalidURLCount=%d, got %d", len(base), got)
	}
	if got := len(res.Web); got != 0 {
		t.Fatalf("expected Web length=0 when ignoreInvalidURLs=true, got %d", got)
	}
}

// Note: propagation of IgnoreInvalidURLs from SearchService to the
// underlying search.Provider is exercised indirectly via the
// search.Request type; here we focus tests on the ScrapeResults
// behavior where ignoreInvalid controls how invalid URLs are handled.
