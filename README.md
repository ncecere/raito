# Raito

Raito is a Go-based web scraping and crawling API inspired by Firecrawl. It exposes a `/v1` HTTP API for scraping single pages, mapping domains, running crawl jobs, and extracting structured data with LLMs.

This document explains how to set up, run, and use the Raito API locally.

## Prerequisites

- Go 1.22+ installed
- Docker + Docker Compose (for Postgres and Redis)
- Network access to the target sites you want to scrape

## Project layout

Inside the `raito/` directory:

- `cmd/raito-api/` – main API server entrypoint
- `config/config.yaml` – application configuration
- `db/migrations/` – goose migrations for Postgres
- `db/queries/` – sqlc query definitions
- `internal/config/` – config loading
- `internal/http/` – Fiber router, handlers, middleware
- `internal/scraper/` – HTTP and rod-based scrapers
- `internal/crawler/` – site mapping logic
- `internal/store/` – DB store using sqlc models
- `internal/llm/` – LLM abstraction (OpenAI, Anthropic, Google)
- `internal/metrics/` – simple Prometheus-style request/LLM metrics

## Configuration

Raito is configured via `config/config.yaml`. The default file looks roughly like this:

```yaml
server:
  host: "0.0.0.0"
  port: 8080

scraper:
  userAgent: "RaitoBot/1.0"
  timeoutMs: 30000

crawler:
  maxDepthDefault: 3
  maxPagesDefault: 100

robots:
  respect: true

rod:
  enabled: true
  browserURL: "" # e.g. ws://127.0.0.1:9222

database:
  dsn: "postgres://raito:raito@localhost:5432/raito?sslmode=disable"

redis:
  url: "redis://localhost:6379"

auth:
  enabled: true
  initialAdminKey: "raito_admin_dev_key"

ratelimit:
  defaultPerMinute: 60

llm:
  defaultProvider: "openai" # or anthropic, google
  openai:
    apiKey: ""
    baseURL: "https://api.openai.com/v1"
    model: "gpt-4.1-mini"
  anthropic:
    apiKey: ""
    model: "claude-3-5-sonnet-20241022"
  google:
    apiKey: ""
    model: "gemini-1.5-flash"
```

Notes:

- **Database** – the `dsn` must point to a Postgres instance with the `raito` database.
- **Redis** – used for per-key rate limiting when `auth.enabled` is true.
- **Auth** – `initialAdminKey` is hashed and stored as an admin API key on startup.
- **rod** – if enabled and `browserURL` is set, `/v1/scrape` can use a headless browser via rod when `useBrowser=true`.
- **llm** – configure API keys and default models for OpenAI, Anthropic, and Google (Gemini). The `baseURL` for OpenAI lets you use OpenAI-compatible APIs.

### Config examples

**OpenAI only (default provider)**

```yaml
llm:
  defaultProvider: "openai"
  openai:
    apiKey: "${OPENAI_API_KEY}"   # set via environment or secrets
    baseURL: "https://api.openai.com/v1"
    model: "gpt-4.1-mini"
  anthropic:
    apiKey: ""
    model: ""
  google:
    apiKey: ""
    model: ""
```

**Anthropic as default provider**

```yaml
llm:
  defaultProvider: "anthropic"
  openai:
    apiKey: ""
    baseURL: "https://api.openai.com/v1"
    model: "gpt-4.1-mini"
  anthropic:
    apiKey: "${ANTHROPIC_API_KEY}"
    model: "claude-3-5-sonnet-20241022"
  google:
    apiKey: ""
    model: ""
```

**OpenAI-compatible base URL (self-hosted/proxy)**

```yaml
llm:
  defaultProvider: "openai"
  openai:
    apiKey: "${OPENAI_COMPAT_API_KEY}"
    baseURL: "https://my-openai-compatible.example.com/v1"
    model: "gpt-4.1-mini"
  anthropic:
    apiKey: ""
    model: ""
  google:
    apiKey: ""
    model: ""
```

Currently, configuration is loaded directly from YAML; environment variable overrides are not yet implemented.

## Running Postgres and Redis

From the `raito/` directory you can start Postgres and Redis with the provided `docker-compose.yaml`:

```bash
cd raito
docker compose up -d postgres redis
```

This will:

- Start Postgres on port 5432 with a `raito` database and `raito` user.
- Start Redis on port 6379.

Ensure the `database.dsn` and `redis.url` in `config/config.yaml` match these settings.

### docker-compose.yaml example

The repo includes a minimal `docker-compose.yaml` for local Postgres and Redis:

```yaml
version: "3.9"

services:
  postgres:
    image: postgres:16
    container_name: raito-postgres
    restart: unless-stopped
    environment:
      POSTGRES_DB: raito
      POSTGRES_USER: raito
      POSTGRES_PASSWORD: raito
    ports:
      - "5432:5432"
    volumes:
      - raito-postgres-data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    container_name: raito-redis
    restart: unless-stopped
    ports:
      - "6379:6379"
    command: ["redis-server", "--appendonly", "yes"]
    volumes:
      - raito-redis-data:/data

volumes:
  raito-postgres-data:
  raito-redis-data:
```

You can customize this file for your environment (different ports, passwords, etc.).

## Running the API server

With Postgres and Redis running:

```bash
cd raito
go run ./cmd/raito-api
```

On startup, the server will:

1. Run goose migrations from `db/migrations/`.
2. Initialize the Store and ensure an admin API key using `auth.initialAdminKey`.
3. Start a background crawl worker that processes pending crawl jobs from the `jobs` table.
4. Start a Fiber HTTP server on `server.host:server.port` (by default `0.0.0.0:8080`).

