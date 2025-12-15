# Raito Documentation Overview

Raito is a Firecrawl-style HTTP API written in Go that provides scraping, mapping, crawling, search, and extraction endpoints.

This overview is the entry point for three main audiences:
- **Deployers / operators** – running the service in staging/production.
- **API users / integrators** – calling the HTTP endpoints from other systems.
- **Raito developers** – working on the internals (handlers, services, workers).

---

## 1. High-Level Architecture

At a high level, Raito is structured as:

- `cmd/raito-api` – the main binary; can run in `api` or `worker` roles.
- `internal/http` – HTTP layer (routing, handlers, request/response DTOs, middleware).
- `internal/services` – feature services for `scrape`, `map`, `crawl`, `batch scrape`, `search`, and `extract`.
- `internal/scraper` – HTTP and browser (rod) scrapers.
- `internal/search` – search providers (currently SearxNG) and request/response models.
- `internal/llm` – LLM integration (OpenAI, Anthropic, Google) for summaries, branding, and JSON extraction.
- `internal/jobs` – job runner and retention logic for crawl, batch, map, and extract jobs.
- `internal/metrics`, `internal/logging` (via `slog`) – observability.
- `internal/config` – YAML-based configuration loaded at startup.

For more detail, see `PLAN.md` in the repo root.

---

## 2. Documentation Map

### 2.1 For Deployers

Start with:

- `docs/deploy.md` – how to run Raito:
  - Docker Compose stack (Postgres, Redis, SearxNG, API, workers).
  - Running via the published Docker image.
  - Running the Go binary locally while using the same Postgres/Redis stack.
- `docs/web-ui.md` – the dashboard UI:
  - Local dev workflow (`bun run dev`) with API proxying.
  - Production embedded UI (Go `embedwebui` build tag and Docker builds).
- `docs/config.md` – how to configure Raito:
  - `config.yaml` structure (`server`, `scraper`, `crawler`, `robots`, `rod`, `database`, `redis`, `auth`, `ratelimit`, `worker`, `search`, `retention`, `llm`).
  - Defaults and example production/local setups.
- `docs/auth-migration.md` – rollout/migration guide for enabling user auth and multi-tenancy in existing deployments.
- `docs/logging.md` – structured logging:
  - `request` logs for all HTTP traffic.
  - Job-level events (`*_enqueued`, `*_completed`, `*_failed`) for crawl, batch-scrape, extract, and queued scrape/map.
- Root `README.md` – quick curl examples and sanity checks once the server is running.

### 2.2 For API Users / Integrators

Use the per-endpoint docs to understand request/response shapes, parameters, and error codes:

- `docs/usage.md` – high-level overview of all public endpoints and authentication.
- `docs/multi-tenancy.md` – how auth, tenants, roles, and tenant-scoped API keys/usage work.
- `docs/scrape.md` – `/v1/scrape` single-page scraping:
  - Formats (`markdown`, `html`, `rawHtml`, `links`, `images`, `summary`, `branding`, `screenshot`, `json`).
  - Engine selection (`useBrowser`, screenshots).
  - Example scrapes and error handling.
- `docs/map.md` – `/v1/map` URL discovery:
  - Limits, subdomain/external link handling, sitemap modes.
  - How to generate link sets to feed into crawl or batch-scrape.
- `docs/crawl.md` – `/v1/crawl` crawling:
  - Job model, status polling, document formats.
  - Example crawl→extract workflows.
- `docs/batch-scrape.md` – `/v1/batch/scrape`:
  - Batch job creation, limits, and result retrieval.
- `docs/search.md` – `/v1/search`:
  - Search-only vs search+scrape modes.
  - Provider configuration and format restrictions.
- `docs/extract.md` – `/v1/extract`:
  - Async multi-URL extraction using a JSON-schema-like `schema`.
  - `ignoreInvalidURLs`, `showSources`, `summary` block, and error codes.

### 2.3 For Raito Developers

In addition to the docs above, developers should read:

- `PLAN.md` – refactor/architecture plan and rationale.
- `TASKS.md` – completed and remaining work items across HTTP, jobs, formats, extract, search, metrics, and docs.
- `internal/README` (if present) or package comments in:
  - `internal/http` – how handlers map to services and DTOs.
  - `internal/services` – where business logic for each feature lives.
  - `internal/jobs` – job runner, retention, and how jobs are dispatched.
  - `internal/search` – `Provider` interface and SearxNG implementation (see also `internal/search/README.providers.md`).

When updating behavior:
- Keep the per-endpoint docs (`docs/*.md`) in sync with request/response changes.
- Update `docs/config.md` when adding or changing config options.
- Consider adding or updating tests in `internal/http`, `internal/services`, `internal/formats`, `internal/metrics`, or `internal/scrapeutil` to cover new scenarios.

---

## 3. Typical Workflows

### 3.1 Deploy and Smoke-Test

1. Follow `docs/deploy.md` to bring up the stack.
2. Use `docs/config.md` to verify/configure `auth.initialAdminKey`, database, Redis, search, and LLM providers.
3. Generate an API key via `/admin/api-keys` as described in `docs/usage.md`.
4. Run the sample curl commands from `README.md` and the per-endpoint docs to verify scrape/search/extract.

### 3.2 Scrape + Extract Workflow

1. Use `/v1/scrape` (`docs/scrape.md`) to pull content and/or JSON for a single page.
2. For multiple pages with a shared schema, use `/v1/extract` (`docs/extract.md`) with `urls[]` + `schema`.
3. Inspect `results[]`, `sources[]`, and `summary` to understand per-URL success/failure and error codes.

### 3.3 Crawl a Site and Search Within It

1. Optionally use `/v1/map` (`docs/map.md`) to enumerate URLs.
2. Kick off `/v1/crawl` (`docs/crawl.md`) to store normalized documents.
3. Use `/v1/search` (`docs/search.md`) against the configured provider, or build your own search over the stored documents.

---

## 4. Getting Help / Contributing

- Check `CHANGELOG.md` at the repo root for notable changes and version history.
- Open issues or PRs referencing the relevant docs (`docs/*.md`) and code packages.
- When proposing behavior changes, update both the tests and the documentation so operators and integrators stay in sync with the API.
