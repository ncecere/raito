# Raito Roadmap

This document tracks **post-v1** enhancements. Items here are not required for the initial v1 release but are candidates for future work.

## 1. Browser Actions & Advanced Rod Features

Raito v1 supports rod-based scraping via the `useBrowser` flag on `/v1/scrape`. The next step is to make browser sessions interactive.

- Add a minimal `actions` array to `ScrapeRequest`:
  - Support a small subset of Firecrawl-style actions: `waitForSelector`, `click`, `type`, `sleep`.
  - Apply actions only when `useBrowser=true` and rod is enabled.
  - Keep the schema simple and well-documented to avoid over-designing the DSL.
- Implement actions in the rod scraper:
  - `waitForSelector` → wait for an element before proceeding.
  - `click` → click an element (with optional retries/timeouts).
  - `type` → focus an element and type provided text.
  - `sleep` → explicit delays for brittle pages, still bounded by the overall request timeout.
- Consider additional niceties once the basics are stable:
  - Ability to scroll to bottom / to a selector.
  - Capture a screenshot (for debugging) alongside HTML/markdown.
  - Per-action timeouts vs a single global timeout.

## 2. Extraction & LLM-Backed Features

These mirror the ideas in `PLAN.md` Phase 6 and include both v1 and v2 work.

- `POST /v1/extract` endpoint (v1 scope) (DONE in v1):
  - Accepts URLs and a simple JSON schema or prompt.
  - Scrapes the URLs and calls a configured LLM provider to produce structured JSON.
  - Returns both raw documents and extracted fields for debugging.

## 3. Search & SearxNG Integration

Raito can offer a Firecrawl-style `/v1/search` endpoint that discovers URLs via pluggable search providers (starting with SearxNG) and optionally scrapes the top results using the existing scraper.

- Add `/v1/search` with a Firecrawl-compatible `SearchRequest`/`SearchData` shape (including `query`, `sources`, `categories`, `limit`, `tbs`, `location`, `timeout`, `ignoreInvalidURLs`, `scrapeOptions`, and `integration`), returning `web`/`news`/`images` arrays that can contain either plain search results or full `Document` objects.
- Introduce an `internal/search` package with a provider interface and a concrete `SearxngProvider` that calls a configured SearxNG instance, mapping our `SearchRequest` into SearxNG's JSON API and results back into Firecrawl-style `SearchResult*` structures.
- Support optional `scrapeOptions` on `/v1/search` that reuses the existing `internal/scraper` (and `useBrowser` when available) to scrape the top N results with bounded concurrency and per-request timeouts, merging documents into the `web`/`news`/`images` arrays.
- Add configuration for search (enable/disable, provider name, SearxNG base URL, default/max results, timeouts, and max concurrent scrapes), plus basic logging and metrics for search requests and scraped result counts.

## 4. Advanced Extract (Firecrawl-Style Async)

Raito's `/v1/extract` can evolve into a Firecrawl-style async extract service that operates on multiple URLs at once, is schema-driven (plus prompt), and exposes job-oriented status and metadata.

- Move to an async, job-based extract API (DONE in v1):
  - Treat `POST /v1/extract` as a job-creation endpoint that immediately returns an `ExtractResponse` with `id`, `status`, and optional `warning`/`error`, and add `GET /v1/extract/:id` for polling job status/results, mirroring Firecrawl's `/v2/extract` semantics.
- Make `urls` the primary input and drop `url`/`fields` entirely (PARTIAL – `urls` added, `url`/`fields` still supported for now):
  - Redefine `ExtractRequest` around `urls: []string`, `prompt`, and `schema` (JSON schema-like object) plus Firecrawl-style flags like `ignoreInvalidURLs`, `enableWebSearch`, `allowExternalLinks`, `showSources`, `scrapeOptions`, and `integration`.
- Use schema + prompt as the core contract:
  - Focus extraction on a `schema` + `prompt` model instead of named fields, allowing nested JSON outputs and richer structures while keeping the endpoint simple to consume.