## Authentication & API keys

All `/v1` and `/admin` endpoints require an API key when `auth.enabled=true`.

- Admin key: taken from `auth.initialAdminKey` (e.g. `raito_admin_dev_key`).
- Authorization header: `Authorization: Bearer <key>`.
- Rate limiting: per-key per-minute limits enforced via Redis.

To create a user API key with the admin key:

```bash
curl -X POST http://localhost:8080/admin/api-keys \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer raito_admin_dev_key' \
  -d '{"label": "example-user-key", "rateLimitPerMinute": 60}'
```

The response will include a `key` value like `raito_...` that you use in `Authorization` for `/v1/*` endpoints.

## Endpoints overview

All non-health endpoints live under `/v1` (plus `/admin` for API key management).

### Health

- `GET /healthz` – basic health check.

### Scrape

- `POST /v1/scrape`

Request (minimal):

```json
{
  "url": "https://example.com"
}
```

Optional fields:

- `useBrowser`: `true` to use rod/JS rendering (when enabled in config).
- `headers`: custom request headers.
- `timeout`: per-request timeout override (ms).

Example:

```bash
curl -X POST http://localhost:8080/v1/scrape \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <user-key>' \
  -d '{"url": "https://example.com", "useBrowser": false}'
```

Response contains a `data` document with:

- `markdown`, `html`, `rawHtml`
- `links` (absolute URLs)
- `engine`: `"http"` or `"browser"`
- `metadata`: title, description, OG tags, language, status code, etc.

### Map

- `POST /v1/map`

Discovers URLs for a domain via sitemap and HTML link discovery.

Example:

```bash
curl -X POST http://localhost:8080/v1/map \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <user-key>' \
  -d '{"url": "https://example.com", "allowExternalLinks": true, "limit": 10}'
```

Returns a `links` array of `{ url, title, description }` and optional `warning`.

### Crawl

- `POST /v1/crawl` – create a crawl job
- `GET /v1/crawl/:id` – get job status + documents

Example:

```bash
# Create a crawl job
curl -X POST http://localhost:8080/v1/crawl \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <user-key>' \
  -d '{"url": "https://example.com", "limit": 5}'

# Later, fetch status and results
curl http://localhost:8080/v1/crawl/<id> \
  -H 'Authorization: Bearer <user-key>'
```

Crawl jobs are stored in the `jobs` and `documents` tables. A background worker polls pending jobs, runs discovery with `internal/crawler.Map`, scrapes pages via the HTTP scraper, and stores documents.

### Extract (LLM-based)

- `POST /v1/extract`

Extracts structured fields from a URL using an LLM.

Example request:

```bash
curl -X POST http://localhost:8080/v1/extract \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <user-key>' \
  -d '{
    "url": "https://example.com/product/123",
    "fields": [
      { "name": "title", "description": "Product name" },
      { "name": "price", "description": "Price in USD", "type": "number" },
      { "name": "inStock", "description": "In stock?", "type": "boolean" }
    ],
    "provider": "openai",
    "model": "gpt-4.1-mini",
    "strict": true
  }'
```

Behavior:

1. Raito scrapes the URL using the HTTP scraper.
2. It calls the configured LLM provider (OpenAI, Anthropic, or Google Gemini) using the model from the request or config.
3. It prompts the model to return a single JSON object with the requested fields.
4. In `strict=false` (default):
   - Raito tries hard to parse JSON from the LLM response.
   - If parsing fails, it falls back to `{ "_raw": "..." }` in `fields`.
5. In `strict=true`:
   - If JSON can’t be parsed, the request fails with `EXTRACT_FAILED`.
   - If not all requested field names are present, it fails with `EXTRACT_INCOMPLETE_FIELDS`.

The response includes:

- `data[0].fields`: structured fields
- `data[0].raw`: the raw scraped document (`markdown`, `html`, `rawHtml`, `links`, `engine`, `metadata`).

### Metrics

- `GET /metrics`

Exposes Prometheus-style metrics including:

- HTTP requests:

  ```text
  raito_http_requests_total{method="GET",path="/v1/scrape",status="200"} 42
  raito_http_request_duration_ms_sum{method="GET",path="/v1/scrape"} 12345
  raito_http_request_duration_ms_count{method="GET",path="/v1/scrape"} 42
  ```

- LLM extract usage:

  ```text
  raito_llm_extract_requests_total{provider="openai",model="gpt-4.1-mini",success="true"} 15
  raito_llm_extract_requests_total{provider="anthropic",model="claude-3-5-sonnet-20241022",success="false"} 3
  ```

You can point Prometheus at `http://<host>:8080/metrics` to scrape these.

## Logging

Raito uses `log/slog` for structured request logging. The logging middleware records per-request entries such as:

- `request_id`
- `method`, `path`, `status`
- `latency_ms`
- For `/v1/extract`: `llm_provider` and `llm_model`

You can extend this by adding more context in handlers (via `c.Locals`) if needed.

## Next steps

For local development, the typical workflow is:

1. Start Postgres and Redis via Docker Compose.
2. Configure `config/config.yaml` for DB, Redis, auth, rod, and LLMs.
3. Run the API server with `go run ./cmd/raito-api`.
4. Create a user API key via `/admin/api-keys`.
5. Call `/v1/scrape`, `/v1/map`, `/v1/crawl`, and `/v1/extract` with your user key.
6. Monitor `/healthz` and `/metrics`, and tail logs for insight into request behavior and LLM usage.
