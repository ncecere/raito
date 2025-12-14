package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"raito/internal/config"
)

// Provider represents a logical LLM provider.
type Provider string

const (
	ProviderOpenAI    Provider = "openai"
	ProviderAnthropic Provider = "anthropic"
	ProviderGoogle    Provider = "google"
)

// FieldSpec mirrors http.ExtractField but lives in llm package to
// keep the dependency direction simple.
type FieldSpec struct {
	Name        string
	Description string
	Type        string
}

// ExtractRequest is the LLM-specific request for field extraction.
type ExtractRequest struct {
	URL      string
	Markdown string
	Fields   []FieldSpec
	Prompt   string
	Provider Provider
	Model    string
	Timeout  time.Duration
	Strict   bool
}

// ExtractResult is the structured output from the LLM.
type ExtractResult struct {
	Fields map[string]any
}

// Client is the abstraction used by the HTTP layer.
type Client interface {
	ExtractFields(ctx context.Context, req ExtractRequest) (ExtractResult, error)
}

// parseJSONFields attempts to parse a JSON object from the given content.
// It first tries the whole string, and if that fails, it attempts to
// extract the first {...} block. On failure it returns an error so the
// caller can decide how to fall back.
func parseJSONFields(content string) (map[string]any, error) {
	var fields map[string]any
	if err := json.Unmarshal([]byte(content), &fields); err == nil {
		return fields, nil
	}

	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start == -1 || end <= start {
		return nil, errors.New("no JSON object found in content")
	}

	snippet := content[start : end+1]
	if err := json.Unmarshal([]byte(snippet), &fields); err != nil {
		return nil, err
	}

	return fields, nil
}

// NewClientFromConfig constructs a Client based on global config and optional
// per-request provider/model overrides.
func NewClientFromConfig(cfg *config.Config, providerOverride, modelOverride string) (Client, Provider, string, error) {
	providerName := cfg.LLM.DefaultProvider
	if providerOverride != "" {
		providerName = providerOverride
	}

	prov := Provider(providerName)

	switch prov {
	case ProviderOpenAI:
		openaiCfg := cfg.LLM.OpenAI
		model := openaiCfg.Model
		if modelOverride != "" {
			model = modelOverride
		}
		if openaiCfg.APIKey == "" || model == "" {
			return nil, prov, model, errors.New("openai llm provider is not fully configured")
		}
		return &openAIClient{
			apiKey:  openaiCfg.APIKey,
			baseURL: openaiCfg.BaseURL,
			model:   model,
			http:    &http.Client{Timeout: 30 * time.Second},
		}, prov, model, nil
	case ProviderAnthropic:
		anthCfg := cfg.LLM.Anthropic
		model := anthCfg.Model
		if modelOverride != "" {
			model = modelOverride
		}
		if anthCfg.APIKey == "" || model == "" {
			return nil, prov, model, errors.New("anthropic llm provider is not fully configured")
		}
		return &anthropicClient{
			apiKey: anthCfg.APIKey,
			model:  model,
			http:   &http.Client{Timeout: 30 * time.Second},
		}, prov, model, nil
	case ProviderGoogle:
		googleCfg := cfg.LLM.Google
		model := googleCfg.Model
		if modelOverride != "" {
			model = modelOverride
		}
		if googleCfg.APIKey == "" || model == "" {
			return nil, prov, model, errors.New("google llm provider is not fully configured")
		}
		return &googleClient{
			apiKey: googleCfg.APIKey,
			model:  model,
			http:   &http.Client{Timeout: 30 * time.Second},
		}, prov, model, nil
	default:
		return nil, prov, "", fmt.Errorf("unsupported llm provider: %s", providerName)
	}
}

// openAIClient implements Client using OpenAI-compatible Chat Completions.
type openAIClient struct {
	apiKey  string
	baseURL string
	model   string
	http    *http.Client
}

// anthropicClient implements Client using Anthropic's Messages API.
type anthropicClient struct {
	apiKey string
	model  string
	http   *http.Client
}

// googleClient implements Client using Google Gemini (Generative Language API).
type googleClient struct {
	apiKey string
	model  string
	http   *http.Client
}

// openAIChatRequest is a minimal representation of the Chat Completions API.
type openAIChatRequest struct {
	Model          string                `json:"model"`
	Messages       []openAIChatMessage   `json:"messages"`
	Temperature    float64               `json:"temperature"`
	ResponseFormat *openAIResponseFormat `json:"response_format,omitempty"`
}

type openAIResponseFormat struct {
	Type string `json:"type"`
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message openAIChatMessage `json:"message"`
	} `json:"choices"`
}

// anthropicMessagesRequest & response are minimal shapes for Anthropic's Messages API.
type anthropicMessagesRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string                 `json:"role"`
	Content []anthropicTextContent `json:"content"`
}

type anthropicTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicMessagesResponse struct {
	Content []anthropicTextContent `json:"content"`
}

// googleGenerateContentRequest & response are minimal shapes for Gemini's generateContent.
type googleGenerateContentRequest struct {
	Contents []googleContent `json:"contents"`
}

type googleContent struct {
	Parts []googlePart `json:"parts"`
}

type googlePart struct {
	Text string `json:"text,omitempty"`
}

type googleGenerateContentResponse struct {
	Candidates []struct {
		Content googleContent `json:"content"`
	} `json:"candidates"`
}

