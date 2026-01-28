package domain

import "time"

// JobStatus represents the status of an import/export job.
type JobStatus string

const (
	JobStatusPending             JobStatus = "pending"
	JobStatusProcessing          JobStatus = "processing"
	JobStatusCompleted           JobStatus = "completed"
	JobStatusCompletedWithErrors JobStatus = "completed_with_errors"
	JobStatusFailed              JobStatus = "failed"
)

// ImportJob represents an import job entity.
type ImportJob struct {
	ID               string                 `json:"id"`
	ResourceType     string                 `json:"resource_type"`
	Status           JobStatus              `json:"status"`
	TotalRecords     int                    `json:"total_records"`
	ProcessedRecords int                    `json:"processed_records"`
	SuccessCount     int                    `json:"success_count"`
	FailureCount     int                    `json:"failure_count"`
	IdempotencyToken string                 `json:"idempotency_token"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	ErrorMessage     *string                `json:"error_message,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
	CompletedAt      *time.Time             `json:"completed_at,omitempty"`
}

// ExportJob represents an export job entity.
type ExportJob struct {
	ID               string     `json:"id"`
	ResourceType     string     `json:"resource_type"`
	Format           string     `json:"format"`
	Status           JobStatus  `json:"status"`
	TotalRecords     int        `json:"total_records"`
	FilePath         string     `json:"file_path,omitempty"`
	IdempotencyToken string     `json:"idempotency_token"`
	ErrorMessage     *string    `json:"error_message,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
}

// RecordError represents a per-record error during import.
type RecordError struct {
	Row    int    `json:"row"`
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

// BatchResult represents the result of processing a single batch.
type BatchResult struct {
	SuccessCount int           `json:"success_count"`
	FailedCount  int           `json:"failed_count"`
	Errors       []RecordError `json:"errors,omitempty"`
}

// ImportResult represents the final result of an import operation.
type ImportResult struct {
	TotalRecords     int           `json:"total_records"`
	ProcessedRecords int           `json:"processed_records"`
	SuccessCount     int           `json:"success_count"`
	FailureCount     int           `json:"failure_count"`
	Errors           []RecordError `json:"errors,omitempty"`
}

// ValidResourceTypes contains all valid resource types.
var ValidResourceTypes = []string{"users", "articles", "comments"}

// ValidFormats contains all valid export formats.
var ValidFormats = []string{"csv", "ndjson"}

// IsValidResourceType checks if a resource type is valid.
func IsValidResourceType(resourceType string) bool {
	for _, rt := range ValidResourceTypes {
		if rt == resourceType {
			return true
		}
	}
	return false
}

// IsValidFormat checks if an export format is valid.
func IsValidFormat(format string) bool {
	for _, f := range ValidFormats {
		if f == format {
			return true
		}
	}
	return false
}
