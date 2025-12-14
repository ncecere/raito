# `/v1/search` – Provider-Backed Search + Scrape

`/v1/search` runs a query against a configured search provider (currently SearxNG) and optionally scrapes each result into a Firecrawl-style document. It supports both **search-only** and **search+scrape** modes.

This doc is for:
- **Deployers** – to see how search uses SearxNG and scrapers.
- **API users** – to run search and scrape workflows.
- **Developers** – to understand provider and ScrapeOptions behavior.

---

## 1. Configuration Requirements

Search is controlled by `search` in `config.yaml` (see `internal/config/config.go`).

Key options (see `deploy/config/config.example.yaml`):

```yaml
search:
  enabled: true
  provider: "searxng"        # currently only "searxng" is supported
  maxResults: 5               # hard upper bound on results
  timeoutMs: 60000            # overall timeout (search + scraping)
  maxConcurrentScrapes: 4     # future use
  searxng:
    baseURL: "http://searxng:8080"
    defaultLimit: 5
    timeoutMs: 10000
```

- When `search.enabled` is `false`, `/v1/search` responds with:
  - `503 Service Unavailable`, `code = "SEARCH_DISABLED"`.
- The provider must be reachable; misconfiguration returns `SEARCH_PROVIDER_ERROR`.

---

## 2. Request Shape (`POST /v1/search`)

Body (`SearchRequest` from `internal/http/types.go`):

```jsonc
{
  "query": "golang context cancellation",          // required
  "limit": 5,                                      // optional
  "timeout": 60000,                                // optional (ms)
  "sources": ["web"],                              // optional, currently only "web"
  "ignoreInvalidURLs": true,                       // optional
  "country": "us",                                // optional
  "location": "us",                               // optional provider hint
  "tbs": "d",                                     // optional time-based search (provider-specific)
  "scrapeOptions": {                               // optional
    "formats": ["markdown", "html"],            // restricted set
    "headers": { "X-Client": "search-example" },
    "useBrowser": false,
    "location": {
      "country": "de",
      "languages": ["de-DE", "en-US"]
    }
  }
}
```

### 2.1 Required fields

- `query` (string, required)
  - Search query string.

### 2.2 Sources

- `sources` (array of strings, optional)
  - Currently only `"web"` is supported.
  - If omitted, defaults to `["web"]`.
  - Any other value yields `400` with `code = "UNSUPPORTED_SOURCE"`.

### 2.3 Limits and timeouts

- `limit` (int, optional)
  - Desired maximum number of results.
  - Default: `search.maxResults` when set, otherwise 5.
  - Enforced as a hard upper bound both at provider request and response trimming.

- `timeout` (ms, optional)
  - Overall timeout for the search (and scraping, if enabled).
  - Default: `search.timeoutMs` or `scraper.timeoutMs`, falling back to 60000ms if unspecified.

### 2.4 `ignoreInvalidURLs`

- When `false` (default) and scraping is enabled, invalid URLs may be preserved in the results but scraping errors are reported.
- When `true`, `SearchService.ScrapeResults` drops results with invalid URLs and counts them in the `warning`/`counts` block.

### 2.5 ScrapeOptions and formats

`scrapeOptions` is optional. When omitted, `/v1/search` runs in **search-only** mode.

- `scrapeOptions.formats` (array, optional)
  - Allowed formats for `/v1/search` are restricted to:
    - `"markdown"`, `"html"`, `"rawHtml"`.
  - Any other string or object format is rejected with `400` and `code = "UNSUPPORTED_FORMAT"`.

- `scrapeOptions.headers`, `scrapeOptions.useBrowser`, `scrapeOptions.location`
  - Passed through to `SearchService.ScrapeResults`, which in turn uses scraper options similar to `/v1/scrape`.

---

## 3. Response Shapes

### 3.1 Search-only mode (no scrapeOptions)

