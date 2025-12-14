package services

import (
	"context"

	"github.com/google/uuid"

	"raito/internal/store"
)

// BatchScrapeEnqueueRequest encapsulates the information needed to
// enqueue a batch scrape job. Body is serialized as the job input
// (typically a BatchScrapeRequest DTO from the HTTP layer).
type BatchScrapeEnqueueRequest struct {
	ID         uuid.UUID
	PrimaryURL string
	Body       interface{}
}

// BatchScrapeService hides the details of inserting batch scrape jobs
// so HTTP handlers do not talk to the store directly.
type BatchScrapeService interface {
	Enqueue(ctx context.Context, req *BatchScrapeEnqueueRequest) error
}

type batchScrapeService struct {
	st *store.Store
}

func NewBatchScrapeService(st *store.Store) BatchScrapeService {
	return &batchScrapeService{st: st}
}

func (s *batchScrapeService) Enqueue(ctx context.Context, req *BatchScrapeEnqueueRequest) error {
	if req == nil {
		return nil
	}
	_, err := s.st.CreateJob(ctx, req.ID, "batch_scrape", req.PrimaryURL, req.Body, false, 10)
	return err
}
