package crawl

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"raito/internal/crawler"
	"raito/internal/model"
	"raito/internal/scraper"
)

// toString is a helper duplicated from internal/http to avoid circular imports.
func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// Status represents the current state of a crawl job.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

// Job represents a crawl job and its results.
type Job struct {
	ID          string
	URL         string
	Status      Status
	CreatedAt   time.Time
	CompletedAt *time.Time
	Error       string
	Docs        []model.Document
}

type Manager struct {
	mu   sync.RWMutex
	jobs map[string]*Job
}

func NewManager() *Manager {
	return &Manager{jobs: make(map[string]*Job)}
}

// NewJob creates a new crawl job with a uuidv7 ID.
func (m *Manager) NewJob(url string) *Job {
	id := uuidMustV7().String()
	job := &Job{
		ID:        id,
		URL:       url,
		Status:    StatusPending,
		CreatedAt: time.Now().UTC(),
		Docs:      []model.Document{},
	}
	m.mu.Lock()
	m.jobs[id] = job
	m.mu.Unlock()
	return job
}

func (m *Manager) Get(id string) (*Job, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	job, ok := m.jobs[id]
	return job, ok
}

// Start launches a crawl for the given job using the provided scraper and map options.
func (m *Manager) Start(ctx context.Context, job *Job, s scraper.Scraper, opts crawler.MapOptions) {
	go func() {
		m.mu.Lock()
		job.Status = StatusRunning
		m.mu.Unlock()

		// Step 1: discover URLs via Map
		res, err := crawler.Map(ctx, opts)
		if err != nil {
			m.fail(job, err.Error())
			return
		}

		urls := make([]string, 0, len(res.Links)+1)
		urls = append(urls, job.URL)
		for _, l := range res.Links {
			urls = append(urls, l.URL)
		}

		// Step 2: scrape each URL sequentially for now
		documents := make([]model.Document, 0, len(urls))

		for _, u := range urls {
			select {
			case <-ctx.Done():
				m.fail(job, ctx.Err().Error())
				return
			default:
			}

			result, err := s.Scrape(ctx, scraper.Request{URL: u})
			if err != nil {
				// Skip failed URLs for now, but mark job as failed if none succeed
				continue
			}

			md := model.Metadata{
				Title:       toString(result.Metadata["title"]),
				Description: toString(result.Metadata["description"]),
				SourceURL:   toString(result.Metadata["sourceURL"]),
				StatusCode:  result.Status,
			}

			documents = append(documents, model.Document{
				Markdown: result.Markdown,
				HTML:     result.HTML,
				RawHTML:  result.RawHTML,
				Links:    result.Links,
				Engine:   result.Engine,
				Metadata: md,
			})

		}

		m.mu.Lock()
		defer m.mu.Unlock()
		job.Docs = documents
		if len(documents) == 0 {
			job.Status = StatusFailed
			job.Error = "no pages successfully scraped"
		} else {
			job.Status = StatusCompleted
		}
		now := time.Now().UTC()
		job.CompletedAt = &now
	}()
}

func (m *Manager) fail(job *Job, msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job.Status = StatusFailed
	job.Error = msg
	now := time.Now().UTC()
	job.CompletedAt = &now
}

// uuidMustV7 generates a uuidv7 if supported, otherwise falls back to v4.
func uuidMustV7() uuid.UUID {
	// google/uuid v1.6.0+ supports uuid.NewV7
	if newV7, ok := interface{}(uuid.NewV7).(func() (uuid.UUID, error)); ok {
		if id, err := newV7(); err == nil {
			return id
		}
	}
	return uuid.New() // v4 fallback
}
