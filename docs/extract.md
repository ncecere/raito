# `/v1/extract` – Structured JSON Extraction

`/v1/extract` lets you extract structured JSON from one or more web pages using an LLM, guided by a caller-provided JSON-schema-like `schema`. This document targets backend/API developers integrating with Raito.

The endpoint is **asynchronous**:
- `POST /v1/extract` enqueues a job.
- `GET /v1/extract/:id` polls job status and, when completed, returns the job output.

This doc focuses on the request payload and the `job.output` shape used by the worker.

---

## 1. Asynchronous Job Model

- `POST /v1/extract`
  - Validates the request body.
  - Enqueues a job into the jobs table.
  - Returns an `id` and a convenience `url` for polling.
- `GET /v1/extract/:id`
  - Returns the current job status (`pending`, `running`, `completed`, `failed`).
  - When `status == "completed"`, exposes the decoded `job.output` in `data`.
  - When `status == "failed"`, exposes a structured error (`code` and `error` message) derived from the stored job error string.

`job.output` is produced by `runExtractJob` in `internal/http/crawl_worker.go` and is always a JSON object.

---

## 2. Request Payload (`POST /v1/extract`)

The request body is defined by `ExtractRequest` in `internal/http/types.go`.

```jsonc
{
  "urls": ["https://example.com/page1", "https://example.com/page2"],
  "schema": { /* JSON-schema-like structure */ },
  "prompt": "Optional task-specific instructions",
  "systemPrompt": "Optional system-level guardrails",
  "provider": "optional-llm-provider",
  "model": "optional-model-id",
  "strict": false,
  "ignoreInvalidURLs": true,
  "enableWebSearch": false,
  "allowExternalLinks": false,
  "showSources": true,
  "scrapeOptions": {
    "headers": {
      "Accept-Language": "en-US,en;q=0.9"
    },
    "location": {
      "country": "us",
      "languages": ["en"]
    }
  },
  "integration": "my-service-name"
}
```

### 2.1 `urls` (required)

- Type: non-empty array of strings.
- Each entry is a page to scrape and extract from.
- The handler validates:
  - Non-empty, trimmed string.
  - Parseable as a URL with non-empty `scheme` and `host`.
  - `scheme` must be `http` or `https`.
- Invalid URLs cause `POST /v1/extract` to fail with:
  - HTTP 400.
  - `code: "BAD_REQUEST_INVALID_URL"` and an error mentioning the failing index.

### 2.2 `schema` (required)

- Type: JSON object (`map[string]interface{}`) describing the expected JSON structure.
- Conceptually similar to JSON Schema; used only as guidance for the LLM.
- Validation in the handler:
  - Must be a non-empty object.
    - On violation: `code: "INVALID_SCHEMA"`.
  - Top-level key count is capped (coarse complexity guard). If exceeded:
    - `code: "SCHEMA_TOO_COMPLEX"`.
  - If a top-level `"type"` is provided, it must be `"object"` or `"array"`.
    - Other primitive types are rejected as `INVALID_SCHEMA`.
- **Output format:** `/v1/extract` always returns JSON matching (or best approximating) this schema. It never returns HTML/markdown directly.

Internally, `runExtractJob` passes the schema as part of the LLM field description to encourage conformant JSON, but the schema is not enforced as a strict validator yet.

### 2.3 `prompt` and `systemPrompt` (optional)

- `prompt` (string): task-specific instructions (what to extract, domain hints, etc.).
- `systemPrompt` (string): higher-level behavior instructions and guardrails.
- Combined into a single string before sending to the LLM as:

  - If only `prompt` is set: use `prompt`.
  - If only `systemPrompt` is set: use `systemPrompt`.
  - If both are set: `systemPrompt + "\n\n" + prompt`.

This combination is used as the `Prompt` field of the LLM request for every URL.

### 2.4 `provider` and `model` (optional)

