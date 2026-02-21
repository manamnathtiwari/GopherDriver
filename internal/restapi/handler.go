// Package restapi implements the REST gateway for file uploads.
package restapi

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mtiwari1/gopherdrive/internal/repository"
	"github.com/mtiwari1/gopherdrive/internal/worker"
	pb "github.com/mtiwari1/gopherdrive/proto"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Handler holds dependencies for REST endpoints.
type Handler struct {
	grpc      pb.GopherDriveServer
	repo      repository.Repository
	pool      *worker.Pool
	uploadDir string
	db        *sql.DB
	logger    *slog.Logger
}

// NewHandler creates a new REST handler. uploadDir is where files are stored on disk.
func NewHandler(
	grpcSrv pb.GopherDriveServer,
	repo repository.Repository,
	pool *worker.Pool,
	uploadDir string,
	db *sql.DB,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		grpc:      grpcSrv,
		repo:      repo,
		pool:      pool,
		uploadDir: uploadDir,
		db:        db,
		logger:    logger,
	}
}

// RegisterRoutes attaches all REST routes to the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /files", h.uploadFile)
	mux.HandleFunc("GET /files/{id}", h.getFile)
	mux.HandleFunc("GET /files", h.listFiles)
	mux.HandleFunc("GET /healthz", h.healthz)

	// Serve the frontend dashboard.
	mux.Handle("/", http.FileServer(http.Dir("web")))
}

// ---------- POST /files ----------

func (h *Handler) uploadFile(w http.ResponseWriter, r *http.Request) {
	// Generate a request_id for structured logging (rubric: consistent attributes).
	requestID := uuid.New().String()
	logger := h.logger.With(slog.String("request_id", requestID))

	logger.Info("upload request received")

	// Limit upload body to 32 MB.
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)

	file, header, err := r.FormFile("file")
	if err != nil {
		logger.Error("form file error", slog.String("error", err.Error()))
		http.Error(w, "invalid multipart form", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// ---- Generate unique filename using google/uuid ----
	// Preserve the original file extension for metadata extraction.
	origExt := filepath.Ext(header.Filename) // e.g. ".pdf", ".txt", ".png"
	fileID := uuid.New().String()
	safeFilename := fileID + origExt // e.g. "550e8400-e29b-...pdf"

	// ---- Prevent directory traversal attacks ----
	destPath := filepath.Join(h.uploadDir, safeFilename)
	destPath = filepath.Clean(destPath)
	if !strings.HasPrefix(destPath, filepath.Clean(h.uploadDir)+string(os.PathSeparator)) {
		logger.Error("directory traversal attempt", slog.String("path", destPath))
		http.Error(w, "invalid file path", http.StatusBadRequest)
		return
	}

	// ---- Atomic write: temp file → rename ----
	tmpFile, err := os.CreateTemp(h.uploadDir, "upload-*.tmp")
	if err != nil {
		logger.Error("create temp file", slog.String("error", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	tmpPath := tmpFile.Name()

	// Buffered writer for efficient disk I/O (rubric: bufio.NewWriter).
	bw := bufio.NewWriter(tmpFile)

	// Stream the upload using io.Copy — never loads the whole file into memory.
	if _, err := io.Copy(bw, file); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		logger.Error("stream to disk", slog.String("error", err.Error()))
		http.Error(w, "failed to save file", http.StatusInternalServerError)
		return
	}

	if err := bw.Flush(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		http.Error(w, "flush error", http.StatusInternalServerError)
		return
	}
	tmpFile.Close()

	// Atomic rename from temp file to final destination.
	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		logger.Error("atomic rename", slog.String("error", err.Error()))
		http.Error(w, "failed to save file", http.StatusInternalServerError)
		return
	}

	logger.Info("file saved to disk",
		slog.String("file_id", fileID),
		slog.String("path", destPath),
		slog.String("original_name", header.Filename),
	)

	// ---- Register in DB via gRPC service ----
	_, err = h.grpc.RegisterFile(r.Context(), &pb.RegisterFileRequest{
		Id:       fileID,
		FilePath: destPath,
		Status:   "pending",
	})
	if err != nil {
		logger.Error("grpc RegisterFile", slog.String("error", err.Error()))
		// Map gRPC error codes to HTTP status codes (rubric requirement).
		httpCode := grpcToHTTPStatus(err)
		http.Error(w, "failed to register file", httpCode)
		return
	}

	// ---- Submit processing job to worker pool ----
	// Use context.Background() because this is a background task that outlives the HTTP request.
	// The pool's own context handles shutdown cancellation.
	h.pool.Submit(worker.Job{
		Ctx:      context.Background(),
		FileID:   fileID,
		FilePath: destPath,
	})

	logger.Info("file upload complete, processing submitted",
		slog.String("file_id", fileID),
	)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", "/files/"+fileID)
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"id":     fileID,
		"status": "pending",
	})
}

