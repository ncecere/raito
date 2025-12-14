package metrics

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Simple Prometheus-style metrics for HTTP requests.
// This is intentionally minimal and in-memory only.

var (
	mu             sync.RWMutex
	requestsTotal  = make(map[reqKey]int64)
	latencyMsSum   = make(map[latKey]int64)
	latencyMsCount = make(map[latKey]int64)
	llmExtracts    = make(map[llmKey]int64)

	retentionJobsDeleted      = make(map[string]int64)
	retentionDocumentsDeleted int64

	searchRequestsTotal       = make(map[searchKey]int64)
	searchResultsTotal        = make(map[string]int64)
	searchScrapedResultsTotal = make(map[string]int64)

	extractJobsTotal         = make(map[extractJobKey]int64)
	extractResultsTotal      = make(map[extractResultKey]int64)
	extractFailureCodesTotal = make(map[extractFailureCodeKey]int64)
)

type reqKey struct {
	Method string
	Path   string
	Status int
}

type latKey struct {
	Method string
	Path   string
}

type llmKey struct {
	Provider string
	Model    string
	Success  string
}

type searchKey struct {
	Provider string
	Scrape   string
}

type extractJobKey struct {
	Provider string
	Model    string
	Status   string
}

type extractResultKey struct {
	Provider string
	Outcome  string
}

type extractFailureCodeKey struct {
	Provider string
	Code     string
}

// RecordRequest increments request counter and records latency.
func RecordRequest(method, path string, status int, latencyMs int64) {
	mu.Lock()
	defer mu.Unlock()

	rk := reqKey{Method: method, Path: path, Status: status}
	requestsTotal[rk]++

	lk := latKey{Method: method, Path: path}
	latencyMsSum[lk] += latencyMs
	latencyMsCount[lk]++
}

// RecordLLMExtract increments LLM extract counters.
func RecordLLMExtract(provider, model string, success bool) {
	mu.Lock()
	defer mu.Unlock()

	s := "false"
	if success {
		s = "true"
	}
	key := llmKey{Provider: provider, Model: model, Success: s}
	llmExtracts[key]++
}

