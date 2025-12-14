package formats

import (
	"fmt"
	"strings"

	"raito/internal/scrapeutil"
)

// Format represents a logical output format supported by Raito.
// The names are aligned with Firecrawl-style format identifiers.
type Format string

const (
	FormatMarkdown   Format = "markdown"
	FormatHTML       Format = "html"
	FormatRawHTML    Format = "rawHtml"
	FormatLinks      Format = "links"
	FormatImages     Format = "images"
	FormatSummary    Format = "summary"
	FormatJSON       Format = "json"
	FormatBranding   Format = "branding"
	FormatScreenshot Format = "screenshot"
)

// HasFormat reports whether the given Firecrawl-style formats array
// contains the specified format name. It is a thin wrapper around
// scrapeutil.WantsFormat so callers do not need to depend on helpers.
func HasFormat(formats []any, name string) bool {
	return scrapeutil.WantsFormat(formats, name)
}

// normalizeFormatName converts a Firecrawl-style format descriptor
// (either a string or {type: string}) into a lowercased name.
func normalizeFormatName(f any) string {
	switch v := f.(type) {
	case string:
		return strings.ToLower(strings.TrimSpace(v))
	case map[string]any:
		if t, ok := v["type"].(string); ok {
			return strings.ToLower(strings.TrimSpace(t))
		}
	}
	return ""
}

// ValidateFormatsForEndpoint validates a formats array for a specific
// endpoint. Currently only /v1/search applies restrictions; other
// endpoints accept the full set of formats and this function returns
// nil for them.
//
// The returned error message is intended to be user-facing and is
// wired directly into HTTP error responses.
func ValidateFormatsForEndpoint(endpoint string, formats []any) error {
	if len(formats) == 0 {
		return nil
	}

	switch endpoint {
	case "search":
		// /v1/search only supports a limited subset of formats when
		// scrapeOptions are provided, to keep payloads small and
		// behavior predictable.
		allowed := map[string]struct{}{
			"markdown": {},
			"html":     {},
			"rawhtml":  {},
		}

		for _, f := range formats {
			name := normalizeFormatName(f)
			if name == "" {
				return fmt.Errorf("Unsupported format for /v1/search; allowed formats are: markdown, html, rawHtml")
			}
			if _, ok := allowed[name]; !ok {
				// Preserve the existing error wording used by the HTTP
				// handler so clients see consistent messages.
				return fmt.Errorf("Unsupported format %q for /v1/search; allowed formats are: markdown, html, rawHtml", name)
			}
		}
	}

	return nil
}
