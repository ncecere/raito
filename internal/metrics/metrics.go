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

	return b.String()
}