// RecordRetentionJobs increments the counter of jobs deleted by TTL for
// a given job type.
func RecordRetentionJobs(jobType string, deleted int64) {
	if deleted <= 0 {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	retentionJobsDeleted[jobType] += deleted
}

// RecordRetentionDocuments increments the counter of documents deleted
// by TTL cleanup.
func RecordRetentionDocuments(deleted int64) {
	if deleted <= 0 {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	retentionDocumentsDeleted += deleted
}

// RecordSearch records basic metrics for search requests, including
// whether scraping was requested and how many results/documents were
// returned.
func RecordSearch(provider string, withScrape bool, results int, scraped int) {
	mu.Lock()
	defer mu.Unlock()

	scrapeFlag := "false"
	if withScrape {
		scrapeFlag = "true"
	}

	key := searchKey{Provider: provider, Scrape: scrapeFlag}
	searchRequestsTotal[key]++

	if results > 0 {
		searchResultsTotal[provider] += int64(results)
	}
	if scraped > 0 {
		searchScrapedResultsTotal[provider] += int64(scraped)
	}
}

// RecordExtractJob increments counters for extract jobs keyed by
// provider, model, and status (e.g., completed/failed).
func RecordExtractJob(provider, model, status string) {
	mu.Lock()
	defer mu.Unlock()

	key := extractJobKey{Provider: provider, Model: model, Status: status}
	extractJobsTotal[key]++
}

// RecordExtractResults increments counters for extracted results by
// provider and outcome (success or failed).
func RecordExtractResults(provider string, success, failed int) {
	if success <= 0 && failed <= 0 {
		return
	}
	mu.Lock()
	defer mu.Unlock()

	if success > 0 {
		key := extractResultKey{Provider: provider, Outcome: "success"}
		extractResultsTotal[key] += int64(success)
	}
	if failed > 0 {
		key := extractResultKey{Provider: provider, Outcome: "failed"}
		extractResultsTotal[key] += int64(failed)
	}
}

// RecordExtractFailureCode increments counters for extract failures by
// provider and error code.
func RecordExtractFailureCode(provider, code string, count int) {
	if count <= 0 || code == "" {
		return
	}
	mu.Lock()
	defer mu.Unlock()

	key := extractFailureCodeKey{Provider: provider, Code: code}
	extractFailureCodesTotal[key] += int64(count)
}

// Export returns Prometheus-style metrics text.
func Export() string {
	mu.RLock()
	defer mu.RUnlock()

	var b strings.Builder

	b.WriteString("# HELP raito_http_requests_total Total HTTP requests\n")
	b.WriteString("# TYPE raito_http_requests_total counter\n")

	// Sort keys for stable output
	var reqKeys []reqKey
	for k := range requestsTotal {
		reqKeys = append(reqKeys, k)
	}
	sort.Slice(reqKeys, func(i, j int) bool {
		if reqKeys[i].Method != reqKeys[j].Method {
			return reqKeys[i].Method < reqKeys[j].Method
		}
		if reqKeys[i].Path != reqKeys[j].Path {
			return reqKeys[i].Path < reqKeys[j].Path
		}
		return reqKeys[i].Status < reqKeys[j].Status
	})

	for _, k := range reqKeys {
		v := requestsTotal[k]
		fmt.Fprintf(&b, "raito_http_requests_total{method=\"%s\",path=\"%s\",status=\"%d\"} %d\n",
			k.Method, k.Path, k.Status, v)
	}

	b.WriteString("# HELP raito_http_request_duration_ms_sum Total request duration in milliseconds\n")
	b.WriteString("# TYPE raito_http_request_duration_ms_sum counter\n")
	b.WriteString("# HELP raito_http_request_duration_ms_count Request count for latency metric\n")
	b.WriteString("# TYPE raito_http_request_duration_ms_count counter\n")

	var latKeys []latKey
	for k := range latencyMsSum {
		latKeys = append(latKeys, k)
	}
	sort.Slice(latKeys, func(i, j int) bool {
		if latKeys[i].Method != latKeys[j].Method {
			return latKeys[i].Method < latKeys[j].Method
		}
		return latKeys[i].Path < latKeys[j].Path
	})

	for _, k := range latKeys {
		sum := latencyMsSum[k]
		cnt := latencyMsCount[k]
		fmt.Fprintf(&b, "raito_http_request_duration_ms_sum{method=\"%s\",path=\"%s\"} %d\n",
			k.Method, k.Path, sum)
		fmt.Fprintf(&b, "raito_http_request_duration_ms_count{method=\"%s\",path=\"%s\"} %d\n",
			k.Method, k.Path, cnt)
	}

	// LLM extract metrics
	b.WriteString("# HELP raito_llm_extract_requests_total Total LLM extract requests\n")
	b.WriteString("# TYPE raito_llm_extract_requests_total counter\n")

	var llmKeys []llmKey
	for k := range llmExtracts {
		llmKeys = append(llmKeys, k)
	}
	sort.Slice(llmKeys, func(i, j int) bool {
		if llmKeys[i].Provider != llmKeys[j].Provider {
			return llmKeys[i].Provider < llmKeys[j].Provider
		}
		if llmKeys[i].Model != llmKeys[j].Model {
			return llmKeys[i].Model < llmKeys[j].Model
		}
		return llmKeys[i].Success < llmKeys[j].Success
	})

	for _, k := range llmKeys {
		v := llmExtracts[k]
		fmt.Fprintf(&b, "raito_llm_extract_requests_total{provider=\"%s\",model=\"%s\",success=\"%s\"} %d\n",
			k.Provider, k.Model, k.Success, v)
	}

	// Search metrics
	b.WriteString("# HELP raito_search_requests_total Total search requests by provider and scrape mode\n")
	b.WriteString("# TYPE raito_search_requests_total counter\n")

	var searchKeys []searchKey
	for k := range searchRequestsTotal {
		searchKeys = append(searchKeys, k)
	}
	sort.Slice(searchKeys, func(i, j int) bool {
		if searchKeys[i].Provider != searchKeys[j].Provider {
			return searchKeys[i].Provider < searchKeys[j].Provider
		}
		return searchKeys[i].Scrape < searchKeys[j].Scrape
	})

	for _, k := range searchKeys {
		v := searchRequestsTotal[k]
		fmt.Fprintf(&b, "raito_search_requests_total{provider=\"%s\",scrape=\"%s\"} %d\n",
			k.Provider, k.Scrape, v)
	}

	b.WriteString("# HELP raito_search_results_total Total search results returned by provider\n")
	b.WriteString("# TYPE raito_search_results_total counter\n")

	var searchProviders []string
	for p := range searchResultsTotal {
		searchProviders = append(searchProviders, p)
	}
	sort.Strings(searchProviders)
	for _, p := range searchProviders {
		v := searchResultsTotal[p]
		fmt.Fprintf(&b, "raito_search_results_total{provider=\"%s\"} %d\n", p, v)
	}

	b.WriteString("# HELP raito_search_scraped_results_total Total search results with scraped documents\n")
	b.WriteString("# TYPE raito_search_scraped_results_total counter\n")

	var scrapedProviders []string
	for p := range searchScrapedResultsTotal {
		scrapedProviders = append(scrapedProviders, p)
	}
	sort.Strings(scrapedProviders)
	for _, p := range scrapedProviders {
		v := searchScrapedResultsTotal[p]
		fmt.Fprintf(&b, "raito_search_scraped_results_total{provider=\"%s\"} %d\n", p, v)
	}

	// Extract metrics
	b.WriteString("# HELP raito_extract_jobs_total Total extract jobs by provider, model, and status\n")
	b.WriteString("# TYPE raito_extract_jobs_total counter\n")

	var extractJobKeys []extractJobKey
	for k := range extractJobsTotal {
		extractJobKeys = append(extractJobKeys, k)
	}
	sort.Slice(extractJobKeys, func(i, j int) bool {
		if extractJobKeys[i].Provider != extractJobKeys[j].Provider {
			return extractJobKeys[i].Provider < extractJobKeys[j].Provider
		}
		if extractJobKeys[i].Model != extractJobKeys[j].Model {
			return extractJobKeys[i].Model < extractJobKeys[j].Model
		}
		return extractJobKeys[i].Status < extractJobKeys[j].Status
	})

	for _, k := range extractJobKeys {
		v := extractJobsTotal[k]
		fmt.Fprintf(&b, "raito_extract_jobs_total{provider=\"%s\",model=\"%s\",status=\"%s\"} %d\n",
			k.Provider, k.Model, k.Status, v)
	}

	b.WriteString("# HELP raito_extract_results_total Total extract results by provider and outcome\n")
	b.WriteString("# TYPE raito_extract_results_total counter\n")

	var extractResultKeys []extractResultKey
	for k := range extractResultsTotal {
		extractResultKeys = append(extractResultKeys, k)
	}
	sort.Slice(extractResultKeys, func(i, j int) bool {
		if extractResultKeys[i].Provider != extractResultKeys[j].Provider {
			return extractResultKeys[i].Provider < extractResultKeys[j].Provider
		}
		return extractResultKeys[i].Outcome < extractResultKeys[j].Outcome
	})

	for _, k := range extractResultKeys {
		v := extractResultsTotal[k]
		fmt.Fprintf(&b, "raito_extract_results_total{provider=\"%s\",outcome=\"%s\"} %d\n",
			k.Provider, k.Outcome, v)
	}

	b.WriteString("# HELP raito_extract_failures_by_code_total Total extract failures by provider and error code\n")
	b.WriteString("# TYPE raito_extract_failures_by_code_total counter\n")

	var extractFailureCodeKeys []extractFailureCodeKey
	for k := range extractFailureCodesTotal {
		extractFailureCodeKeys = append(extractFailureCodeKeys, k)
	}
	sort.Slice(extractFailureCodeKeys, func(i, j int) bool {
		if extractFailureCodeKeys[i].Provider != extractFailureCodeKeys[j].Provider {
			return extractFailureCodeKeys[i].Provider < extractFailureCodeKeys[j].Provider
		}
		return extractFailureCodeKeys[i].Code < extractFailureCodeKeys[j].Code
	})

	for _, k := range extractFailureCodeKeys {
		v := extractFailureCodesTotal[k]
		fmt.Fprintf(&b, "raito_extract_failures_by_code_total{provider=\"%s\",code=\"%s\"} %d\n",
			k.Provider, k.Code, v)
	}

	// Retention metrics
	b.WriteString("# HELP raito_retention_jobs_deleted_total Total jobs deleted by TTL\n")
	b.WriteString("# TYPE raito_retention_jobs_deleted_total counter\n")

	// Sort job types for stable output
	var jobTypes []string
	for t := range retentionJobsDeleted {
		jobTypes = append(jobTypes, t)
	}
	sort.Strings(jobTypes)
	for _, t := range jobTypes {
		v := retentionJobsDeleted[t]
		fmt.Fprintf(&b, "raito_retention_jobs_deleted_total{job_type=\"%s\"} %d\n", t, v)
	}

	b.WriteString("# HELP raito_retention_documents_deleted_total Total documents deleted by TTL\n")
	b.WriteString("# TYPE raito_retention_documents_deleted_total counter\n")
	fmt.Fprintf(&b, "raito_retention_documents_deleted_total %d\n", retentionDocumentsDeleted)

	return b.String()
}
