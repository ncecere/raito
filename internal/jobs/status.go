package jobs

// Status represents the lifecycle state of a job in the
// jobs table. These values must match the text values
// stored in the database (jobs.status).
//
// Centralizing these here avoids scattering string
// literals like "pending" or "completed" across
// packages.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)
