# Changelog

All notable changes to this project will be documented in this file.

## v0.3.0 – 2025-12-14

### Added

- User authentication stack:
  - Local email/password login with bcrypt, OIDC login/callback, and JWT-based session cookies.
  - `Principal` abstraction and auth middleware supporting API keys or sessions.
- Multi-tenancy and tenant-scoped APIs:
  - `users`, `tenants`, `tenant_members`, and tenant-aware `jobs.tenant_id` schema/migrations.
  - `/v1/tenants`, `/v1/tenants/:id/select`, tenant membership admin endpoints, tenant usage endpoint, and tenant-scoped API key endpoints.
  - `/v1/jobs` and `/v1/jobs/:id` tenant-aware listings for UI clients and admins.
- Documentation:
  - `docs/auth.md` – local/OIDC auth and session flows.
  - `docs/multi-tenancy.md` – tenants, roles, tenant-scoped keys, usage.
  - `docs/auth-migration.md` – rollout/migration guide for enabling auth and multi-tenancy in existing deployments.
- Tests:
  - Session issuance/parsing and `authMiddleware` Principal construction tests.
  - HTTP tests covering tenant usage, tenant membership, tenant API key validation, `/v1/jobs` auth/validation, and `/auth/session` behavior.
- Deploy configs:
  - Multiple example configs in `deploy/config/` for API-key-only, local-auth, and OIDC-auth setups.

### Changed

- Enforced tenant-scoped access control on job status endpoints and `/v1/jobs`/`/v1/jobs/:id`, with system admins retaining global access.
- Updated routing to expose `/auth/session` and tenant APIs behind shared auth and rate-limit middleware, aligned with the new Principal model.
- Updated docs overview to link to the new auth/multi-tenancy and migration docs, and clarified recommended rollout patterns for operators.

### Fixed / Cleaned Up

- Hardened auth-related error handling and validation paths (e.g., invalid IDs, missing principals) in new tenant and key management endpoints.
- Ensured new auth and tenant features integrate with existing metrics, logging, and configuration validation without breaking existing API-key-only deployments.

## v0.2.0 – 2025-12-14

### Added

- **Endpoint documentation** under `raito/docs/`:
  - `scrape.md` – detailed `/v1/scrape` request/response contract, formats, and curl examples.
  - `map.md` – `/v1/map` URL discovery parameters, sitemap modes, and usage patterns.
  - `crawl.md` – `/v1/crawl` job model, status responses, and crawl→extract workflows.
  - `batch-scrape.md` – `/v1/batch/scrape` batch job semantics and limits.
  - `search.md` – `/v1/search` provider config, search-only vs search+scrape behavior, and error codes.
  - `extract.md` – `/v1/extract` asynchronous extraction contract, schema validation, per-URL results, sources, and summary.
  - `config.md` – explanation of `config.yaml` sections, defaults, and deployment scenarios.
  - `overview.md` – documentation entry point describing architecture, audiences, and how docs are organized.
- **Metrics tests** in `internal/metrics/metrics_test.go` for HTTP, search, and extract metric families.
- **Scrape utility tests** in `internal/scrapeutil/helpers_test.go` for `ToString` and `FilterLinks` behavior.
- **Extract worker tests** in `internal/http/crawl_worker_test.go` covering:
  - Single URL success.
  - Mixed success/failure with `ignoreInvalidURLs` on/off.
  - `showSources` behavior and `summary.failedByCode` aggregation.
  - LLM error and empty-result handling (`EXTRACT_FAILED`, `EXTRACT_EMPTY_RESULT`).
- **Search service tests** in `internal/services/search_test.go` exercising `ScrapeResults` and `ignoreInvalidURLs` semantics.
- **Formats tests** in `internal/formats/formats_test.go` validating `/v1/search` format restrictions.
- **Makefile** under `raito/` with `build` and `test` shortcuts for `go build ./...` and `go test ./...`.

### Changed

- Hardened `/v1/extract`:
  - Enforced `urls[]` + `schema` as required fields and added early URL/schema validation with clear error codes (`BAD_REQUEST_INVALID_URL`, `INVALID_SCHEMA`, `SCHEMA_TOO_COMPLEX`).
  - Implemented per-URL scraping and LLM extraction with `results[]`, optional `sources[]`, and `summary` (`total`, `success`, `failed`, `failedByCode`).
  - Clarified `strict`, `ignoreInvalidURLs`, `enableWebSearch`, `allowExternalLinks`, and `showSources` semantics.
- Updated `/v1/search`:
  - Restricted scraped formats to `markdown`, `html`, `rawHtml` for `scrapeOptions.formats` and return `UNSUPPORTED_FORMAT` for others.
  - Standardized `ignoreInvalidURLs` behavior between search and extract.
- Centralized format handling in `internal/formats` and ScrapeOptions behavior in `internal/scraper`/`internal/services`.
- Introduced `jobStore` and `extractDeps` abstractions to make `runExtractJob` testable via injected store/scraper/LLM.
- Updated various structs and helpers to use `any` instead of `interface{}` where appropriate for clarity and modern Go style.

### Fixed / Cleaned Up

- Removed the legacy `scrapeURLForSearch` helper in `internal/http/handlers.go` in favor of the normalized scrape/search services.
- Normalized extract, crawl, and batch-scrape job status handling and logging (`*_enqueued`, `*_completed`, `*_failed` events).
- Ensured `config.Validate()` performs LLM configuration checks at startup so misconfigurations fail fast (`LLM_NOT_CONFIGURED`).

---

Older changes prior to v0.2.0 were not tracked in this file.
