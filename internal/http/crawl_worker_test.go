package http

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"raito/internal/config"
	"raito/internal/llm"
	"raito/internal/scraper"
)

type fakeJobStore struct {
	lastStatus string
	lastError  *string
	output     json.RawMessage
}

func (f *fakeJobStore) UpdateCrawlJobStatus(_ context.Context, _ uuid.UUID, status string, errMsg *string) error {
	f.lastStatus = status
	f.lastError = errMsg
	return nil
}

func (f *fakeJobStore) SetJobOutput(_ context.Context, _ uuid.UUID, output json.RawMessage) error {
	f.output = output
	return nil
}

type fakeScraper struct {
	byURL    map[string]*scraper.Result
	errByURL map[string]error
}

func (f *fakeScraper) Scrape(_ context.Context, req scraper.Request) (*scraper.Result, error) {
	if err, ok := f.errByURL[req.URL]; ok {
		return nil, err
	}
	if res, ok := f.byURL[req.URL]; ok {
		return res, nil
	}
	return nil, nil
}

type fakeLLM struct {
	fieldsByURL map[string]map[string]any
	errByURL    map[string]error
}

func (f *fakeLLM) ExtractFields(_ context.Context, req llm.ExtractRequest) (llm.ExtractResult, error) {
	if err, ok := f.errByURL[req.URL]; ok {
		return llm.ExtractResult{}, err
	}
	if fields, ok := f.fieldsByURL[req.URL]; ok {
		return llm.ExtractResult{Fields: fields}, nil
	}
	return llm.ExtractResult{Fields: map[string]any{}}, nil
}

func newTestConfig() *config.Config {
	return &config.Config{
		Scraper: config.ScraperConfig{TimeoutMs: 1000},
	}
}

func withFakeDeps(t *testing.T, deps *extractDeps) func() {
	t.Helper()
	orig := newExtractDeps
	newExtractDeps = func(_ *config.Config, _ ExtractRequest) (*extractDeps, error) {
		return deps, nil
	}
	return func() { newExtractDeps = orig }
}

func decodeOutput(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	if len(raw) == 0 {
		t.Fatalf("expected non-empty output")
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("failed to decode output: %v", err)
	}
	return out
}

func readArray(t *testing.T, m map[string]any, key string) []any {
	t.Helper()
	raw, ok := m[key]
	if !ok {
		t.Fatalf("expected key %q in map", key)
	}
	arr, ok := raw.([]any)
	if !ok {
		t.Fatalf("expected %q to be []any, got %T", key, raw)
	}
	return arr
}

func readMap(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", v)
	}
	return m
}

func TestRunExtractJob_SingleURLSuccess(t *testing.T) {
	cfg := newTestConfig()
	st := &fakeJobStore{}

	url := "https://example.com"
	fakeScr := &fakeScraper{
		byURL: map[string]*scraper.Result{
			url: {
				URL:      url,
				Markdown: "Hello world",
				Status:   200,
			},
		},
		errByURL: map[string]error{},
	}
	fakeLLMClient := &fakeLLM{
		fieldsByURL: map[string]map[string]any{
			url: {
				"json": map[string]any{"title": "Example"},
			},
		},
		errByURL: map[string]error{},
	}

	deps := &extractDeps{
		scraper:   fakeScr,
		client:    fakeLLMClient,
		provider:  llm.Provider("test"),
		modelName: "test-model",
		timeout:   time.Second,
	}
	reset := withFakeDeps(t, deps)
	defer reset()

	jobID := uuid.New()
	req := ExtractRequest{
		URLs:   []string{url},
		Schema: map[string]any{"type": "object"},
	}

	runExtractJob(context.Background(), cfg, st, jobID, req)

	if st.lastStatus != "completed" {
		t.Fatalf("expected status completed, got %q", st.lastStatus)
	}

	out := decodeOutput(t, st.output)
	results := readArray(t, out, "results")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	res0 := readMap(t, results[0])
	if res0["url"] != url {
		t.Fatalf("unexpected url: %#v", res0["url"])
	}
	if ok, _ := res0["success"].(bool); !ok {
		t.Fatalf("expected success=true, got %#v", res0["success"])
	}
}

func TestRunExtractJob_MixedIgnoreInvalidFalse(t *testing.T) {
	cfg := newTestConfig()
	st := &fakeJobStore{}

	okURL := "https://ok.com"
	badURL := "https://bad.com"

	fakeScr := &fakeScraper{
		byURL: map[string]*scraper.Result{
			okURL: {
				URL:      okURL,
				Markdown: "OK",
				Status:   200,
			},
		},
		errByURL: map[string]error{
			badURL: fmt.Errorf("timeout"),
		},
	}
	fakeLLMClient := &fakeLLM{
		fieldsByURL: map[string]map[string]any{
			okURL: {
				"json": map[string]any{"title": "OK"},
			},
		},
		errByURL: map[string]error{},
	}

	deps := &extractDeps{
		scraper:   fakeScr,
		client:    fakeLLMClient,
		provider:  llm.Provider("test"),
		modelName: "test-model",
		timeout:   time.Second,
	}
	reset := withFakeDeps(t, deps)
	defer reset()

	jobID := uuid.New()
	ignoreInvalid := false
	req := ExtractRequest{
		URLs:              []string{okURL, badURL},
		Schema:            map[string]any{"type": "object"},
		IgnoreInvalidURLs: &ignoreInvalid,
	}

	runExtractJob(context.Background(), cfg, st, jobID, req)

	if st.lastStatus != "failed" {
		t.Fatalf("expected status failed, got %q", st.lastStatus)
	}
	if st.output != nil && len(st.output) > 0 {
		t.Fatalf("expected no output on failure, got %s", string(st.output))
	}
}

