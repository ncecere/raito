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

- `POST /v1/extract` endpoint (v1 scope):
  - Accepts URLs and a simple JSON schema or prompt.
  - Scrapes the URLs and calls a configured LLM provider to produce structured JSON.
  - Returns both raw documents and extracted fields for debugging.
- Search + scrape (Firecrawl `search` analogue, v2+):
  - Use a search API to find URLs for a query.
  - Optionally map/crawl and scrape those URLs before returning results.

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

Raito currently returns markdown, HTML, raw HTML, and links in `/v1/scrape`. To more closely match Firecrawl's scrape formats, consider:

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
