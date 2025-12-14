# Configuration – `config.yaml`

Raito is configured via a single YAML file loaded at startup (typically `config/config.yaml` or `deploy/config/config.yaml`). This document explains the main sections, defaults, and example scenarios.

This doc is aimed at:
- **Deployers** – to configure production or staging deployments.
- **Power users** – to tune timeouts and limits.
- **Developers** – to understand how config flows into services.

The Go struct backing the config is `internal/config.Config`.

---

## 1. Top-Level Structure

```yaml
server:
  host: "0.0.0.0"
  port: 8080

scraper:
  userAgent: "RaitoBot/1.0"
  timeoutMs: 30000
  linksSameDomainOnly: false
  linksMaxPerDocument: 0

crawler:
  maxDepthDefault: 3
  maxPagesDefault: 100

robots:
  respect: true

rod:
  enabled: true

database:
  dsn: "postgres://raito:raito@localhost:5432/raito?sslmode=disable"

redis:
  url: "redis://localhost:6379"

auth:
  enabled: true
  initialAdminKey: "change_me_admin_key"

ratelimit:
  defaultPerMinute: 60

worker:
  maxConcurrentJobs: 4
  pollIntervalMs: 2000
  maxConcurrentURLsPerJob: 1
  syncJobWaitTimeoutMs: 60000

search:
  enabled: true
  provider: "searxng"
  maxResults: 5
  timeoutMs: 60000
  maxConcurrentScrapes: 4
  searxng:
    baseURL: "http://searxng:8080"
    defaultLimit: 5
    timeoutMs: 10000

retention:
  enabled: true
  cleanupIntervalMinutes: 60
  jobs:
    defaultDays: 14
    scrapeDays: 7
    mapDays: 7
    extractDays: 30
    crawlDays: 30
  documents:
    defaultDays: 30

llm:
  defaultProvider: "openai"   # or anthropic, google
  openai:
    apiKey: "${OPENAI_API_KEY}"
    baseURL: "https://api.openai.com/v1"
    model: "gpt-4.1-mini"
  anthropic:
    apiKey: "${ANTHROPIC_API_KEY}"
    model: "claude-3-5-sonnet-20241022"
  google:
    apiKey: "${GOOGLE_API_KEY}"
    model: "gemini-1.5-flash"
```

---

## 2. Server and Storage

### 2.1 `server`

- `host` – bind address (inside containers usually `0.0.0.0`).
- `port` – HTTP port. Default 8080 in examples.

### 2.2 `database`

- `dsn` – Postgres DSN used by `internal/store` and `internal/db`.
  - Example: `postgres://raito:raito@postgres:5432/raito?sslmode=disable`.
  - Must be reachable from both API and worker processes.

### 2.3 `redis`

- `url` – Redis URL for rate limiting.
  - Example: `redis://redis:6379`.

---

## 3. Scraper, Crawler, Robots, Rod

### 3.1 `scraper`

Controls the low-level HTTP scraper used by `/v1/scrape`, `/v1/crawl`, `/v1/batch/scrape`, and search scraping.

- `userAgent` – default User-Agent header.
- `timeoutMs` – default timeout for scraping if the request does not override it.
- `linksSameDomainOnly` – influences link extraction; when `true`, only links on the same host are considered in link lists.
- `linksMaxPerDocument` – 0 means no explicit limit; otherwise caps links per document.

### 3.2 `crawler`

Default limits for `/v1/crawl` and `/v1/map` when the request omits them.

- `maxDepthDefault` – default max depth of link traversal.
- `maxPagesDefault` – default max number of pages.

### 3.3 `robots`

- `respect` (bool)
  - When `true`, the crawler respects `robots.txt` and may skip URLs.

### 3.4 `rod`

Controls headless browser scraping.

- `enabled` (bool)
  - When `true`, browser-based scraping is available for `/v1/scrape` and related flows (e.g., screenshots).
  - When `false`, requests that require the browser (e.g. screenshot format) return `SCREENSHOT_NOT_AVAILABLE`.

---

## 4. Auth and Rate Limiting

### 4.1 `auth`

- `enabled` (bool)
  - When `true`, all `/v1/*` and `/admin/*` endpoints require `Authorization: Bearer <key>`.
- `initialAdminKey` (string)
  - On startup, Raito ensures an admin API key exists with this value.
  - Use it to call `/admin/api-keys` and create proper user keys.

### 4.2 `ratelimit`

