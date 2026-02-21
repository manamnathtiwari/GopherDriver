package repository

import (
	"context"
	"time"
)

// FileRecord represents a persisted file entry.
type FileRecord struct {
	ID        string
	Hash      string
	Size      int64
	Status    string
	FilePath  string
	CreatedAt time.Time
	Metadata  map[string]interface{} // Flexible JSON storage
}

// Repository is a small, focused interface for file metadata persistence.
// Implementations must honour the supplied context for cancellation and timeouts.
type Repository interface {
	// Create inserts a new file record.
	Create(ctx context.Context, record *FileRecord) error

	// GetByID retrieves a file record by its UUID.
	GetByID(ctx context.Context, id string) (*FileRecord, error)

	// ListAll retrieves all file records (for dashboard display).
	ListAll(ctx context.Context) ([]*FileRecord, error)

	// UpdateStatus sets the processing status for a file.
	UpdateStatus(ctx context.Context, id, status string) error

	// UpdateMetadata sets the computed hash, size, and rich metadata.
	UpdateMetadata(ctx context.Context, id, hash string, size int64, meta map[string]interface{}) error
}
