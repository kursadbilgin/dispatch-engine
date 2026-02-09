package domain

import "time"

// BatchStatus represents the processing state of a batch.
type BatchStatus string

const (
	BatchStatusProcessing     BatchStatus = "PROCESSING"
	BatchStatusCompleted      BatchStatus = "COMPLETED"
	BatchStatusPartialFailure BatchStatus = "PARTIAL_FAILURE"
)

func (s BatchStatus) String() string { return string(s) }

func (s BatchStatus) IsValid() bool {
	switch s {
	case BatchStatusProcessing, BatchStatusCompleted, BatchStatusPartialFailure:
		return true
	}
	return false
}

// Batch groups multiple notifications submitted together.
type Batch struct {
	ID         string
	TotalCount int
	Status     BatchStatus
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
