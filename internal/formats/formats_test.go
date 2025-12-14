package formats

import "testing"

func TestValidateFormatsForEndpoint_Search_AllowsMarkdownHtmlRawHtml(t *testing.T) {
	formats := []interface{}{"markdown", "html", "rawHtml"}
	if err := ValidateFormatsForEndpoint("search", formats); err != nil {
		t.Fatalf("expected allowed formats to pass, got error: %v", err)
	}
}

func TestValidateFormatsForEndpoint_Search_RejectsUnsupportedString(t *testing.T) {
	formats := []interface{}{"markdown", "summary"}
	err := ValidateFormatsForEndpoint("search", formats)
	if err == nil {
		t.Fatalf("expected error for unsupported format, got nil")
	}
	if got := err.Error(); got == "" {
		t.Fatalf("expected non-empty error message")
	}
}

func TestValidateFormatsForEndpoint_Search_RejectsUnsupportedObject(t *testing.T) {
	formats := []interface{}{
		map[string]interface{}{"type": "json"},
	}
	err := ValidateFormatsForEndpoint("search", formats)
	if err == nil {
		t.Fatalf("expected error for unsupported object format, got nil")
	}
}

func TestValidateFormatsForEndpoint_OtherEndpointNoRestriction(t *testing.T) {
	formats := []interface{}{"markdown", "summary", map[string]interface{}{"type": "json"}}
	if err := ValidateFormatsForEndpoint("scrape", formats); err != nil {
		t.Fatalf("expected no restriction for non-search endpoint, got %v", err)
	}
}
