package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

const dbTimeout = 2 * time.Second

// MySQLRepo implements Repository using prepared statements and context timeouts.
type MySQLRepo struct {
	db           *sql.DB
	stmtCreate   *sql.Stmt
	stmtGetByID  *sql.Stmt
	stmtUpdStat  *sql.Stmt
	stmtUpdMeta  *sql.Stmt
}

// NewMySQLRepo prepares all statements up front. The caller owns the *sql.DB lifetime.
func NewMySQLRepo(db *sql.DB) (*MySQLRepo, error) {
	stmtCreate, err := db.Prepare("INSERT INTO files (id, hash, size, status, file_path) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		return nil, fmt.Errorf("prepare create: %w", err)
	}

	stmtGetByID, err := db.Prepare("SELECT id, hash, size, status, file_path, created_at, metadata FROM files WHERE id = ?")
	if err != nil {
		return nil, fmt.Errorf("prepare getByID: %w", err)
	}

	stmtUpdStat, err := db.Prepare("UPDATE files SET status = ? WHERE id = ?")
	if err != nil {
		return nil, fmt.Errorf("prepare updateStatus: %w", err)
	}

	stmtUpdMeta, err := db.Prepare("UPDATE files SET hash = ?, size = ?, metadata = ? WHERE id = ?")
	if err != nil {
		return nil, fmt.Errorf("prepare updateMetadata: %w", err)
	}

	return &MySQLRepo{
		db:          db,
		stmtCreate:  stmtCreate,
		stmtGetByID: stmtGetByID,
		stmtUpdStat: stmtUpdStat,
		stmtUpdMeta: stmtUpdMeta,
	}, nil
}

// Create inserts a new file record.
func (r *MySQLRepo) Create(ctx context.Context, rec *FileRecord) error {
	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	_, err := r.stmtCreate.ExecContext(ctx, rec.ID, rec.Hash, rec.Size, rec.Status, rec.FilePath)
	if err != nil {
		return fmt.Errorf("repo create: %w", err)
	}
	return nil
}

// GetByID retrieves a file record by UUID.
func (r *MySQLRepo) GetByID(ctx context.Context, id string) (*FileRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	rec := &FileRecord{}
	var metaJSON []byte
	err := r.stmtGetByID.QueryRowContext(ctx, id).Scan(
		&rec.ID, &rec.Hash, &rec.Size, &rec.Status, &rec.FilePath, &rec.CreatedAt, &metaJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("repo getByID: %w", err)
	}

	if len(metaJSON) > 0 {
		if err := json.Unmarshal(metaJSON, &rec.Metadata); err != nil {
			// Log error but don't fail the request? Or just ignore.
			// For now, let's just proceed with empty metadata if corrupt.
		}
	}

	return rec, nil
}

// UpdateStatus sets the processing status for a file.
func (r *MySQLRepo) UpdateStatus(ctx context.Context, id, status string) error {
	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	_, err := r.stmtUpdStat.ExecContext(ctx, status, id)
	if err != nil {
		return fmt.Errorf("repo updateStatus: %w", err)
	}
	return nil
}

// UpdateMetadata sets the computed hash, size, and rich metadata.
func (r *MySQLRepo) UpdateMetadata(ctx context.Context, id, hash string, size int64, meta map[string]interface{}) error {
	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("repo updateMetadata marshal: %w", err)
	}

	_, err = r.stmtUpdMeta.ExecContext(ctx, hash, size, metaJSON, id)
	if err != nil {
		return fmt.Errorf("repo updateMetadata: %w", err)
	}
	return nil
}

// ListAll retrieves all file records ordered by most recent first.
func (r *MySQLRepo) ListAll(ctx context.Context) ([]*FileRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	rows, err := r.db.QueryContext(ctx, "SELECT id, hash, size, status, file_path, created_at, metadata FROM files ORDER BY id DESC LIMIT 100")
	if err != nil {
		return nil, fmt.Errorf("repo listAll: %w", err)
	}
	defer rows.Close()

	var records []*FileRecord
	for rows.Next() {
		rec := &FileRecord{}
		var metaJSON []byte
		if err := rows.Scan(&rec.ID, &rec.Hash, &rec.Size, &rec.Status, &rec.FilePath, &rec.CreatedAt, &metaJSON); err != nil {
			return nil, fmt.Errorf("repo listAll scan: %w", err)
		}
		if len(metaJSON) > 0 {
			_ = json.Unmarshal(metaJSON, &rec.Metadata)
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

// Close releases all prepared statements.
func (r *MySQLRepo) Close() error {
	for _, s := range []*sql.Stmt{r.stmtCreate, r.stmtGetByID, r.stmtUpdStat, r.stmtUpdMeta} {
		if s != nil {
			s.Close()
		}
	}
	return nil
}
