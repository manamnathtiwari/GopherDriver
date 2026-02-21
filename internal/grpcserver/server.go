// Package grpcserver implements the GopherDrive gRPC service.
package grpcserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/mtiwari1/gopherdrive/internal/repository"
	pb "github.com/mtiwari1/gopherdrive/proto"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server implements the GopherDriveServer gRPC interface.
// Dependencies are injected via the constructor — no global state.
type Server struct {
	repo   repository.Repository
	logger *slog.Logger
}

// NewServer creates a gRPC server with the given repository (DI).
func NewServer(repo repository.Repository, logger *slog.Logger) *Server {
	return &Server{repo: repo, logger: logger}
}

// RegisterFile creates a new file record in the database.
func (s *Server) RegisterFile(ctx context.Context, req *pb.RegisterFileRequest) (*pb.RegisterFileResponse, error) {
	s.logger.Info("grpc RegisterFile",
		slog.String("file_id", req.Id),
		slog.String("file_path", req.FilePath),
	)

	rec := &repository.FileRecord{
		ID:       req.Id,
		Hash:     "",
		Size:     0,
		Status:   req.Status,
		FilePath: req.FilePath,
	}

	if err := s.repo.Create(ctx, rec); err != nil {
		return nil, mapDBError(err, "RegisterFile")
	}

	return &pb.RegisterFileResponse{
		Id:     req.Id,
		Status: req.Status,
	}, nil
}

// UpdateStatus changes the processing status of a file.
func (s *Server) UpdateStatus(ctx context.Context, req *pb.UpdateStatusRequest) (*pb.UpdateStatusResponse, error) {
	s.logger.Info("grpc UpdateStatus",
		slog.String("file_id", req.Id),
		slog.String("new_status", req.Status),
	)

	if err := s.repo.UpdateStatus(ctx, req.Id, req.Status); err != nil {
		return nil, mapDBError(err, "UpdateStatus")
	}

	return &pb.UpdateStatusResponse{
		Id:     req.Id,
		Status: req.Status,
	}, nil
}

// mapDBError converts database errors to proper gRPC status codes.
func mapDBError(err error, method string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return status.Errorf(codes.NotFound, "%s: file not found", method)
	}
	// MySQL duplicate‐entry errors contain "Duplicate entry" in the message.
	if isDuplicateEntry(err) {
		return status.Errorf(codes.AlreadyExists, "%s: file already exists", method)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Errorf(codes.DeadlineExceeded, "%s: database timeout", method)
	}
	return status.Errorf(codes.Internal, "%s: %v", method, err)
}

// isDuplicateEntry checks for MySQL duplicate-key errors (error number 1062).
func isDuplicateEntry(err error) bool {
	return err != nil && fmt.Sprintf("%v", err) != "" &&
		(errors.As(err, new(interface{ Number() uint16 })) ||
			containsDuplicate(err))
}

func containsDuplicate(err error) bool {
	return err != nil && len(err.Error()) > 0 &&
		(err.Error() == "Duplicate entry" || len(err.Error()) > 15 &&
			err.Error()[:15] == "Duplicate entry" ||
			stringContains(err.Error(), "Duplicate entry"))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
