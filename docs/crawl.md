# `/v1/crawl` – Site Crawling (Job-Based)

`/v1/crawl` launches an asynchronous crawl job that follows links across a site, stores normalized documents in the database, and lets you retrieve them later.

This doc is for:
- **Deployers** – to understand how crawls affect storage and workers.
- **API users** – to know how to fire crawls and retrieve results.
- **Developers** – to understand job lifecycle and how formats are applied.

---

## 1. Job Model

Crawls are long-running jobs executed by worker processes.

- `POST /v1/crawl` – enqueue a new crawl job.
- `GET /v1/crawl/:id` – fetch job status and, when complete, documents.

The crawl job record is stored in the `jobs` table; documents live in `documents`. A worker process (role `worker`) picks up crawl jobs and populates documents.

---

## 2. Enqueue Request (`POST /v1/crawl`)

Body (see `CrawlRequest` in `internal/http/types.go`):

```jsonc
{
  "url": "https://example.com",          // required
  "limit": 100,                           // optional, max pages
  "maxDepth": 3,                          // optional, link depth (from start URL)
  "timeout": 300000,                      // optional, overall timeout (ms)
  "allowExternalLinks": false,            // optional
  "allowSubdomains": false,               // optional
  "sitemap": "include",                  // optional: include|only|ignore
  "formats": ["markdown", "summary"],   // optional, applied to documents
  "scrapeOptions": {                      // optional (per-page scrape options)
    "headers": { "Accept-Language": "en-US" },
    "location": { "country": "us", "languages": ["en-US", "en"] }
  }
}
```

Key fields:

- `url` (string, required)
  - Starting URL for the crawl.

- `limit` (int, optional)
  - Maximum number of pages to crawl.
  - Defaults to `crawler.maxPagesDefault` (100 in the example config).

- `maxDepth` (int, optional)
  - Maximum link depth from the starting URL (0 = just the start URL, 1 = links from that page, etc.).
  - Defaults to `crawler.maxDepthDefault`.

- `timeout` (ms, optional)
  - Maximum time the worker will spend on the crawl.
  - If omitted, derived from worker and scraper defaults.

- `allowExternalLinks` / `allowSubdomains` (bool, optional)
  - When `false`, restricts crawling to the same host (and optionally subdomains).

- `sitemap` (string, optional)
  - How to use `sitemap.xml`:
    - `"include"` (default): sitemap + HTML link discovery.
    - `"only"`: only sitemap URLs.
    - `"ignore"`: ignore sitemaps.

- `formats` and `scrapeOptions`
  - Control which formats are stored per page (`markdown`, `html`, `rawHtml`, etc.) and how pages are scraped (headers, location, browser usage).
  - Same semantics as `/v1/scrape` and `ScrapeOptions`.

On success (`200 OK`), `crawlHandler` responds with:

```jsonc
{
  "success": true,
  "id": "<uuid>",
  "url": "http://localhost:8080/v1/crawl/<uuid>"
}
```

The `id` is a UUID (v7 when available, otherwise v4), and `url` is a convenience status URL.

---

## 3. Status Request (`GET /v1/crawl/:id`)

Example:

```bash
curl http://localhost:8080/v1/crawl/<id> \
  -H 'Authorization: Bearer <api-key>'
```

Possible responses:

### 3.1 Not found

```jsonc
{
  "success": false,
  "code": "NOT_FOUND",
  "error": "crawl job not found"
}
```

### 3.2 Pending/running/failed

```jsonc
{
  "success": true,
  "id": "<uuid>",
  "status": "pending" | "running" | "failed",
  "total": 0,
  "error": "optional job-level error string"
}
```

### 3.3 Completed with documents

When `status == "completed"`, documents are included:

```jsonc
{
  "success": true,
  "id": "<uuid>",
  "status": "completed",
  "total": 42,
  "data": [
    {
      "markdown": "...",          // if requested
      "html": "...",              // if requested
      "rawHtml": "...",           // if requested
      "links": ["..."],
      "images": ["..."],
      "summary": "...",           // if requested and LLM enabled
      "json": { ... },             // if requested
      "branding": { ... },
      "engine": "http" | "browser",
      "metadata": { ... }
    },
    {
      "markdown": "...",
      "metadata": { ... }
    }
  ]
}
```

Documents are built via `JobDocumentService.BuildDocuments`, using the formats from the *original* `CrawlRequest`. Summary and JSON are enabled by default for crawls (see `crawlStatusHandler`).

---

## 4. Operational Notes

- **Workers required**: crawl jobs are executed only by processes running with role `worker`. Ensure at least one worker is running.
- **Storage**: crawls write into `jobs` and `documents`; configure `retention` in `config.yaml` to GC old jobs and documents.
- **Robots**: respect for `robots.txt` is controlled by `robots.respect` in the config.
- **LLM usage**: formats like `summary`, `branding`, and JSON extraction use the configured LLM provider; misconfigurations surface as job-level errors.

---

## 5. Example Scenario – Crawl then Extract

1. Map the site with `/v1/map` to get a subset of URLs.
2. Submit those URLs to `/v1/crawl` with `formats: ["markdown"]`.
3. Once the crawl completes, run `/v1/extract` on a subset of URLs with a schema describing the data you want.

This pattern lets you:
- Discover the site structure.
- Collect normalized content in the DB.
- Extract structured JSON across pages using `/v1/extract` without re-scraping everything.
