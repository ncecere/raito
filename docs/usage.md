# Using the Raito API

Raito is a Go-based HTTP API for scraping single pages, mapping and crawling sites, performing search via SearxNG, and extracting structured data with LLMs.

This document gives a high-level overview of the main endpoints and how to call them. For detailed "golden" curl examples for `/v1/scrape` and `/v1/search`, see the top-level `README.md` in the repository root.

Assumptions:

- The API is running on `http://localhost:8080`.
- You have a valid API key and send it as `Authorization: Bearer <key>`.

---

## Authentication and API keys

When `auth.enabled` is `true` (the default), all `/v1/*` and `/admin/*` endpoints require an API key.

1. Start the API with a config that sets `auth.initialAdminKey`.
2. Use that value as your admin key to create additional keys via `/admin/api-keys`.

For multi-tenancy and browser sessions (local/OIDC auth, tenants, and tenant-scoped API keys), see `docs/multi-tenancy.md`.

Example (admin creates a user key):

```bash
curl -X POST http://localhost:8080/admin/api-keys \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <admin-key>' \
  -d '{"label": "example-user-key", "rateLimitPerMinute": 60}'
```

The response includes a `key` field. Use that key for requests to `/v1/*`.

---

## Health and metrics

- `GET /healthz` – basic health check (no auth required by default).
- `GET /metrics` – Prometheus-style metrics (HTTP requests, LLM usage, etc.).

These are useful for integrating Raito into monitoring systems and for basic readiness checks.

For a reference of structured log events (request logs and job-level logs for scrape/map/crawl/batch/extract), see `docs/logging.md`.

**Formats note:** Raito uses a Firecrawl-style `formats` array across endpoints. `/v1/scrape` supports rich formats like `markdown`, `html`, `rawHtml`, `links`, `images`, `summary`, `branding`, `screenshot`, and `json` (via `{type:"json", ...}` objects). `/v1/search` is intentionally restricted to the subset `markdown`, `html`, `rawHtml` for scraped documents to keep payloads small and predictable.

---

## /v1/scrape – single-page scrape

`POST /v1/scrape` fetches a single URL and returns a document with optional formats.

Key request fields:

- `url` (string, required) – page to scrape.
- `useBrowser` (bool, optional) – when `true` and rod is enabled, uses a headless browser.
- `headers` (object, optional) – extra HTTP headers to send.
- `timeout` (number, optional) – per-request timeout (ms).
- `formats` (array, optional) – which outputs to compute. Supported values include:
  - Strings: `"markdown"`, `"html"`, `"rawHtml"`, `"links"`, `"images"`, `"summary"`, `"branding"`, `"screenshot"`.
  - Objects with `type: "json"` for structured extraction with a prompt and optional JSON schema.

Response shape (simplified):

```json
{
  "data": {
    "markdown": "...",   // when requested
    "html": "...",       // when requested
    "rawHtml": "...",    // when requested
    "links": ["..."],     // absolute links
    "images": ["..."],    // image URLs
    "summary": "...",     // LLM-generated summary
    "json": { ... },       // structured JSON per your schema
    "branding": { ... },   // branding profile
    "screenshot": "...", // base64-encoded image (if enabled)
    "engine": "http" | "browser",
    "metadata": {
      "url": "...",
      "title": "...",
      "description": "...",
      "statusCode": 200,
      "language": "en",
      "openGraph": { ... }
    }
  }
}
```

See `README.md` at the repo root for concrete examples combining `markdown`, `summary`, `json`, and `branding` formats.

---

## /v1/map – URL discovery

`POST /v1/map` discovers links for a site using sitemap and HTML link discovery.

Common request fields:

- `url` (string, required) – starting URL or domain.
- `allowExternalLinks` (bool, optional) – whether to follow links off the starting domain.
- `limit` (number, optional) – maximum number of links.

Response:

- `links[]` – array of `{ url, title, description }`.
- `warning` – optional message when limits are hit or robots.txt prevents full discovery.

`/v1/map` is often used as a precursor to a crawl.

---

## /v1/crawl – crawl jobs

Crawls are asynchronous jobs stored in the database and processed by worker processes.

Endpoints:

- `POST /v1/crawl` – create a new crawl job.
- `GET /v1/crawl/:id` – get job status and any scraped documents.

Typical flow:

1. Create a job:

   ```bash
   curl -X POST http://localhost:8080/v1/crawl \
     -H 'Content-Type: application/json' \
     -H 'Authorization: Bearer <user-key>' \
     -d '{"url": "https://example.com", "limit": 10}'
   ```

   Response includes a job `id`.

2. Poll status:

   ```bash
   curl http://localhost:8080/v1/crawl/<id> \
     -H 'Authorization: Bearer <user-key>'
   ```

Status responses include:

- `status` – e.g. `pending`, `running`, `completed`, `failed`.
- `documents[]` – scraped documents with the same shape as `/v1/scrape` (filtered by formats stored for the job).

---

## /v1/batch/scrape – batch jobs

`/v1/batch/scrape` lets you submit multiple URLs in a single job and retrieve all results later.

- `POST /v1/batch/scrape` – create batch job with an array of URLs and formats.
- `GET /v1/batch/scrape/:id` – retrieve job status and per-URL documents.

Each document in the response uses the same document shape as `/v1/scrape`, but organized by job and URL.

---

## /v1/search – SearxNG-backed search

`POST /v1/search` runs a query against a configured search provider (currently SearxNG) and optionally scrapes each result.

Important fields:

- `query` (string, required).
- `limit` (number, optional) – capped by `search.maxResults` from config.
- `ignoreInvalidURLs` (bool, optional) – when `true`, invalid URLs and scrape failures are dropped.
- `sources` (array, optional) – currently `"web"`.
- `scrapeOptions` (object, optional):
  - `formats` – subset of `"markdown"`, `"html"`, `"rawHtml"`.
  - `headers` – headers applied when scraping search results.
  - `location` – `{ country, languages[] }` to influence provider behavior and `Accept-Language`.

Behavior:

- Without `scrapeOptions` → search-only: results include `title`, `description`, `url`.
- With `scrapeOptions` → search + scrape: each result includes an optional `document` (same shape as `/v1/scrape` but limited formats), plus `engine` and `metadata`.

The response also includes:

- `warning` – explains how many results were dropped due to invalid URLs or scrape errors.
- `counts` – numbers of scraped results, invalid URLs, and scrape errors.

Golden examples for both modes are in the root `README.md`.

---

## /v1/extract – structured JSON extraction

`/v1/extract` is an asynchronous endpoint that scrapes one or more URLs and uses an LLM to produce JSON shaped by a caller-provided schema.

High-level flow:

- `POST /v1/extract` – enqueue a job with `urls[]`, `schema`, optional prompts, and LLM overrides.
- `GET /v1/extract/:id` – poll job status and retrieve per-URL `results[]` JSON plus optional `sources[]` and `summary` when the job completes.

For full details on request/response shape, field semantics, and error codes, see `docs/extract.md`.

---

## Admin and observability summary

- `GET /healthz` – probe for readiness/liveness.
- `GET /metrics` – scrape with Prometheus.
- `/admin/api-keys` – manage API keys (requires admin key).

Once the API is running (see `docs/deploy.md`), you can use these endpoints with the examples above and the golden curl snippets in the root `README.md` to validate that scraping, crawling, search, and extraction all behave as expected.