```jsonc
{
  "success": true,
  "data": {
    "web": [
      {
        "title": "...",
        "description": "...",
        "url": "https://example.com/article"
      },
      {
        "title": "...",
        "description": "...",
        "url": "https://example.org/post"
      }
    ]
  }
}
```

- Each `web[]` entry is a normalized search result from the provider.
- No scraping is performed; `document` is not present.

### 3.2 Search + scrape mode (with scrapeOptions)

```jsonc
{
  "success": true,
  "data": {
    "web": [
      {
        "title": "...",
        "description": "...",
        "url": "https://example.com/article",
        "document": {
          "markdown": "...",
          "html": "...",
          "rawHtml": "...",
          "engine": "http" | "browser",
          "metadata": { ... }
        }
      },
      {
        "title": "...",
        "description": "...",
        "url": "https://invalid-url",
        "document": null
      }
    ],
    "warning": "2 results dropped due to invalid URLs or scrape errors",
    "counts": {
      "results": 10,
      "scraped": 7,
      "invalidUrls": 2,
      "scrapeErrors": 1
    }
  }
}
```

The exact shape of `warning` and `counts` is defined by `SearchService.ScrapeResults`, but in general you can expect:

- `warning` (string) – summarizing partial failures.
- `counts` (object) – numeric breakdown of total, scraped, invalid URLs, and scrape errors.

---

## 4. Error Handling

Errors are returned with the standard envelope:

```jsonc
{
  "success": false,
  "code": "BAD_REQUEST" | "BAD_REQUEST_INVALID_JSON" | "SEARCH_DISABLED" |
          "UNSUPPORTED_SOURCE" | "UNSUPPORTED_FORMAT" |
          "SEARCH_PROVIDER_ERROR" | "SEARCH_FAILED",
  "error": "human-readable message"
}
```

HTTP status codes:

- `400` – malformed JSON, missing `query`, unsupported source or format.
- `503` – search disabled in config.
- `500` – provider construction failure (`SEARCH_PROVIDER_ERROR`).
- `502` – upstream provider error (`SEARCH_FAILED`).
- `504` – overall timeout expired.

---

## 5. Examples

### 5.1 Search only

```bash
curl -X POST http://localhost:8080/v1/search \
  -H 'Authorization: Bearer <api-key>' \
  -H 'Content-Type: application/json' \
  -d '{
    "query": "golang context cancellation",
    "limit": 5
  }'
```

### 5.2 Search + scrape (markdown)

```bash
curl -X POST http://localhost:8080/v1/search \
  -H 'Authorization: Bearer <api-key>' \
  -H 'Content-Type: application/json' \
  -d '{
    "query": "test invalid urls",
    "limit": 10,
    "ignoreInvalidURLs": true,
    "scrapeOptions": {
      "formats": ["markdown"]
    }
  }'
```

### 5.3 Search + scrape with HTML and location hints

```bash
curl -X POST http://localhost:8080/v1/search \
  -H 'Authorization: Bearer <api-key>' \
  -H 'Content-Type: application/json' \
  -d '{
    "query": "nextjs app router tutorial",
    "limit": 3,
    "scrapeOptions": {
      "formats": ["markdown", "html", "rawHtml"],
      "headers": {
        "X-Debug-Client": "raito-search-example"
      },
      "location": {
        "country": "de",
        "languages": ["de-DE", "en-US"]
      }
    }
  }'
```

---

## 6. Operational Notes

- **SearxNG**: `/v1/search` depends on a SearxNG instance (or future providers) reachable at `search.searxng.baseURL` with JSON output enabled.
- **Scraping**: search+scrape mode can generate many scrape requests; use `limit` conservatively.
- **Rate limiting**: use `ratelimit.defaultPerMinute` and API-key rate limits to control abuse.
- **Future providers**: the `search.Provider` interface is designed to support additional providers without changing the HTTP layer.

For provider internals, see `internal/search` and `internal/search/README.providers.md`.
