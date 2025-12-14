package services

import (
	"context"

	"github.com/google/uuid"

	"raito/internal/store"
)

// CrawlEnqueueRequest captures the minimal information needed to enqueue
// a crawl job. The Body field is serialized as the job's input payload
// (typically a CrawlRequest DTO from the HTTP layer).
type CrawlEnqueueRequest struct {
	ID   uuid.UUID
	URL  string
	Body interface{}
}

// CrawlService encapsulates the persistence of crawl jobs so HTTP
// handlers do not depend directly on the store implementation.
type CrawlService interface {
	Enqueue(ctx context.Context, req *CrawlEnqueueRequest) error
}

type crawlService struct {
	st *store.Store
}

func NewCrawlService(st *store.Store) CrawlService {
	return &crawlService{st: st}
}

func (s *crawlService) Enqueue(ctx context.Context, req *CrawlEnqueueRequest) error {
	if req == nil {
		return nil
	}
	_, err := s.st.CreateCrawlJob(ctx, req.ID, req.URL, req.Body)
	return err
}
