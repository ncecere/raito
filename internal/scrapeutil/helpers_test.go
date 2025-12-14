package scrapeutil

import "testing"

func TestToString(t *testing.T) {
	if got := ToString(nil); got != "" {
		t.Fatalf("ToString(nil) = %q, want empty string", got)
	}
	if got := ToString("hello"); got != "hello" {
		t.Fatalf("ToString(\"hello\") = %q, want \"hello\"", got)
	}
	if got := ToString(123); got != "" {
		t.Fatalf("ToString(123) = %q, want empty string for non-string", got)
	}
}

func TestFilterLinks(t *testing.T) {
	links := []string{
		"https://example.com/a",
		"https://example.com/b",
		"https://other.com/x",
		"",
	}

	// sameDomainOnly=true should keep only example.com links.
	filtered := FilterLinks(links, "https://example.com/base", true, 0)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 filtered links, got %d (%v)", len(filtered), filtered)
	}
	for _, l := range filtered {
		if l[:19] != "https://example.com" {
			t.Fatalf("expected same-domain link, got %q", l)
		}
	}

	// maxPerDocument should cap the number of returned links.
	filtered = FilterLinks(links, "https://example.com/base", false, 1)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered link with maxPerDocument=1, got %d", len(filtered))
	}
}
