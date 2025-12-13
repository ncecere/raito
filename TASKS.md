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

## Phase 2b – Map Options Parity

- [ ] Extend `MapRequest` and map implementation to support additional Firecrawl map options:
  - `integration` field to tag map calls with the originating integration/client.
  - `location` (country, languages) to enable geo-aware mapping when needed.

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

## Phase 5 – Extraction & Advanced Features

- [x] Design a minimal `POST /v1/extract` that:
  - [x] Accepts URLs and a simple JSON schema or prompt.
  - [x] Scrapes URLs and calls a configured LLM provider to produce structured JSON.
- [ ] Add search + scrape (Firecrawl-style `search` analogue) that uses a search API to discover URLs and optionally map/crawl/scrape them.

## Phase 6 – Reliability, Roles & Concurrency

- [x] Add a `-role` flag (`api|worker|all`) to `cmd/raito-api` and wire roles so API and worker processes can be run separately or together.
- [x] Add worker configuration to `config/config.yaml` (e.g. `worker.maxConcurrentJobs`, `worker.pollIntervalMs`, `worker.maxConcurrentURLsPerJob`) and enforce a max number of concurrent crawl jobs per worker in `StartCrawlWorker`.
- [x] Optionally add per-job URL concurrency controls in `runCrawlJob`.
- [x] Enhance health checks to cover DB and Redis connectivity (and rod when enabled), either via `/healthz?deep=true` or a dedicated `/healthz/deps` endpoint.

## Phase 8 – Scrape Ergonomics & Metadata

- [x] Persist engine information in stored crawl documents (e.g., add an `engine` column in `documents`) so `GET /v1/crawl/:id` can report which scraper was used per page.
- [x] Add configuration for link inclusion/filters, such as same-domain-only links vs all HTTP(S) links and optional max number of links per document.
- [x] Consider exposing additional link metadata (e.g., link text, `rel` attributes) in a future `links` structure while keeping the current simple string array for backward compatibility.

## Phase 9 – Scrape & Crawl Formats Parity

- [x] Add a `summary` format that produces a short natural-language summary for documents returned by `/v1/scrape` and `/v1/crawl`, using the configured LLM provider (reusing `/v1/extract` infrastructure where possible).
- [x] Implement `summary` format support for `/v1/scrape`, gated via `formats` and using the configured LLM provider.
- [x] Extend crawl responses to support the `summary` format for `/v1/crawl`, gated via `formats` on the original crawl request.
- [x] Populate the existing `Images []string` field on `Document` and support an `images` format that extracts image URLs from `<img src>`, `<source srcset>`, etc., for both scrape and crawl documents.
- [ ] Add a `screenshot` format that uses rod to capture screenshots (with options like `fullPage`, `quality`, `viewport`), represented in an appropriate format (e.g., base64-encoded PNG/JPEG), and make it usable via both scrape and crawl. (Scrape implemented; crawl pending.)
- [x] Add a `json` format that returns structured JSON directly for documents produced by `/v1/scrape` and `/v1/crawl`, delegating to `/v1/extract` or a shared extraction layer.
- [x] Add a `branding` format that extracts brand identity/design system information (colors, typography, logo, tone of voice) using LLMs and returns it as structured JSON for both scrape and crawl.

## Phase 10 – Scrape & Crawl Advanced Options

- [x] Extend `ScrapeRequest` to support non-format scrape options from Firecrawl, including:
  - [x] Location (`country`, `languages`) for geo-aware scraping (wired to `Accept-Language` on outgoing requests).
  - [x] An `integration` field for tagging usage (e.g., which client/integration triggered the request).
- [x] Extend `CrawlRequest` to include additional crawl-level options from Firecrawl:
  - [x] `crawlEntireDomain` / `maxConcurrency` / nested `scrapeOptions` added to the request types.
  - [x] `maxConcurrency` is enforced as a per-crawl URL concurrency cap (never exceeding global worker settings).
  - [x] Implement initial semantics for `crawlEntireDomain` (forcing `includeSubdomains` in URL discovery) and `scrapeOptions` (headers + location) inside `runCrawlJob`.

## Phase 11 – Batch

- [x] Design `BatchScrapeRequest` and `BatchScrapeResponse` types in `internal/http/types.go` for a basic batch scrape (required `urls: []string`, optional `formats`).
- [x] Extend the `jobs` model and store usage to support a `batch_scrape` job type, storing the URL list and options in `jobs.input` and tracking progress via associated `documents`, so `GET /v1/batch/scrape/:id` can report it.
- [x] Implement `POST /v1/batch/scrape` to create a batch job:
  - Validate that `urls` is a non-empty array and normalize/validate URLs.
  - Enforce a simple upper bound on batch size (currently 1000 URLs).
