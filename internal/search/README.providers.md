# Search Providers

Raito's `/v1/search` endpoint is implemented on top of a small provider
abstraction in `internal/search`.

## Provider interface

The core contract is `search.Provider`:

```go
// Provider defines the contract for pluggable search providers.
// Implementations are responsible for mapping a provider-agnostic
// Request into provider-specific API calls and normalizing results
// back into the shared Results shape. Providers should:
//   - respect the Limit and Timeout fields where possible,
//   - treat IgnoreInvalidURL as a hint for filtering malformed URLs,
//   - avoid returning sensitive configuration details in errors.
type Provider interface {
    Search(ctx context.Context, req *Request) (*Results, error)
}
```

`search.Request` and `search.Results` define the provider-agnostic
inputs/outputs:

- `Request` fields:
  - `Query`, `Sources`, `Limit`, `Country`, `Location`, `TBS`, `Timeout`.
  - `IgnoreInvalidURL` – hint that providers may drop results with
    obviously invalid or empty URLs instead of returning them.
- `Results` fields:
  - `Web`, `News`, `Images` – slices of `Result{Title, Description, URL}`.

## SearxNG provider

The default implementation is `SearxngProvider`, constructed via
`NewSearxngProvider(config.SearchConfig)` and selected from
`NewProviderFromConfig(*config.Config)` when `search.enabled` is true.

### Behavior

- Uses the configured `search.searxng.baseURL` as the upstream endpoint
  and sends form-encoded POST requests to `/search`.
- Respects `Request.Limit` (with a provider-specific default and
  server-side clamping in the service layer).
- Applies a request-scoped timeout derived from provider-specific,
  search-level, or scraper-level timeouts.
- Maps logical `Sources` into SearxNG `categories` (currently
  `general`, `images`, `news`); `/v1/search` only exposes `web` for now.
- Uses `Country`/`Location` and `TBS` (time-based search) as best-effort
  hints where the SearxNG API supports them.

### Limitations

- Requires a SearxNG instance with the JSON API enabled.
- Only a subset of the SearxNG response is used (title, url, content).
- Provider does not perform scraping; it only returns metadata. Scraping
  is handled separately by `SearchService.ScrapeResults`.
- Error messages intentionally avoid including API keys, internal
  configuration, or full upstream URLs beyond what is necessary for
  debugging.

This structure allows additional providers (for example, direct web
search APIs) to be added by implementing `search.Provider` and updating
`NewProviderFromConfig` with a new selector, without touching the
HTTP or services layers.
