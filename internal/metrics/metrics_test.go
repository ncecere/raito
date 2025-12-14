package metrics

import (
	"strings"
	"testing"
)

func TestRecordRequestAndExport(t *testing.T) {
	// Record a single request and ensure it appears in the export.
	RecordRequest("GET", "/v1/scrape", 200, 42)

	out := Export()
	if !strings.Contains(out, "raito_http_requests_total{method=\"GET\",path=\"/v1/scrape\",status=\"200\"}") {
		t.Fatalf("expected HTTP request metric for GET /v1/scrape in export, got:\n%s", out)
	}
	if !strings.Contains(out, "raito_http_request_duration_ms_sum") || !strings.Contains(out, "raito_http_request_duration_ms_count") {
		t.Fatalf("expected latency metrics headers in export, got:\n%s", out)
	}
}

func TestRecordSearchMetrics(t *testing.T) {
	RecordSearch("searxng", false, 3, 0)
	RecordSearch("searxng", true, 2, 1)

	out := Export()
	if !strings.Contains(out, "raito_search_requests_total{provider=\"searxng\",scrape=\"false\"}") {
		t.Fatalf("expected search_requests_total without scrape, got:\n%s", out)
	}
	if !strings.Contains(out, "raito_search_requests_total{provider=\"searxng\",scrape=\"true\"}") {
		t.Fatalf("expected search_requests_total with scrape, got:\n%s", out)
	}
	if !strings.Contains(out, "raito_search_results_total{provider=\"searxng\"}") {
		t.Fatalf("expected search_results_total for searxng, got:\n%s", out)
	}
	if !strings.Contains(out, "raito_search_scraped_results_total{provider=\"searxng\"}") {
		t.Fatalf("expected search_scraped_results_total for searxng, got:\n%s", out)
	}
}

func TestRecordExtractMetrics(t *testing.T) {
	RecordExtractJob("openai", "gpt-test", "completed")
	RecordExtractResults("openai", 2, 1)
	RecordExtractFailureCode("openai", "EXTRACT_FAILED", 1)

	out := Export()
	if !strings.Contains(out, "raito_extract_jobs_total{provider=\"openai\",model=\"gpt-test\",status=\"completed\"}") {
		t.Fatalf("expected extract_jobs_total for openai/gpt-test, got:\n%s", out)
	}
	if !strings.Contains(out, "raito_extract_results_total{provider=\"openai\",outcome=\"success\"}") {
		t.Fatalf("expected extract_results_total success for openai, got:\n%s", out)
	}
	if !strings.Contains(out, "raito_extract_results_total{provider=\"openai\",outcome=\"failed\"}") {
		t.Fatalf("expected extract_results_total failed for openai, got:\n%s", out)
	}
	if !strings.Contains(out, "raito_extract_failures_by_code_total{provider=\"openai\",code=\"EXTRACT_FAILED\"}") {
		t.Fatalf("expected extract_failures_by_code_total for openai/EXTRACT_FAILED, got:\n%s", out)
	}
}