- `provider` overrides `llm.defaultProvider` from config.
- `model` overrides the default model for the chosen provider.
- When both are omitted, `llm.defaultProvider` and its configured default model are used.
- At startup, `Config.Validate()` ensures:
  - `llm.defaultProvider` is one of `openai`, `anthropic`, or `google`.
  - The selected provider has both `apiKey` and `model` configured.
- If `runExtractJob` cannot construct an LLM client (e.g., misconfiguration), the job fails with:
  - Stored error starting with `"LLM_NOT_CONFIGURED:"`.
  - `GET /v1/extract/:id` surfaces `code = "LLM_NOT_CONFIGURED"`.

### 2.5 `strict` (optional)

- Type: boolean.
- **Current semantics:** treated as a **hint**, not a hard guarantee.
  - `runExtractJob` currently sets `Strict: false` on the LLM request regardless of this value; extraction is best-effort.
- **Future intent:** may tighten schema adherence and failure behavior when `true`.
- Do not rely on `strict` to cause job-level failures yet.

### 2.6 `ignoreInvalidURLs` (optional)

- Type: boolean.
- Controls whether per-URL failures abort the job or are recorded as per-URL errors:
  - When `true` (recommended for untrusted/heterogeneous URLs):
    - Scrape or LLM failures for a URL are recorded in `results[]` (and `sources[]` when enabled).
    - The job can still complete successfully as long as at least one URL produces JSON.
  - When `false` (or omitted):
    - A scrape or extraction failure for any URL causes the job to be marked failed.
    - The job error string starts with a code such as `SCRAPE_FAILED`, `EXTRACT_FAILED`, or `EXTRACT_EMPTY_RESULT`.

Even when `ignoreInvalidURLs == true`, if **all** URLs fail, the job is failed with `EXTRACT_EMPTY_RESULT: no URLs produced extracted JSON`.

### 2.7 `enableWebSearch` and `allowExternalLinks` (reserved)

- Type: boolean.
- Currently **not wired** in the worker; reserved for future use.
  - `enableWebSearch`: may later allow augmenting scraped content with web search.
  - `allowExternalLinks`: may later allow following links outside the original domains.
- Setting these today has no effect.

### 2.8 `showSources` (optional)

- Type: boolean.
- When `true`:
  - `job.output` includes a `sources[]` array with basic per-URL scrape metadata.
- When `false` or omitted:
  - `sources[]` is omitted; only `results[]` and `summary` are returned.

### 2.9 `scrapeOptions` (optional)

- Type: object; reuses the semantics of `ScrapeOptions` used by `/v1/scrape`, `/v1/crawl`, and `/v1/search`.
- Key subfields:
  - `headers` (object): additional HTTP headers to send when scraping each URL.
  - `location` (object): `{ country, languages[] }` used by `BuildRequestFromOptions` to derive `Accept-Language` and related behavior.
- These options only influence **scraping** (what HTML/markdown is fetched).
- The extract output is always JSON, shaped by `schema` and LLM behavior.

### 2.10 `integration` (optional)

- Type: string.
- Opaque tag for the caller integration (e.g., `"billing-service"`, `"partner-foo"`).
- Logged and propagated internally for observability.
- Not validated or used for authorization.

---

## 3. Job Output (`job.output`)

When an extract job completes successfully, `runExtractJob` stores a JSON object in the job’s `output` column. This is exposed from `GET /v1/extract/:id` under `data`.

### 3.1 Shape

```jsonc
{
  "results": [
    {
      "url": "https://example.com/page1",
      "success": true,
      "json": { /* extracted JSON matching your schema */ }
    },
    {
      "url": "https://example.com/page2",
      "success": false,
      "error": "SCRAPE_FAILED: timeout"
    }
  ],
  "sources": [
    {
      "url": "https://example.com/page1",
      "statusCode": 200,
      "error": ""
    },
    {
      "url": "https://example.com/page2",
      "statusCode": 504,
      "error": "SCRAPE_FAILED: timeout"
    }
  ],
  "summary": {
    "total": 2,
    "success": 1,
    "failed": 1,
    "failedByCode": {
      "SCRAPE_FAILED": 1
    }
  }
}
```

