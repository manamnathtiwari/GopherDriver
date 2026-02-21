// GopherDrive
//
// Entry point: wires all components together and manages graceful shutdown.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"google.golang.org/grpc"

	grpcserver "github.com/mtiwari1/gopherdrive/internal/grpcserver"
	"github.com/mtiwari1/gopherdrive/internal/repository"
	"github.com/mtiwari1/gopherdrive/internal/restapi"
	"github.com/mtiwari1/gopherdrive/internal/worker"
	pb "github.com/mtiwari1/gopherdrive/proto"
)

const (
	numWorkers = 5
	grpcPort   = ":50051"
	httpPort   = ":8080"
	uploadDir  = "./data"
)

func main() {
	// ── Structured logger ──
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	logger.Info("starting GopherDrive")

	// ── Ensure upload directory exists ──
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		logger.Error("create upload dir", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// ── MySQL connection with pooling ──
	dsn := envOrDefault("DB_DSN", "root:password@tcp(127.0.0.1:3306)/gopherdrive?parseTime=true")
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		logger.Error("open database", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer db.Close()

	// Connection pool tuning.
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)

	if err := db.Ping(); err != nil {
		logger.Error("ping database", slog.String("error", err.Error()))
		os.Exit(1)
	}
	logger.Info("database connected")

	// ── Repository ──
	repo, err := repository.NewMySQLRepo(db)
	if err != nil {
		logger.Error("init repository", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer repo.Close()

	// ── Worker pool (5 bounded goroutines) ──
	pool := worker.NewPool(numWorkers, logger)
	pool.Start()
	logger.Info("worker pool started", slog.Int("workers", numWorkers))

	// ── Results handler goroutine ──
	// Consumes results from the worker pool and updates the database.
	resultsDone := make(chan struct{})
	go func() {
		defer close(resultsDone)
		handleResults(pool.Results(), repo, logger)
	}()

	// ── gRPC server ──
	grpcSrv := grpc.NewServer()
	grpcImpl := grpcserver.NewServer(repo, logger)
	pb.RegisterGopherDriveServer(grpcSrv, grpcImpl)

	lis, err := net.Listen("tcp", grpcPort)
	if err != nil {
		logger.Error("listen gRPC", slog.String("error", err.Error()))
		os.Exit(1)
	}

	go func() {
		logger.Info("gRPC server listening", slog.String("addr", grpcPort))
		if err := grpcSrv.Serve(lis); err != nil {
			logger.Error("gRPC serve", slog.String("error", err.Error()))
		}
	}()

	// ── REST API ──
	handler := restapi.NewHandler(grpcImpl, repo, pool, uploadDir, db, logger)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	httpSrv := &http.Server{
		Addr:         httpPort,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("HTTP server listening", slog.String("addr", httpPort))
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP serve", slog.String("error", err.Error()))
		}
	}()

	// ── Graceful shutdown (SIGINT / SIGTERM) ──
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	logger.Info("shutdown signal received", slog.String("signal", sig.String()))

	// 1. Stop accepting new HTTP requests.
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()

	if err := httpSrv.Shutdown(shutCtx); err != nil {
		logger.Error("HTTP shutdown", slog.String("error", err.Error()))
	}
	logger.Info("HTTP server stopped")

	// 2. Stop gRPC server gracefully.
	grpcSrv.GracefulStop()
	logger.Info("gRPC server stopped")

	// 3. Drain worker pool.
	pool.Shutdown()
	logger.Info("worker pool drained")

	// 4. Wait for results handler to finish.
	<-resultsDone
	logger.Info("results handler finished")

	logger.Info("GopherDrive shutdown complete")
}

// handleResults processes worker results and persists metadata back to the DB.
func handleResults(results <-chan worker.Result, repo repository.Repository, logger *slog.Logger) {
	for res := range results {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)

		if res.Err != nil {
			logger.Error("processing failed for file",
				slog.String("file_id", res.FileID),
				slog.String("error", res.Err.Error()),
			)
			if err := repo.UpdateStatus(ctx, res.FileID, "failed"); err != nil {
				logger.Error("update status to failed", slog.String("error", err.Error()))
			}
			cancel()
			continue
		}

		// Update hash + size + metadata.
		if err := repo.UpdateMetadata(ctx, res.FileID, res.Hash, res.Size, res.Metadata); err != nil {
			logger.Error("update metadata", slog.String("file_id", res.FileID), slog.String("error", err.Error()))
			cancel()
			continue
		}

		// Mark as completed.
		if err := repo.UpdateStatus(ctx, res.FileID, "completed"); err != nil {
			logger.Error("update status to completed", slog.String("file_id", res.FileID), slog.String("error", err.Error()))
		} else {
			logger.Info("file processing completed",
				slog.String("file_id", res.FileID),
				slog.String("hash", res.Hash),
				slog.Int64("size", res.Size),
			)
		}
		cancel()
	}
}

// envOrDefault reads an env variable or returns the fallback.
func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func init() {
	// Suppress unused import warning for fmt.
	_ = fmt.Sprintf
}
