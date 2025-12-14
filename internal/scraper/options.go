package scraper

import (
	"strings"
	"time"
)

// LocationOptions describes geo-related hints that influence HTTP
// headers (for example Accept-Language) when scraping.
type LocationOptions struct {
	Country   string
	Languages []string
}

// RequestOptions is a higher-level set of options used to construct a
// low-level scraper.Request in a consistent way across handlers and
// workers.
type RequestOptions struct {
	URL       string
	Headers   map[string]string
	TimeoutMs int
	UserAgent string
	Location  *LocationOptions
}

// BuildRequestFromOptions builds a scraper.Request from higher-level
// RequestOptions, applying shared behavior such as Accept-Language
// headers derived from LocationOptions.
func BuildRequestFromOptions(opts RequestOptions) Request {
	headers := map[string]string{}
	for k, v := range opts.Headers {
		headers[k] = v
	}

	// Apply location settings to Accept-Language when provided.
	if opts.Location != nil {
		if len(opts.Location.Languages) > 0 {
			headers["Accept-Language"] = strings.Join(opts.Location.Languages, ",")
		} else if opts.Location.Country != "" {
			headers["Accept-Language"] = opts.Location.Country
		}
	}

	var timeout time.Duration
	if opts.TimeoutMs > 0 {
		timeout = time.Duration(opts.TimeoutMs) * time.Millisecond
	}

	return Request{
		URL:       opts.URL,
		Headers:   headers,
		Timeout:   timeout,
		UserAgent: opts.UserAgent,
	}
}
