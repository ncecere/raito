# Logging & Job Events

Raito emits structured logs using Go's `slog` package. Logs are designed to be machine-friendly and easy to filter in systems like Loki, Elasticsearch, or Cloud Logging.

This document focuses on:

- Request-level logs produced by the HTTP middleware.
- Job-level logs for crawl, batch scrape, extract, scrape (queued), and map (queued).

---

## Request logs

Every HTTP request passes through a logging and metrics middleware in `internal/http/router.go`.

- **Event name**: `request`
- **Fields**:
  - `request_id` – unique request ID (from `X-Request-Id` header or generated UUID).
  - `method` – HTTP method (e.g. `GET`, `POST`).
  - `path` – request path (e.g. `/v1/scrape`).
  - `status` – HTTP status code.
  - `latency_ms` – request latency in milliseconds.
  - `llm_provider` (optional) – provider used by LLM-based features, when present.
  - `llm_model` (optional) – model name used for LLM-based features.

These logs are emitted regardless of which endpoint is called and provide a consistent view of API traffic.

---

## Extract job logs

Extract jobs are created via `POST /v1/extract` and processed asynchronously by workers.

### `extract_enqueued`

Emitted in `extractHandler` when a new extract job is enqueued.

- **Event name**: `extract_enqueued`
- **Fields**:
  - `extract_id` – job UUID.
  - `primary_url` – first URL in the `urls` array.
  - `urls_count` – length of `urls`.
  - `provider` – requested LLM provider override (may be empty).
  - `model` – requested LLM model override (may be empty).
  - `ignore_invalid_urls` – raw value of `ignoreInvalidURLs` option.
  - `show_sources` – raw value of `showSources` option.

Additional extract metrics (`raito_extract_jobs_total`, `raito_extract_results_total`) are exported separately via `/metrics`.

---

## Crawl job logs

Crawl jobs are created via `POST /v1/crawl` and executed by the worker.

### `crawl_enqueued`

Emitted in `crawlHandler` when a new crawl job is enqueued.

- **Event name**: `crawl_enqueued`
- **Fields**:
  - `crawl_id` – job UUID.
  - `url` – crawl starting URL.
  - `limit` – effective page limit (request override or `crawler.maxPagesDefault`).
  - `allow_external_links` – raw value from the request body.
  - `allow_subdomains` – raw value from the request body.

### `crawl_completed` / `crawl_failed`

Emitted in `crawlStatusHandler` whenever a job status is retrieved.

- **Event name**:
  - `crawl_completed` when `status == "completed"`.
  - `crawl_failed` for all other statuses (e.g. `failed`, `pending`, `running`).
- **Fields**:
  - `crawl_id` – job UUID.
  - `status` – raw job status string from the DB.
  - `total_documents` – number of documents associated with the job.
  - `error` (optional) – job error message when present.

These logs help correlate crawl activity with job outcomes and document counts.

---

## Batch scrape job logs

Batch scrape jobs are created via `POST /v1/batch/scrape` and executed by the worker.

### `batch_scrape_enqueued`

Emitted in `batchScrapeHandler` when a new batch scrape job is enqueued.

- **Event name**: `batch_scrape_enqueued`
- **Fields**:
  - `batch_scrape_id` – job UUID.
  - `primary_url` – first URL in `urls`.
  - `urls_count` – number of URLs in the batch.
  - `has_formats` – whether any formats were requested.

### `batch_scrape_completed` / `batch_scrape_failed`

Emitted in `batchScrapeStatusHandler` when job status is fetched.

- **Event name**:
  - `batch_scrape_completed` when `status == "completed"`.
  - `batch_scrape_failed` otherwise.
- **Fields**:
  - `batch_scrape_id` – job UUID.
  - `status` – raw job status.
  - `total_documents` – number of documents associated with the job.
  - `error` (optional) – job error string when present.

---

## Queued scrape job logs (JobQueueExecutor)

When `NewServer` is constructed, it creates a `JobQueueExecutor` used by `/v1/scrape` (and `/v1/map`) when the executor is available. This executor enqueues jobs and polls them synchronously. It uses the same logger instance as the HTTP server.

### `scrape_enqueued`

Emitted inside `JobQueueExecutor.Scrape` after a scrape job is created.

- **Event name**: `scrape_enqueued`
- **Fields**:
  - `scrape_id` – job UUID.
  - `url` – URL to be scraped.
  - `has_formats` – whether any formats were requested in the original `ScrapeRequest`.

### `scrape_completed`

Emitted when the job reaches `completed` status and the output document is successfully decoded.

- **Event name**: `scrape_completed`
- **Fields**:
  - `scrape_id` – job UUID.
  - `status` – job status (`"completed"`).

### `scrape_failed`

Emitted for scrape jobs that fail or time out.

- **Event name**: `scrape_failed`
- **Fields**:
  - `scrape_id` – job UUID.
  - `status` – last known job status (`pending`, `running`, or `failed`).
  - `code` – error code such as `JOB_NOT_STARTED`, `SCRAPE_TIMEOUT`, or `SCRAPE_FAILED`.
  - `error` – human-readable error message (for example, the DB error or worker-side failure text).

These events make it easy to distinguish between enqueue failures, queue timeouts, and worker-level errors for `/v1/scrape` when using the job-queue-backed path.

---

## Queued map job logs (JobQueueExecutor)

Similarly, the `JobQueueExecutor.Map` method emits log events when `/v1/map` is routed through the job queue.

### `map_enqueued`

Emitted after a map job is created.

- **Event name**: `map_enqueued`
- **Fields**:
  - `map_id` – job UUID.
  - `url` – starting URL for the map operation.
  - `limit` – effective link limit applied by the executor.

### `map_completed`

Emitted when the map job completes and the output is decoded.

- **Event name**: `map_completed`
- **Fields**:
  - `map_id` – job UUID.
  - `status` – job status (`"completed"`).

### `map_failed`

Emitted when a map job fails or times out.

- **Event name**: `map_failed`
- **Fields**:
  - `map_id` – job UUID.
  - `status` – last known job status.
  - `code` – error code such as `JOB_NOT_STARTED`, `MAP_TIMEOUT`, or `MAP_FAILED`.
  - `error` – human-readable error message.

---

## How to use these logs

- **Correlate with metrics**: combine `request` logs with Prometheus metrics from `/metrics` to understand traffic and performance.
- **Trace job lifecycles**: follow `*_enqueued`, `*_completed`, and `*_failed` events by job ID to understand job outcomes and timings.
- **Filter by event name**: in most log backends you can filter by the `msg`/`event` field (e.g., `scrape_failed`) and then narrow down by attributes like `url` or `status`.

For deployment and configuration details (including how the logger is wired), see `docs/deploy.md`. For endpoint-level behavior, see `docs/usage.md`.