func TestRunExtractJob_MixedIgnoreInvalidTrueWithSources(t *testing.T) {
	cfg := newTestConfig()
	st := &fakeJobStore{}

	okURL := "https://ok.com"
	badURL := "https://bad.com"

	fakeScr := &fakeScraper{
		byURL: map[string]*scraper.Result{
			okURL: {
				URL:      okURL,
				Markdown: "OK",
				Status:   200,
			},
		},
		errByURL: map[string]error{
			badURL: fmt.Errorf("timeout"),
		},
	}
	fakeLLMClient := &fakeLLM{
		fieldsByURL: map[string]map[string]any{
			okURL: {
				"json": map[string]any{"title": "OK"},
			},
		},
		errByURL: map[string]error{},
	}

	deps := &extractDeps{
		scraper:   fakeScr,
		client:    fakeLLMClient,
		provider:  llm.Provider("test"),
		modelName: "test-model",
		timeout:   time.Second,
	}
	reset := withFakeDeps(t, deps)
	defer reset()

	jobID := uuid.New()
	ignoreInvalid := true
	showSources := true
	req := ExtractRequest{
		URLs:              []string{okURL, badURL},
		Schema:            map[string]any{"type": "object"},
		IgnoreInvalidURLs: &ignoreInvalid,
		ShowSources:       &showSources,
	}

	runExtractJob(context.Background(), cfg, st, jobID, req)

	if st.lastStatus != "completed" {
		t.Fatalf("expected status completed, got %q", st.lastStatus)
	}

	out := decodeOutput(t, st.output)
	results := readArray(t, out, "results")
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	res0 := readMap(t, results[0])
	res1 := readMap(t, results[1])

	// One success and one failure
	successCount := 0
	failureCount := 0
	for _, r := range []map[string]any{res0, res1} {
		if ok, _ := r["success"].(bool); ok {
			successCount++
		} else {
			failureCount++
		}
	}
	if successCount != 1 || failureCount != 1 {
		t.Fatalf("expected 1 success and 1 failure, got %d success / %d failure", successCount, failureCount)
	}

	sources := readArray(t, out, "sources")
	if len(sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(sources))
	}

	src0 := readMap(t, sources[0])
	src1 := readMap(t, sources[1])
	if src0["url"] == badURL {
		src0, src1 = src1, src0
	}
	if src0["url"] != okURL || src1["url"] != badURL {
		t.Fatalf("unexpected urls in sources: %#v %#v", src0["url"], src1["url"])
	}
	if errStr, _ := src1["error"].(string); errStr == "" || errStr[:13] != "SCRAPE_FAILED" {
		t.Fatalf("expected SCRAPE_FAILED error for bad URL, got %q", errStr)
	}

	summary := readMap(t, out["summary"])
	if total, _ := summary["total"].(float64); int(total) != 2 {
		t.Fatalf("expected summary.total=2, got %v", summary["total"])
	}
	if success, _ := summary["success"].(float64); int(success) != 1 {
		t.Fatalf("expected summary.success=1, got %v", summary["success"])
	}
	if failed, _ := summary["failed"].(float64); int(failed) != 1 {
		t.Fatalf("expected summary.failed=1, got %v", summary["failed"])
	}

	failedByCode := readMap(t, summary["failedByCode"])
	if v, _ := failedByCode["SCRAPE_FAILED"].(float64); int(v) != 1 {
		t.Fatalf("expected SCRAPE_FAILED count=1, got %v", failedByCode["SCRAPE_FAILED"])
	}
}

