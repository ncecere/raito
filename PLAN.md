# Raito Plan

## Vision

Raito is a Go-based alternative to Firecrawl that exposes a clean HTTP API for turning URLs and websites into LLM-ready data (markdown, HTML, links, and later structured data). It should be self-hostable, efficient, and easy to integrate.

## Core Stack

- Language: Go 1.22+
- Web framework: Fiber
- Browser engine: rod (for JS-heavy pages, later phase)
- Config: YAML (with env overrides)
- DB: Postgres + goose + sqlc
- Caching/queues: Redis (for crawl jobs in later phase)

## API Surface (v1)

Raito exposes `/v1` endpoints whose payloads closely mirror Firecrawl v2 for interoperability:

- `POST /v1/scrape` – single URL scrape (sync)
- `POST /v1/map` – discover URLs for a domain
- `POST /v1/crawl` – async crawl job creation (later)
- `GET /v1/crawl/:id` – crawl job status + data (later)

No billing/payments/Stripe are included; authentication is via static API keys or a simple DB table.

## Phases

### Phase 1 – Skeleton & Scrape (DONE)

- Go module, config loader, and Fiber server.
- `/healthz` and `/v1` router group.
- `/v1/scrape` wired to a basic HTTP scraper returning markdown, HTML, raw HTML, links, and metadata.

### Phase 2 – Map (DONE)

- Implement map engine:
  - Fetch `/sitemap.xml` and extract URLs.
  - Fetch root HTML and extract anchor links.
  - Filter by domain/subdomain, robots.txt, search term, and limit.
- Wire `/v1/map` to the map engine using Firecrawl-like request/response types.
- Add quality-of-life features:
  - `allowExternalLinks` and `includeSubdomains` behavior.
  - Helpful warnings when results are very small.

### Phase 3 – Crawl (DONE)

- Introduce Postgres schema (goose migrations) for:
  - `api_keys`, `jobs`, `documents`.
- Implement a simple in-process crawl job runner:
  - `/v1/crawl` creates a job row and starts a background worker.
  - Worker uses the crawler + scraper to process URLs and store documents.
- Implement `GET /v1/crawl/:id`:
  - Returns status, total docs, and full crawl data.

### Phase 4 – Auth & Rate Limiting (DONE)

- Add `api_keys` table and sqlc helpers for API key storage.
- Implement API key auth using `Authorization: Bearer raito_*` tokens.
- Add Redis-backed per-key, per-minute rate limiting.
- Add `/admin/api-keys` admin-only endpoint to create user API keys.

### Phase 5 – Rod & Dynamic Pages (DONE)

- Integrate rod for JS-rendered pages:
  - Current: rod-based scraper implemented and `/v1/scrape` can use it via `useBrowser` when enabled in config.
- Extend scrape options and responses for v1:
  - Add an `engine` field in scrape responses to indicate HTTP vs browser engine.
  - Normalize extracted links to absolute URLs in both HTTP and rod scrapers.
- Add configuration for rod (browser URL, headless mode) and document how to run a compatible browser in local/dev.
- Future (see `ROADMAP.md`, not required for v1):
  - Support a small subset of Firecrawl-style `actions` (click, wait, scroll) for simple page interactions.
  - Add `actions` to `ScrapeRequest` for rod-only scripted interactions.

### Phase 6 – Extraction & Advanced Features (DONE)

- Add a simple extract endpoint that calls a configured LLM provider to turn scraped markdown into JSON. (DONE)

### Phase 7 – Reliability & Persistence (IN PROGRESS)

- Refine DB connection handling to use a shared `*sql.DB` with a tuned connection pool instead of opening and closing connections on every Store call. (DONE)
- Add basic observability: structured logging, request metrics, and simple health checks that cover DB and Redis connectivity. (logging and request metrics DONE; DB/Redis health checks TBD)

## Non-Goals (for this project)

- Payments, Stripe, or X402.
- Full replication of Firecrawl's advanced features (deep-research, complex billing, full SDK matrix) in v1.