- `defaultPerMinute` (int)
  - Default rate limit per minute for API keys that don’t have their own limit set.

In production you typically:
- Set `auth.enabled: true`.
- Choose a strong `initialAdminKey`.
- Rely on `/admin/api-keys` to manage per-key rate limits.

---

## 5. Worker and Jobs

### 5.1 `worker`

Controls how the background worker processes crawl, batch-scrape, map, and extract jobs.

- `maxConcurrentJobs` – max active jobs per worker process.
- `pollIntervalMs` – how often the worker polls for new jobs.
- `maxConcurrentURLsPerJob` – per-job concurrency (e.g., how many URLs to process in parallel for extract).
- `syncJobWaitTimeoutMs` – how long API-side executor waits for synchronous jobs (e.g., `/v1/scrape` via queue) before timing out.

### 5.2 `retention`

Controls automatic deletion of old jobs and documents.

- `enabled` – whether cleanup runs.
- `cleanupIntervalMinutes` – how often cleanup runs.
- `jobs` – per-job-type retention in days.
- `documents` – document retention in days.

This keeps the database from growing without bound.

---

## 6. Search

See also `docs/search.md`.

- `enabled` – master switch for `/v1/search`.
- `provider` – provider name (`"searxng"` currently).
- `maxResults` – hard upper bound on results per request.
- `timeoutMs` – overall search timeout (search + scrape).
- `maxConcurrentScrapes` – planned for controlling parallel scrapes.
- `searxng` block:
  - `baseURL` – URL for the SearxNG instance.
  - `defaultLimit` – default result limit when request omits `limit`.
  - `timeoutMs` – per-provider-request timeout.

If `search.enabled` is off or misconfigured, `/v1/search` either returns `SEARCH_DISABLED` or `SEARCH_PROVIDER_ERROR`.

---

## 7. LLM

LLM configuration is validated at startup via `Config.Validate()`:

- `defaultProvider` must be one of `openai`, `anthropic`, or `google`.
- The corresponding provider block must have non-empty `apiKey` and `model`.

The `llm` block is used by:
- `/v1/scrape` for `summary`, `branding`, and `json` formats.
- `/v1/extract` for multi-URL structured extraction.

Example “OpenAI-only” setup:

```yaml
llm:
  defaultProvider: "openai"
  openai:
    apiKey: "${OPENAI_API_KEY}"   # set via environment
    baseURL: "https://api.openai.com/v1"
    model: "gpt-4.1-mini"
  anthropic:
    apiKey: ""
    model: ""
  google:
    apiKey: ""
    model: ""
```

In this case, `defaultProvider: openai` is valid because the OpenAI block is fully configured, even though the others are blank.

---

## 8. Example Scenarios

### 8.1 Local development

Goal: run Raito locally with minimal setup.

- Use `deploy/config/config.example.yaml` as a starting point.
- Run Postgres and Redis via Docker Compose:

```bash
cd raito/deploy
docker compose up -d postgres redis
```

- Run the API binary directly:

```bash
cd raito
go run ./cmd/raito-api -config deploy/config/config.example.yaml
```

- Set a simple `initialAdminKey` and use it to create a user key.

### 8.2 Production with external services

Goal: run Raito behind a reverse proxy with managed Postgres/Redis.

- Set `server.host: "0.0.0.0"` and an internal `port` (e.g., 8080).
- Point `database.dsn` to your managed Postgres instance.
- Point `redis.url` to your managed Redis.
- Configure `auth`, `ratelimit`, and `llm` as appropriate.
- Optionally disable `rod.enabled` on API-only nodes and run separate worker nodes with rod enabled.

### 8.3 Search-disabled deployment

If you don’t want `/v1/search`:

```yaml
search:
  enabled: false
  provider: ""
```

Requests to `/v1/search` return `SEARCH_DISABLED`, but all other endpoints continue to function.

---

## 9. Where Config Is Used

- `internal/http` – pulls config from `c.Locals("config")` for each request.
- `internal/services` – receives config in service constructors (scrape, search, extract, etc.).
- `internal/llm` – uses `llm` block to construct provider clients.
- `internal/jobs` and `internal/crawl` – use `worker`, `crawler`, and `retention` for job behavior.

Understanding `config.yaml` is essential whether you are deploying Raito, integrating with its endpoints, or extending its internals.

For deployment specifics (Docker vs local Go), see `docs/deploy.md`. For endpoint-level behavior, see `docs/usage.md` and the per-endpoint docs in this directory.