package services

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"raito/internal/store"
)

// ExtractRequest is the internal representation of an extract
// enqueue request used by ExtractService.
type ExtractRequest struct {
	ID         uuid.UUID
	Body       any
	PrimaryURL string
	TenantID   *uuid.UUID
	APIKeyID   *uuid.UUID
}

// ExtractService encapsulates the business logic for enqueuing
// extract jobs. It mirrors the behavior of the HTTP handler but
// is decoupled from Fiber.
type ExtractService interface {
	Enqueue(ctx context.Context, req *ExtractRequest) error
}

type extractService struct {
	st *store.Store
}

// NewExtractService constructs an ExtractService backed by the
// store layer.
func NewExtractService(st *store.Store) ExtractService {
	return &extractService{st: st}
}

func (s *extractService) Enqueue(ctx context.Context, req *ExtractRequest) error {
	if req == nil {
		return errors.New("nil extract request")
	}
	if req.ID == uuid.Nil {
		return errors.New("extract id is required")
	}
	if req.PrimaryURL == "" {
		return errors.New("primary url is required")
	}

	_, err := s.st.CreateJob(ctx, req.ID, "extract", req.PrimaryURL, req.Body, false, 10, req.TenantID, req.APIKeyID)
	return err
}
