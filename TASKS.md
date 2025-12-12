# Raito Tasks

## Legend

- `[x]` – done
- `[ ]` – pending / planned

## Phase 1 – Skeleton & Scrape

- [x] Initialize Go module and folder structure (`cmd/raito-api`, `internal/config`, `internal/http`, `config/`, `db/`).
- [x] YAML config loader and sample `config/config.yaml`.
- [x] Fiber server with `/healthz` and `/v1` router group.
- [x] Define Firecrawl-style types for scrape/map/crawl in `internal/http/types.go`.
- [x] Implement basic HTTP scraper (`internal/scraper`) using `net/http` + `goquery`.
- [x] Wire `/v1/scrape` to the scraper with Firecrawl-like response shape.
- [x] Improve markdown generation and metadata extraction (html-to-markdown, OG tags, language, canonical URL).

## Phase 2 – Map

- [x] Implement `internal/crawler.Map`:
  - [x] Fetch `/sitemap.xml` and parse URLs.
  - [x] Fetch root HTML and extract anchor links.
  - [x] Filter by domain/subdomain (with `includeSubdomains`).
  - [x] Respect robots.txt when enabled.
  - [x] Apply `search`, `limit`, and `ignoreQueryParameters` filters.
- [x] Wire `/v1/map` handler to `crawler.Map`:
  - [x] Parse `MapRequest` body and validate `url`.
  - [x] Derive options (limit, sitemap mode, timeouts) from request + config.
  - [x] Return `MapResponse` with `success`, `links`, and optional `warning`.
- [x] Improve ergonomics:
  - [x] Always return a `links` array (empty on error or no results).
  - [x] Support `allowExternalLinks` to include off-domain URLs when requested.
  - [x] Add warning when very few results are returned for a deep path, suggesting mapping the base domain instead.

## Phase 3 – Crawl (DONE)

- [x] Design crawl request/response in more detail using Firecrawl v2 as reference.
- [x] Add goose migrations for:
  - [x] `api_keys` (id, key_hash, label, is_admin, rate_limit_per_minute, tenant_id, created_at, revoked_at).
  - [x] `jobs` (id, type, status, url, input JSONB, error, created_at, updated_at, completed_at).
  - [x] `documents` (id, job_id, url, markdown, html, raw_html, metadata JSONB, status_code, created_at).
- [x] Configure sqlc to generate `internal/db` package and write initial queries:
  - [x] `GetAPIKeyByHash` (plus Store helper `GetAPIKeyByRawKey`).
  - [x] `InsertJob`, `UpdateJobStatus`, `GetJobByID`.
  - [x] `InsertDocument`, `GetDocumentsByJobID`.
- [x] Implement a simple job runner for crawl jobs:
  - [x] Poll for pending jobs and mark them running.
  - [x] Use crawler + scraper to process URLs according to options.
  - [x] Store documents and update job status + totals.
- [x] Implement `/v1/crawl`:
  - [x] Validate request and create crawl job.
  - [x] Return `id` and status URL.
- [x] Implement `GET /v1/crawl/:id`:
  - [x] Return status, totals, and batch of documents.

## Phase 4 – Rod & Dynamic Pages (DONE)

- [x] Add rod integration in `internal/scraper` for JS-rendered pages.
- [x] Add an `engine` field to scrape responses to indicate HTTP vs browser engine.
- [x] Normalize extracted links to absolute URLs in both HTTP and rod scrapers.

## Phase 5 – Extraction & Advanced Features (Future)

- [x] Design a minimal `POST /v1/extract` that:
  - [x] Accepts URLs and a simple JSON schema or prompt.
  - [x] Scrapes URLs and calls a configured LLM provider to produce structured JSON.

## Housekeeping

- [x] Add logging wrappers with contextual fields (request id, method, path).
- [x] Add minimal metrics (request counts, latencies, error rates).
- [x] Add configuration/env documentation for running Raito locally.
- [x] Revisit DB connection handling to use a shared *sql.DB with proper pooling instead of opening a new connection per Store call.