func (c *openAIClient) ExtractFields(ctx context.Context, req ExtractRequest) (ExtractResult, error) {
	// Build a simple, JSON-focused prompt.
	fieldJSON, _ := json.Marshal(req.Fields)
	userContent := fmt.Sprintf("You are a JSON-only extractor. Given markdown content from URL %s and the following field definitions, extract a JSON object with exactly those keys. Fields: %s\n\nMarkdown:\n%s", req.URL, string(fieldJSON), req.Markdown)
	if req.Prompt != "" {
		userContent = req.Prompt + "\n\n" + userContent
	}

	body := openAIChatRequest{
		Model: c.model,
		Messages: []openAIChatMessage{
			{Role: "system", Content: "You are a JSON-only extractor. Respond with a single JSON object and no extra text."},
			{Role: "user", Content: userContent},
		},
		Temperature:    0.0,
		ResponseFormat: &openAIResponseFormat{Type: "json_object"},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return ExtractResult{}, err
	}

	endpoint := c.baseURL
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1"
	}
	endpoint = endpoint + "/chat/completions"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return ExtractResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return ExtractResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ExtractResult{}, fmt.Errorf("openai chat completion failed with status %d", resp.StatusCode)
	}

	var parsed openAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return ExtractResult{}, err
	}
	if len(parsed.Choices) == 0 {
		return ExtractResult{}, errors.New("openai chat completion returned no choices")
	}

	content := parsed.Choices[0].Message.Content

	fields, err := parseJSONFields(content)
	if err != nil {
		if req.Strict {
			return ExtractResult{}, fmt.Errorf("failed to parse JSON from LLM response: %w", err)
		}
		fields = map[string]any{"_raw": content}
	}

	return ExtractResult{Fields: fields}, nil
}

// ExtractFields for anthropicClient uses Anthropic's Messages API.
func (c *anthropicClient) ExtractFields(ctx context.Context, req ExtractRequest) (ExtractResult, error) {
	fieldJSON, _ := json.Marshal(req.Fields)
	userContent := fmt.Sprintf("You are a JSON-only extractor. Given markdown content from URL %s and the following field definitions, extract a JSON object with exactly those keys. Fields: %s\n\nMarkdown:\n%s", req.URL, string(fieldJSON), req.Markdown)
	if req.Prompt != "" {
		userContent = req.Prompt + "\n\n" + userContent
	}

	body := anthropicMessagesRequest{
		Model:     c.model,
		MaxTokens: 512,
		System:    "You are a JSON-only extractor. Respond with a single JSON object and no extra text.",
		Messages: []anthropicMessage{
			{
				Role: "user",
				Content: []anthropicTextContent{
					{Type: "text", Text: userContent},
				},
			},
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return ExtractResult{}, err
	}

	endpoint := "https://api.anthropic.com/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return ExtractResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return ExtractResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ExtractResult{}, fmt.Errorf("anthropic messages request failed with status %d", resp.StatusCode)
	}

	var parsed anthropicMessagesResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return ExtractResult{}, err
	}
	if len(parsed.Content) == 0 {
		return ExtractResult{}, errors.New("anthropic messages returned no content")
	}

	content := parsed.Content[0].Text

	fields, err := parseJSONFields(content)
	if err != nil {
		if req.Strict {
			return ExtractResult{}, fmt.Errorf("failed to parse JSON from LLM response: %w", err)
		}
		fields = map[string]any{"_raw": content}
	}

	return ExtractResult{Fields: fields}, nil
}

// ExtractFields for googleClient uses Gemini's generateContent API.
func (c *googleClient) ExtractFields(ctx context.Context, req ExtractRequest) (ExtractResult, error) {
	fieldJSON, _ := json.Marshal(req.Fields)
	userContent := fmt.Sprintf("You are a JSON-only extractor. Given markdown content from URL %s and the following field definitions, extract a JSON object with exactly those keys. Fields: %s\n\nMarkdown:\n%s", req.URL, string(fieldJSON), req.Markdown)
	if req.Prompt != "" {
		userContent = req.Prompt + "\n\n" + userContent
	}

	body := googleGenerateContentRequest{
		Contents: []googleContent{
			{
				Parts: []googlePart{{Text: userContent}},
			},
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return ExtractResult{}, err
	}

	base := "https://generativelanguage.googleapis.com/v1beta"
	endpoint := fmt.Sprintf("%s/models/%s:generateContent?key=%s", base, c.model, url.QueryEscape(c.apiKey))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return ExtractResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return ExtractResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ExtractResult{}, fmt.Errorf("google generateContent failed with status %d", resp.StatusCode)
	}

	var parsed googleGenerateContentResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return ExtractResult{}, err
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return ExtractResult{}, errors.New("google generateContent returned no candidates")
	}

	// Concatenate all parts' text for simplicity.
	var sb strings.Builder
	for _, part := range parsed.Candidates[0].Content.Parts {
		sb.WriteString(part.Text)
	}
	content := sb.String()

	fields, err := parseJSONFields(content)
	if err != nil {
		if req.Strict {
			return ExtractResult{}, fmt.Errorf("failed to parse JSON from LLM response: %w", err)
		}
		fields = map[string]any{"_raw": content}
	}

	return ExtractResult{Fields: fields}, nil
}
