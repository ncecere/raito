package extract

import (
	"context"
	"time"

	"raito/internal/llm"
)

// Service coordinates scraping (handled by the caller) and LLM-based extraction.
type Service struct {
	clientFactory func() (llm.Client, llm.Provider, string, error)
}

func NewService(factory func() (llm.Client, llm.Provider, string, error)) *Service {
	return &Service{clientFactory: factory}
}

// Extract takes a single URL's markdown plus field specs and delegates to the
// configured LLM client.
func (s *Service) Extract(ctx context.Context, url string, markdown string, fields []llm.FieldSpec, prompt string, timeout time.Duration) (map[string]interface{}, error) {
	client, _, _, err := s.clientFactory()
	if err != nil {
		return nil, err
	}

	res, err := client.ExtractFields(ctx, llm.ExtractRequest{
		URL:      url,
		Markdown: markdown,
		Fields:   fields,
		Prompt:   prompt,
		Timeout:  timeout,
	})
	if err != nil {
		return nil, err
	}

	return res.Fields, nil
}