- [x] Implement `GET /v1/batch/scrape/:id` to return batch status and documents (`status`, `total`, `data`), using existing crawl document mapping logic and honoring `formats` (markdown/html/rawHtml/images).
- [x] Update the worker layer to pick up `batch_scrape` jobs:
  - Reuse the existing HTTP scraper to process each URL in the batch with per-job URL concurrency control.
  - Respect global worker settings so batch jobs coexist correctly with crawl jobs.
- [x] Integrate batch scrapes with retention and metrics (TTL cleanup via `retention` config and job/doc deletion counters).
- [ ] Add optional features such as idempotency, per-URL error reporting, webhooks, and richer metrics for batch scrapes (requests, pages processed, failures).

## Phase 12 – Search & SearxNG

- [x] Design Firecrawl-style `SearchRequest`/`SearchData` types in `internal/http/types.go` for `/v1/search`, including `query`, `sources`, `categories`, `limit`, `tbs`, `location`, `timeout`, `ignoreInvalidURLs`, `scrapeOptions`, and `integration`, with `web`/`news`/`images` arrays that can contain either plain search results or full `Document` objects (v1 currently populates `web` only and attaches scraped documents when requested).
- [x] Add a `search` configuration section in `config/config.yaml` (and example) to control enabling/disabling search, select a provider name (`searxng` initially), and set defaults for max results, timeouts, and max concurrent scrapes.
- [x] Implement an `internal/search` package with a provider interface and a concrete `SearxngProvider` that calls a configured SearxNG instance (JSON API), mapping Raito's `SearchRequest` into provider-specific query parameters and mapping results back into Firecrawl-style `SearchResult*` structures.
- [x] Implement `POST /v1/search` using the configured provider to perform search-only requests (no scraping) with validation for `query`, `limit`, `timeout`, and basic error handling when search is disabled or misconfigured.
- [x] Extend `/v1/search` to support `scrapeOptions` by reusing `internal/scraper` to scrape the top N web search results with request-scoped timeouts, merging scraped `Document` objects into the `data.web` array (v1 scrapes sequentially; concurrency controls are a possible future enhancement).
- [ ] Define behavior for `ignoreInvalidURLs` during the scrape phase (e.g., whether to keep plain search results when scrapes fail vs dropping them) and consider how to surface partial-failure warnings beyond the current generic warning message.
- [ ] Add structured logging and metrics for search requests (query, provider, sources, scraped count, errors) and expose basic counters/latencies alongside existing `/metrics` output.

## Phase 13 – Extract++ (Firecrawl-Style Async)

- [ ] Redesign `ExtractRequest` and `ExtractResponse` in `internal/http/types.go` to match Firecrawl-style extract:
  - [ ] Make `urls: []string` the only URL field (remove `url`).
  - [ ] Remove legacy `fields` mode from the public API.
  - [ ] Add new request fields: `systemPrompt`, `ignoreInvalidURLs`, `enableWebSearch`, `allowExternalLinks`, `showSources`, `scrapeOptions`, and `integration`.
  - [ ] Extend `ExtractStatusResponse` to include additional metadata (e.g. `expiresAt`, `sources`, per-URL errors).
  - [ ] Update worker and handlers to use the new shapes end-to-end and adjust validation accordingly.
- [x] Convert `/v1/extract` into an async, job-based endpoint that immediately enqueues an `extract` job using the existing `jobs`/worker infrastructure (storing URLs, schema, prompts, provider/model, and options in `jobs.input`) and returns an initial `ExtractResponse` with `id`, `status`, and any immediate validation errors.
- [x] Add `GET /v1/extract/:id` (and optionally a lightweight polling helper if needed) that returns Firecrawl-like job metadata (`id`, `status`, `data`, `error`, `warning`, `expiresAt`, `creditsUsed`, `sources`) so clients can track progress and retrieve results once extraction completes.
- [x] Implement schema-driven extraction in the LLM layer by adjusting prompt construction and response parsing so the LLM returns JSON that matches the provided schema, including support for nested objects, and mapping that into the `data` field of `ExtractResponse`.
- [ ] Define clear semantics for `ignoreInvalidURLs`, `enableWebSearch`, and `allowExternalLinks` on extract requests, including how they interact with search/crawl under the hood in future phases, and ensure multi-URL jobs handle per-URL failures without aborting the entire job when appropriate.
- [ ] Expand extract-related logging and metrics (beyond `metrics.RecordLLMExtract`) to cover async jobs: track extract request/job counts, per-provider/model success/failure, URLs/documents processed per job, partial failures, and latency, and expose these via `/metrics` alongside existing scrape/crawl metrics.
