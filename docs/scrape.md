# `/v1/scrape` – Single-Page Scraping

`/v1/scrape` fetches a single URL and returns a Firecrawl-style `Document` with one or more formats: markdown, HTML, links, images, summary, JSON, screenshots, etc.

This doc is aimed at:
- **Deployers** – to understand resource impact (browser/LLM usage).
- **API users** – to know which parameters to send.
- **Developers** – to see how the HTTP layer, scraper, and services interact.

---

## 1. Request Shape

The JSON body is defined by `ScrapeRequest` in `internal/http/types.go`.

```jsonc
{
  "url": "https://example.com",          // required
  "formats": ["markdown", "summary"],   // optional
  "timeout": 30000,                       // optional (ms)
  "useBrowser": true,                     // optional
  "headers": {                            // optional
    "Accept-Language": "en-US,en;q=0.9"
  },
  "location": {                           // optional
    "country": "us",
    "languages": ["en-US", "en"]
  }
}
```

### 1.1 Required fields

- `url` (string, required)
  - The page to scrape.
  - Must be a valid URL (scheme + host).

### 1.2 Timeouts and engine selection

- `timeout` (number, optional)
  - Per-request timeout in milliseconds for scraping and any optional LLM calls.
  - If omitted, falls back to `scraper.timeoutMs` from config (default 30000ms in `config.example.yaml`).

- `useBrowser` (bool, optional)
  - When `true` and `rod.enabled == true`, uses a headless Chromium browser via `RodScraper`.
  - When `false` or omitted, uses the HTTP-only scraper.
  - **Note:** Certain formats implicitly enable the browser:
    - If `formats` includes `"screenshot"`, the browser engine is used even if `useBrowser` is not set.

### 1.3 Headers and location

- `headers` (object, optional)
  - Extra HTTP headers to send when scraping `url`.
  - Useful for custom `User-Agent` or application-specific headers.

- `location` (object, optional)
  - `{ country, languages[] }` influences `Accept-Language` and link filtering:
    - `languages` → joined as `Accept-Language` if non-empty.
    - `country` → used as a fallback `Accept-Language` when `languages` is empty.

The final request sent to the underlying scraper is built via `scraper.BuildRequestFromOptions` using:
- `url`, `headers`, `timeoutMs` (from request or config), `userAgent` (from config), and `location`.

### 1.4 Formats

`formats` is an array of strings and/or objects that controls which fields are materialized on the returned document. Supported string formats include:

- `"markdown"` – cleaned, readable text.
- `"html"` – sanitized HTML.
- `"rawHtml"` – raw HTML as received.
- `"links"` – outbound links after filtering Duplicates and domain rules.
- `"images"` – image URLs.
- `"summary"` – LLM-generated summary (requires LLM config).
- `"branding"` – branding profile (colors, typography, etc.; requires LLM config).
- `"screenshot"` – base64-encoded screenshot (requires `rod.enabled == true`).

Structured JSON extraction is requested via an object format:

```jsonc
{
  "formats": [
    {
      "type": "json",
      "prompt": "Extract the main pricing tiers.",
      "schema": {
        "type": "object",
        "properties": {
          "plans": {
            "type": "array",
            "items": {
              "type": "object",
              "properties": {
                "name": { "type": "string" },
                "price": { "type": "string" }
              }
            }
          }
        }
      }
    }
  ]
}
```

- `type: "json"` – enables structured extraction.
- `prompt` – instructions to the LLM about what to extract.
- `schema` – JSON-schema-like object guiding the output shape.

The extracted JSON is returned in `data.json` for `/v1/scrape`. For multi-URL extraction across pages, prefer `/v1/extract`.

---

## 2. Response Shape

On success (`200 OK`):

```jsonc
{
  "success": true,
  "data": {
    "markdown": "...",     // if requested
    "html": "...",         // if requested
    "rawHtml": "...",      // if requested
    "links": ["..."],
    "images": ["..."],
    "summary": "...",      // if requested and LLM succeeds
    "branding": { ... },    // if requested and LLM succeeds
    "json": { ... },        // if json format requested
    "screenshot": "...",   // base64, if requested and available
    "engine": "http" | "browser",
    "metadata": {
      "title": "...",
      "description": "...",
      "language": "en",
      "keywords": "...",
      "robots": "...",
      "ogTitle": "...",
      "ogDescription": "...",
      "ogUrl": "...",
      "ogImage": "...",
      "ogSiteName": "...",
      "sourceURL": "https://example.com",
      "statusCode": 200
    }
  }
}
```

Error responses use a standard envelope:

```jsonc
{
  "success": false,
  "code": "SCRAPE_FAILED" | "BAD_REQUEST" | "BAD_REQUEST_INVALID_JSON" | "SCREENSHOT_NOT_AVAILABLE" | "SCREENSHOT_FAILED" | "LLM_NOT_CONFIGURED" | "SUMMARY_FAILED",
  "error": "human-readable explanation"
}
```

---

## 3. Examples

### 3.1 Basic markdown scrape

```bash
curl -X POST http://localhost:8080/v1/scrape \
  -H 'Authorization: Bearer <api-key>' \
  -H 'Content-Type: application/json' \
  -d '{
    "url": "https://example.com",
    "formats": ["markdown"]
  }'
```

### 3.2 Browser-based scrape with screenshot

```bash
curl -X POST http://localhost:8080/v1/scrape \
  -H 'Authorization: Bearer <api-key>' \
  -H 'Content-Type: application/json' \
  -d '{
    "url": "https://example.com/dashboard",
    "formats": ["markdown", "screenshot"],
    "useBrowser": true,
    "timeout": 45000
  }'
```

- Requires `rod.enabled: true` in config.
- Returns `data.screenshot` as a base64 string and `data.engine == "browser"`.

### 3.3 Summary-only scrape

```bash
curl -X POST http://localhost:8080/v1/scrape \
  -H 'Authorization: Bearer <api-key>' \
  -H 'Content-Type: application/json' \
  -d '{
    "url": "https://example.com/blog/post",
    "formats": ["summary"]
  }'
```

- Uses the configured LLM provider (`llm.defaultProvider`) and model.
- Returns `data.summary` and `data.metadata`; `data.markdown` is omitted because it was not requested.

### 3.4 JSON extraction for a single page

```bash
curl -X POST http://localhost:8080/v1/scrape \
  -H 'Authorization: Bearer <api-key>' \
  -H 'Content-Type: application/json' \
  -d '{
    "url": "https://news.ycombinator.com/",
    "formats": [
      "markdown",
      {
        "type": "json",
        "prompt": "Extract the top 5 story titles and their points.",
        "schema": {
          "type": "object",
          "properties": {
            "top": {
              "type": "array",
              "items": {
                "type": "object",
                "properties": {
                  "title": { "type": "string" },
                  "points": { "type": "number" }
                }
              }
            }
          }
        }
      }
    ]
  }'
```

- Returns `data.markdown` and `data.json.top[]` entries with `{ "title", "points" }`.

---

## 4. Operational Notes

- **Timeouts**: long-running pages or LLM calls can hit `SCRAPE_FAILED` or `SUMMARY_FAILED` with `504` if they exceed the configured timeout.
- **Browser usage**: enabling `useBrowser` or `screenshot` increases CPU and memory usage; for high-volume workloads, consider running separate worker nodes with rod enabled.
- **LLM usage**: `summary`, `branding`, and `json` extraction all use the LLM provider configured under `llm` in `config.yaml`. Misconfiguration is reported as `LLM_NOT_CONFIGURED`.

For more on configuration and deployment, see `docs/config.md` and `docs/deploy.md`.