func TestRunExtractJob_LLMErrorsIgnoredWhenAllowed(t *testing.T) {
	cfg := newTestConfig()
	st := &fakeJobStore{}

	okURL := "https://ok.com"
	badURL := "https://bad.com"

	fakeScr := &fakeScraper{
		byURL: map[string]*scraper.Result{
			okURL:  {URL: okURL, Markdown: "OK", Status: 200},
			badURL: {URL: badURL, Markdown: "BAD", Status: 200},
		},
		errByURL: map[string]error{},
	}
	fakeLLMClient := &fakeLLM{
		fieldsByURL: map[string]map[string]any{
			okURL: {"json": map[string]any{"title": "OK"}},
		},
		errByURL: map[string]error{
			badURL: fmt.Errorf("llm failure"),
		},
	}

	deps := &extractDeps{
		scraper:   fakeScr,
		client:    fakeLLMClient,
		provider:  llm.Provider("test"),
		modelName: "test-model",
		timeout:   time.Second,
	}
	reset := withFakeDeps(t, deps)
	defer reset()

	jobID := uuid.New()
	ignoreInvalid := true
	req := ExtractRequest{
		URLs:              []string{okURL, badURL},
		Schema:            map[string]any{"type": "object"},
		IgnoreInvalidURLs: &ignoreInvalid,
	}

	runExtractJob(context.Background(), cfg, st, jobID, req)

	if st.lastStatus != "completed" {
		t.Fatalf("expected status completed, got %q", st.lastStatus)
	}

	out := decodeOutput(t, st.output)
	results := readArray(t, out, "results")
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	summary := readMap(t, out["summary"])
	if total, _ := summary["total"].(float64); int(total) != 2 {
		t.Fatalf("expected summary.total=2, got %v", summary["total"])
	}
	if success, _ := summary["success"].(float64); int(success) != 1 {
		t.Fatalf("expected summary.success=1, got %v", summary["success"])
	}
	if failed, _ := summary["failed"].(float64); int(failed) != 1 {
		t.Fatalf("expected summary.failed=1, got %v", summary["failed"])
	}

	failedByCode := readMap(t, summary["failedByCode"])
	if v, _ := failedByCode["EXTRACT_FAILED"].(float64); int(v) != 1 {
		t.Fatalf("expected EXTRACT_FAILED count=1, got %v", failedByCode["EXTRACT_FAILED"])
	}
}

func TestRunExtractJob_LLMErrorsFailJobWhenNotIgnored(t *testing.T) {
	cfg := newTestConfig()
	st := &fakeJobStore{}

	url := "https://example.com"

	fakeScr := &fakeScraper{
		byURL: map[string]*scraper.Result{
			url: {URL: url, Markdown: "OK", Status: 200},
		},
		errByURL: map[string]error{},
	}
	fakeLLMClient := &fakeLLM{
		fieldsByURL: map[string]map[string]any{},
		errByURL: map[string]error{
			url: fmt.Errorf("llm failure"),
		},
	}

	deps := &extractDeps{
		scraper:   fakeScr,
		client:    fakeLLMClient,
		provider:  llm.Provider("test"),
		modelName: "test-model",
		timeout:   time.Second,
	}
	reset := withFakeDeps(t, deps)
	defer reset()

	jobID := uuid.New()
	req := ExtractRequest{
		URLs:   []string{url},
		Schema: map[string]any{"type": "object"},
	}

	runExtractJob(context.Background(), cfg, st, jobID, req)

	if st.lastStatus != "failed" {
		t.Fatalf("expected status failed, got %q", st.lastStatus)
	}
	if st.output != nil && len(st.output) > 0 {
		t.Fatalf("expected no output on failure, got %s", string(st.output))
	}
}

func TestRunExtractJob_EmptyResultHandling(t *testing.T) {
	cfg := newTestConfig()
	st := &fakeJobStore{}

	okURL := "https://ok.com"
	badURL := "https://empty.com"

	fakeScr := &fakeScraper{
		byURL: map[string]*scraper.Result{
			okURL:  {URL: okURL, Markdown: "OK", Status: 200},
			badURL: {URL: badURL, Markdown: "", Status: 200},
		},
		errByURL: map[string]error{},
	}
	fakeLLMClient := &fakeLLM{
		fieldsByURL: map[string]map[string]any{
			okURL: {"json": map[string]any{"title": "OK"}},
			// badURL intentionally returns empty fields map
		},
		errByURL: map[string]error{},
	}

	deps := &extractDeps{
		scraper:   fakeScr,
		client:    fakeLLMClient,
		provider:  llm.Provider("test"),
		modelName: "test-model",
		timeout:   time.Second,
	}
	reset := withFakeDeps(t, deps)
	defer reset()

	jobID := uuid.New()
	ignoreInvalid := true
	req := ExtractRequest{
		URLs:              []string{okURL, badURL},
		Schema:            map[string]any{"type": "object"},
		IgnoreInvalidURLs: &ignoreInvalid,
	}

	runExtractJob(context.Background(), cfg, st, jobID, req)

	if st.lastStatus != "completed" {
		t.Fatalf("expected status completed, got %q", st.lastStatus)
	}

	out := decodeOutput(t, st.output)
	summary := readMap(t, out["summary"])
	if total, _ := summary["total"].(float64); int(total) != 2 {
		t.Fatalf("expected summary.total=2, got %v", summary["total"])
	}
	if success, _ := summary["success"].(float64); int(success) != 1 {
		t.Fatalf("expected summary.success=1, got %v", summary["success"])
	}
	if failed, _ := summary["failed"].(float64); int(failed) != 1 {
		t.Fatalf("expected summary.failed=1, got %v", summary["failed"])
	}

	failedByCode := readMap(t, summary["failedByCode"])
	if v, _ := failedByCode["EXTRACT_EMPTY_RESULT"].(float64); int(v) != 1 {
		t.Fatalf("expected EXTRACT_EMPTY_RESULT count=1, got %v", failedByCode["EXTRACT_EMPTY_RESULT"])
	}
}