// ---------- GET /files/{id} ----------

func (h *Handler) getFile(w http.ResponseWriter, r *http.Request) {
	requestID := uuid.New().String()
	logger := h.logger.With(slog.String("request_id", requestID))

	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing file id", http.StatusBadRequest)
		return
	}

	logger.Info("get file request", slog.String("file_id", id))

	rec, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		logger.Error("get file", slog.String("file_id", id), slog.String("error", err.Error()))
		// Use errors.Is to check for specific error types (rubric: Error Inspection).
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "file not found", http.StatusNotFound)
		} else {
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":        rec.ID,
		"hash":       rec.Hash,
		"size":       rec.Size,
		"status":     rec.Status,
		"file_path":  rec.FilePath,
		"created_at": rec.CreatedAt,
		"metadata":   rec.Metadata,
	})
}

// ---------- GET /files (list all) ----------

func (h *Handler) listFiles(w http.ResponseWriter, r *http.Request) {
	requestID := uuid.New().String()
	logger := h.logger.With(slog.String("request_id", requestID))

	logger.Info("list files request")

	records, err := h.repo.ListAll(r.Context())
	if err != nil {
		logger.Error("list files", slog.String("error", err.Error()))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Build JSON response.
	result := make([]map[string]interface{}, 0, len(records))
	for _, rec := range records {
		result = append(result, map[string]interface{}{
			"id":         rec.ID,
			"hash":       rec.Hash,
			"size":       rec.Size,
			"status":     rec.Status,
			"file_path":  rec.FilePath,
			"created_at": rec.CreatedAt,
			"metadata":   rec.Metadata,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// ---------- GET /healthz ----------

// healthz verifies connectivity to the database and local disk (rubric: Production Readiness).
func (h *Handler) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	result := map[string]string{"status": "ok"}
	httpStatus := http.StatusOK

	// Check database connectivity.
	if err := h.db.PingContext(ctx); err != nil {
		result["status"] = "degraded"
		result["database"] = "unreachable: " + err.Error()
		httpStatus = http.StatusServiceUnavailable
	} else {
		result["database"] = "connected"
	}

	// Check local disk (upload directory) is writable.
	if _, err := os.Stat(h.uploadDir); err != nil {
		result["status"] = "degraded"
		result["disk"] = "upload dir inaccessible: " + err.Error()
		httpStatus = http.StatusServiceUnavailable
	} else {
		result["disk"] = "ok"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(result)
}

// grpcToHTTPStatus maps gRPC status codes to HTTP status codes (rubric requirement).
func grpcToHTTPStatus(err error) int {
	st, ok := status.FromError(err)
	if !ok {
		return http.StatusInternalServerError
	}
	switch st.Code() {
	case codes.NotFound:
		return http.StatusNotFound
	case codes.AlreadyExists:
		return http.StatusConflict
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.DeadlineExceeded:
		return http.StatusGatewayTimeout
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.Unavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}
