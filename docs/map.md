# `/v1/map` – URL Discovery

`/v1/map` discovers URLs for a site using a combination of sitemap parsing and HTML link discovery. It is typically used as a precursor to crawling or batch scraping.

This doc is useful for:
- **Deployers** – to understand the impact on target sites and limits.
- **API users** – to generate seed URL lists.
- **Developers** – to see how map behaves with the job queue vs inline execution.

---

## 1. Request Shape

The JSON body is defined by `MapRequest` in `internal/http/types.go`.

```jsonc
{
  "url": "https://example.com",      // required
  "limit": 100,                       // optional
  "timeout": 30000,                   // optional (ms)
  "includeSubdomains": true,          // optional
  "ignoreQueryParams": true,          // optional
  "allowExternal": false,             // optional
  "search": "docs",                  // optional, filters URLs by substring
  "sitemap": "include"              // optional: "include" | "only" | "ignore"
}
```

### 1.1 Required fields

- `url` (string, required)
  - Starting URL for discovery. Typically a site root like `https://example.com`.

### 1.2 Limits and timeouts

- `limit` (number, optional)
  - Maximum number of links to return.
  - If omitted or non-positive, falls back to `crawler.maxPagesDefault` from config (default 100 in `config.example.yaml`).

- `timeout` (number, optional)
  - Per-request timeout in milliseconds for discovery.
  - If omitted, uses `scraper.timeoutMs` from config.

### 1.3 Domain and URL filtering

- `includeSubdomains` (bool, optional)
  - When `true`, links on subdomains of the starting host are included (e.g. `blog.example.com`).
  - When `false` or omitted, only the exact host is considered for same-domain checks.

- `ignoreQueryParams` (bool, optional)
  - When `true` (default), URLs are normalized without query parameters for deduplication.
  - When `false`, query parameters are considered part of the URL.

- `allowExternal` (bool, optional)
  - When `false` (default), only links on the same host (and optional subdomains) are included.
  - When `true`, links that point to external domains may be included up to the `limit`.

- `search` (string, optional)
  - When non-empty, used as a substring filter on URLs and/or titles. Only links matching the search string are kept.

- `sitemap` (string, optional)
  - Controls how `sitemap.xml` is used:
    - `"include"` (default) – combine sitemap URLs with HTML link discovery.
    - `"only"` – use only sitemap URLs.
    - `"ignore"` – ignore sitemaps and rely purely on HTML link discovery.

---

## 2. Execution Model

`/v1/map` prefers to use the job queue when configured, but can fall back to inline execution.

- **Job queue path**:
  - When a `WorkExecutor` is attached to the request context, `mapHandler` enqueues a map job and waits synchronously for completion.
  - Suitable for API-only nodes with separate workers.

- **Inline path**:
  - If no executor is present, `mapHandler` constructs a `services.MapRequest` and calls `MapService.Map` directly.
  - Discovery happens within the request lifecycle, subject to `timeout`.

---

## 3. Response Shape

On success (`200 OK`):

```jsonc
{
  "success": true,
  "links": [
    {
      "url": "https://example.com/",
      "title": "Home",
      "description": "Welcome to Example" 
    },
    {
      "url": "https://example.com/docs",
      "title": "Documentation",
      "description": "Docs overview"
    }
  ],
  "warning": "sitemap blocked by robots.txt; falling back to HTML discovery"
}
```

Fields:

- `links[]` – discovered links mapped from internal crawler links.
  - `url` – absolute URL.
  - `title` – page title when available.
  - `description` – short description/snippet when available.
- `warning` (string, optional)
  - Explains partial failures, limits reached, or robots.txt restrictions.

On error:

```jsonc
{
  "success": false,
  "links": [],
  "code": "BAD_REQUEST" | "BAD_REQUEST_INVALID_JSON" | "MAP_FAILED" | "MAP_TIMEOUT" | "JOB_NOT_STARTED",
  "error": "human-readable message"
}
```

HTTP status codes:

- `400` – bad request (missing `url`, invalid JSON).
- `502` – upstream (worker) error when using the executor.
- `504` – timeout (`MAP_TIMEOUT` or queue timeout).
- `500` – internal errors (unexpected).

---

## 4. Examples

### 4.1 Basic map of a site

```bash
curl -X POST http://localhost:8080/v1/map \
  -H 'Authorization: Bearer <api-key>' \
  -H 'Content-Type: application/json' \
  -d '{
    "url": "https://example.com",
    "limit": 50
  }'
```

### 4.2 Discover only sitemap URLs

```bash
curl -X POST http://localhost:8080/v1/map \
  -H 'Authorization: Bearer <api-key>' \
  -H 'Content-Type: application/json' \
  -d '{
    "url": "https://example.com",
    "sitemap": "only",
    "limit": 200
  }'
```

### 4.3 Include subdomains and external links

```bash
curl -X POST http://localhost:8080/v1/map \
  -H 'Authorization: Bearer <api-key>' \
  -H 'Content-Type: application/json' \
  -d '{
    "url": "https://example.com",
    "includeSubdomains": true,
    "allowExternal": true,
    "limit": 100
  }'
```

### 4.4 Filter by search string

```bash
curl -X POST http://localhost:8080/v1/map \
  -H 'Authorization: Bearer <api-key>' \
  -H 'Content-Type: application/json' \
  -d '{
    "url": "https://example.com",
    "search": "docs",
    "limit": 50
  }'
```

- Only URLs whose URL/title/description contain `"docs"` are returned (subject to crawler implementation).

---

## 5. Operational Notes

- Mapping a large site can generate significant traffic. Use `limit`, `sitemap`, and domain filters to keep load bounded.
- Respect for `robots.txt` is controlled by `robots.respect` in config (default `true`).
- Combine `/v1/map` with `/v1/crawl` or `/v1/batch/scrape` for deeper content extraction.