- Reuse jobs/worker infrastructure and enrich metadata:
  - Implement an `extract` job type backed by the existing `jobs`/`documents` tables, and extend extract responses with job metadata (`id`, `status`, `expiresAt`, `creditsUsed`, `sources`, per-URL errors) plus structured logging and metrics that capture provider/model usage and URLs processed.


## 3. Reliability, Roles & Concurrency

These expand on the reliability phase in `PLAN.md`.

- DB pooling: replace per-call DB connection opens with a shared `*sql.DB` and tuned pool settings.
- Process roles: add a `--role=api|worker|all` flag so API and worker processes can be scaled and deployed independently.
- Concurrency tuning: introduce configuration for max concurrent crawl jobs per worker (and per-job limits) to better control resource usage.
- Add structured logging with request IDs and key fields (method, path, status, latency).
- Expose basic metrics (request counts, latencies, error rates, crawl job outcomes).
- Enhance health checks to cover DB, Redis, and rod connectivity where applicable.

## 4. Multi-Tenant & Quotas (Future)

- Extend `api_keys` with clearer tenant semantics and optional per-tenant quotas.
- Add simple per-tenant usage tracking (scrapes, crawls, pages processed).
- Optional: basic soft limits / warnings when tenants approach configured caps.

## 5. Scrape Ergonomics & Metadata

- Persist engine information in stored crawl documents (e.g., add an `engine` column in `documents`) so `GET /v1/crawl/:id` can report which scraper was used per page.
- Add configuration for link inclusion/filters, such as:
  - Same-domain-only links vs all HTTP(S) links.
  - Optional max number of links per document.
- Consider exposing additional link metadata (e.g., link text, `rel` attributes) in a future `links` structure while keeping the current simple string array for backward compatibility.

## 6. Scrape Formats Parity (with Firecrawl)

Raito currently returns markdown, HTML, raw HTML, and links in `/v1/scrape` and per-page documents from `/v1/crawl`. To more closely match Firecrawl's scrape formats, consider:

- **Summary format (`summary`)**
  - Add an optional `summary` field (or `formats` option) on `/v1/scrape` that produces a short natural-language summary of the page using the configured LLM provider.
  - Reuse the `/v1/extract` infrastructure under the hood where possible.

- **Images format (`images`)**
  - Implement extraction of image URLs into the existing `Images []string` field on the `Document` type.
  - Support common sources: `<img src>`, `<source srcset>`, and CSS background images where feasible.

- **Screenshot format (`screenshot`)**
  - Using rod, add an optional `screenshot` output on `/v1/scrape` with options like `fullPage`, `quality`, and `viewport`.
  - Decide on representation (e.g., base64-encoded PNG/JPEG) and ensure it's optional to avoid large responses by default.

- **JSON format (`json`)**
  - Provide a `json` scrape format that returns structured JSON directly from `/v1/scrape`, likely by delegating to `/v1/extract` or a shared LLM-backed extraction layer.
  - Keep this configurable to avoid unnecessary LLM calls when not needed.

- **Branding format (`branding`)**
  - Add a `branding` format that extracts brand identity and design system information (colors, typography, logo, tone of voice) using LLMs.
  - Return this as a structured JSON block alongside the main document.

## 7. Batch Scrape & Bulk Operations

Batch scraping is Firecrawl's way to process many independent URLs in a single async job; Raito can mirror this to make large, one-off or recurring scrapes more efficient than running many individual `/v1/scrape` calls.

- Introduce a `/v1/batch/scrape` family of endpoints for starting, polling, cancelling, and inspecting batch jobs, modelled loosely on Firecrawl's `/v2/batch/scrape` (including `GET .../:id`, `DELETE .../:id`, and an `/errors` sub-resource).
- Represent batch jobs using the existing `jobs`/`documents` tables with a dedicated `batch_scrape` job type and per-job progress (`completed`, `total`) so clients can monitor batch progress and paginate through results.
- Support core batch options: a shared `options` block for scrape settings (formats, headers, actions, etc.), `maxConcurrency` per batch, `ignoreInvalidURLs`, basic `webhook` notifications, and `zeroDataRetention`/`integration` flags in line with other phases.
- Optionally accept an idempotency key when creating batches so client libraries can safely retry batch-start calls without duplicating work.