Fields:

- `results[]` (always present)
  - One entry per URL processed.
  - Each entry has:
    - `url` (string): the URL.
    - `success` (bool): whether extraction for this URL succeeded.
    - `json` (object, **present only when `success == true`**): extracted JSON.
    - `error` (string, **present only when `success == false`**): error description starting with an error code (see below).

- `sources[]` (optional; present when `showSources == true` and at least one URL was scraped)
  - One entry per URL processed.
  - Each entry has:
    - `url` (string): the URL.
    - `statusCode` (int): HTTP status code from the scraper (0 if the request failed before receiving a response).
    - `error` (string):
      - `""` when scraping succeeded.
      - A string starting with an error code when scraping failed (e.g., `"SCRAPE_FAILED: timeout"`).

- `summary` (always present on successful jobs)
  - `total` (int): number of entries in `results[]`.
  - `success` (int): number of entries with `success == true`.
  - `failed` (int): `total - success`.
  - `failedByCode` (object, optional): present when there is at least one failure and at least one error code was recorded:
    - Keys are error codes such as `"SCRAPE_FAILED"`, `"EXTRACT_FAILED"`, `"EXTRACT_EMPTY_RESULT"`.
    - Values are counts of URLs that failed with that code.

If all URLs fail (no successful JSON produced), `runExtractJob` marks the job as failed and does **not** persist this payload; instead, the job has an error message like `"EXTRACT_EMPTY_RESULT: no URLs produced extracted JSON"`.

---

## 4. Error Codes and Failure Modes

Errors surface in two places:

1. The job’s `status`/`error` fields (exposed by `GET /v1/extract/:id`).
2. Per-URL `results[].error` and `sources[].error` strings in `job.output`.

Common error codes (prefixes) include:

- `SCRAPE_FAILED`
  - Scraping the URL failed (network errors, timeouts, invalid responses, etc.).
  - Example per-URL errors:
    - `"SCRAPE_FAILED: timeout"`
    - `"SCRAPE_FAILED: 404 Not Found"`.

- `EXTRACT_FAILED`
  - The LLM or extraction pipeline failed for that URL (request error, parsing failure, etc.).
  - Example: `"EXTRACT_FAILED: failed to parse JSON from LLM response"`.

- `EXTRACT_EMPTY_RESULT`
  - The LLM returned no usable fields:
    - Per-URL: `"EXTRACT_EMPTY_RESULT: LLM did not return any fields"`.
    - Job-level (all URLs failed): `"EXTRACT_EMPTY_RESULT: no URLs produced extracted JSON"`.

- `LLM_NOT_CONFIGURED`
  - The configured provider/model is unavailable or misconfigured.
  - Fails the job before any URL is processed:
    - Stored error: `"LLM_NOT_CONFIGURED: <details>"`.
    - `GET /v1/extract/:id` exposes `code = "LLM_NOT_CONFIGURED"`.

All error strings are constructed to be safe and not include raw API keys or provider URLs. They are intended to be parseable by taking the prefix up to the first `:` as a machine-friendly code.

---

## 5. Usage Notes & Best Practices

- Keep your `schema` as simple and targeted as possible; large or deeply nested schemas can be harder for the model to satisfy.
- Use `prompt` for per-call instructions and `systemPrompt` for stable, reusable behavior (house style, safety, etc.).
- Prefer `ignoreInvalidURLs = true` when you expect some URLs to be broken or flaky; rely on `results[]` and `summary.failedByCode` to understand partial failure patterns.
- Enable `showSources = true` when debugging scraping issues or monitoring HTTP status behavior; disable it in high-volume production flows if you don’t need per-URL status to keep payloads smaller.
- Treat `strict`, `enableWebSearch`, and `allowExternalLinks` as forward-looking knobs; today they do not tighten behavior beyond what’s described above.
