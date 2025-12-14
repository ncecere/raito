# `/v1/batch/scrape` – Batch Scraping Jobs

`/v1/batch/scrape` lets you submit a set of URLs as a job and retrieve all scraped documents once the job completes. It is useful when you already know which URLs you want and want the work done in the background.

This doc targets:
- **Deployers** – to understand batch load patterns.
- **API users** – to run batch scrapes and retrieve the results.
- **Developers** – to see how batch jobs relate to crawl jobs.

---

## 1. Enqueue Request (`POST /v1/batch/scrape`)

Request body (`BatchScrapeRequest` from `internal/http/types.go`):

```jsonc
{
  "urls": [
    "https://example.com/a",
    "https://example.com/b"
  ],
  "formats": ["markdown", "summary"],   // optional
  "scrapeOptions": {                      // optional
    "headers": {
      "Accept-Language": "en-US,en;q=0.9"
    },
    "location": {
      "country": "us",
      "languages": ["en-US", "en"]
    },
    "useBrowser": true
  }
}
```

Rules and limits:

- `urls[]` (required)
  - Must be a non-empty array of URLs.
  - A soft upper bound of 1000 URLs is enforced by the handler.

- `formats` (optional)
  - Same semantics as `/v1/scrape` formats: `markdown`, `html`, `rawHtml`, `links`, `images`, `summary`, `branding`, `screenshot`, and `json` (via format objects).

- `scrapeOptions` (optional)
  - Same semantics as for `/v1/search` and `/v1/scrape`: control headers, location, and browser usage.

On success (`200 OK`):

```jsonc
{
  "success": true,
  "id": "<uuid>",
  "url": "http://localhost:8080/v1/batch/scrape/<uuid>"
}
```

The job is enqueued and processed by worker processes.

---

## 2. Status Request (`GET /v1/batch/scrape/:id`)

Example:

```bash
curl http://localhost:8080/v1/batch/scrape/<id> \
  -H 'Authorization: Bearer <api-key>'
```

Responses mirror the crawl status shape, but without summary/JSON added by default:

### 2.1 Not found

```jsonc
{
  "success": false,
  "code": "NOT_FOUND",
  "error": "batch scrape job not found"
}
```

### 2.2 Pending/running/failed

```jsonc
{
  "success": true,
  "id": "<uuid>",
  "status": "pending" | "running" | "failed",
  "total": 0,
  "error": "optional job-level error string"
}
```

### 2.3 Completed with documents

```jsonc
{
  "success": true,
  "id": "<uuid>",
  "status": "completed",
  "total": 2,
  "data": [
    {
      "markdown": "...",
      "html": "...",
      "rawHtml": "...",
      "links": ["..."],
      "images": ["..."],
      "engine": "http" | "browser",
      "metadata": { ... }
    },
    {
      "markdown": "...",
      "metadata": { ... }
    }
  ]
}
```

Documents are built via `JobDocumentService.BuildDocuments` using the `formats` from the original `BatchScrapeRequest`. Unlike `/v1/crawl`, batch-scrape status does **not** force summary/JSON; only the requested formats are materialized.

---

## 3. Error Codes

Error envelope on enqueue:

```jsonc
{
  "success": false,
  "code": "BAD_REQUEST" | "BAD_REQUEST_INVALID_JSON" | "BATCH_SCRAPE_JOB_CREATE_FAILED",
  "error": "..."
}
```

Error envelope on status:

```jsonc
{
  "success": false,
  "code": "BAD_REQUEST" | "NOT_FOUND" | "BATCH_SCRAPE_JOB_LOOKUP_FAILED",
  "error": "..."
}
```

HTTP status codes:

- `400` – invalid JSON, missing `urls`, too many URLs, invalid job ID.
- `404` – job not found.
- `500` – job creation/lookup failures.

---

## 4. Example Scenarios

### 4.1 Batch scrape a marketing site

```bash
curl -X POST http://localhost:8080/v1/batch/scrape \
  -H 'Authorization: Bearer <api-key>' \
  -H 'Content-Type: application/json' \
  -d '{
    "urls": [
      "https://example.com",
      "https://example.com/about",
      "https://example.com/pricing"
    ],
    "formats": ["markdown", "summary"]
  }'
```

Later:

```bash
curl http://localhost:8080/v1/batch/scrape/<id> \
  -H 'Authorization: Bearer <api-key>'
```

### 4.2 Batch scrape with custom headers and location

```bash
curl -X POST http://localhost:8080/v1/batch/scrape \
  -H 'Authorization: Bearer <api-key>' \
  -H 'Content-Type: application/json' \
  -d '{
    "urls": ["https://example.com","https://example.co.uk"],
    "formats": ["markdown"],
    "scrapeOptions": {
      "headers": {
        "X-Client": "batch-scrape-example"
      },
      "location": {
        "country": "gb",
        "languages": ["en-GB", "en"]
      }
    }
  }'
```

---

## 5. Operational Notes

- **Workers**: batch jobs are executed by worker processes; ensure at least one worker is running.
- **Limits**: the hard cap of 1000 URLs per job is there to prevent accidental overload. For very large sets, consider multiple batch jobs.
- **Retention**: job and document retention is controlled by the `retention` block in `config.yaml`.
