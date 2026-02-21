// Package worker implements a bounded worker pool for concurrent file metadata processing.
package worker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/mtiwari1/gopherdrive/internal/hasher"
)

// Job represents a file processing request.
// Contains a context.Context for cancellation and deadline propagation.
type Job struct {
	Ctx      context.Context
	FileID   string
	FilePath string
}

// Result holds the outcome of processing a single job.
type Result struct {
	FileID    string
	Hash      string
	Size      int64
	Extension string
	Metadata  map[string]interface{}
	Err       error
}

// Pool manages a fixed set of worker goroutines that process Jobs from a channel
// and emit Results to another channel.
type Pool struct {
	workers int
	jobs    chan Job
	results chan Result
	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
	logger  *slog.Logger
}

// NewPool creates a pool with the given number of workers.
// Call Start() to launch the goroutines.
func NewPool(workers int, logger *slog.Logger) *Pool {
	ctx, cancel := context.WithCancel(context.Background())
	return &Pool{
		workers: workers,
		jobs:    make(chan Job, workers*2),   // small buffer for backpressure
		results: make(chan Result, workers*2),
		ctx:     ctx,
		cancel:  cancel,
		logger:  logger,
	}
}

// Start launches worker goroutines. Each reads from the jobs channel until it is
// closed or the context is cancelled.
func (p *Pool) Start() {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
}

// Submit enqueues a job. It blocks if the jobs channel buffer is full (backpressure).
// Returns false if the pool context is already cancelled.
func (p *Pool) Submit(job Job) bool {
	select {
	case p.jobs <- job:
		return true
	case <-p.ctx.Done():
		return false
	}
}

// Results returns the read-only results channel for the consumer.
func (p *Pool) Results() <-chan Result {
	return p.results
}

// Shutdown closes the jobs channel, waits for all workers to finish,
// then closes the results channel. Safe to call once.
func (p *Pool) Shutdown() {
	close(p.jobs) // signal workers to drain and exit
	p.wg.Wait()   // wait for all workers to complete
	close(p.results)
}

// worker is the goroutine body. It processes jobs until the channel is closed
// or the context is cancelled, preventing goroutine leaks.
func (p *Pool) worker(id int) {
	defer p.wg.Done()

	for {
		select {
		case job, ok := <-p.jobs:
			if !ok {
				// Channel closed — exit cleanly.
				p.logger.Info("worker exiting", slog.Int("worker_id", id))
				return
			}
			p.process(id, job)

		case <-p.ctx.Done():
			p.logger.Info("worker cancelled", slog.Int("worker_id", id))
			return
		}
	}
}

// process handles a single job: logs start/end, computes metadata, sends result.
// Respects the job's context for cancellation.
func (p *Pool) process(workerID int, job Job) {
	// Use the job's context; fall back to background if nil.
	ctx := job.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// Check if context is already cancelled before doing work.
	if err := ctx.Err(); err != nil {
		p.results <- Result{FileID: job.FileID, Err: fmt.Errorf("job cancelled before processing: %w", err)}
		return
	}

	start := time.Now()
	p.logger.Info("processing started",
		slog.Int("worker_id", workerID),
		slog.String("file_id", job.FileID),
		slog.Time("start_time", start),
	)

	meta, err := hasher.ComputeMetadata(job.FilePath)

	end := time.Now()
	latency := end.Sub(start)

	// Check if context was cancelled during processing.
	if ctx.Err() != nil {
		p.logger.Warn("job context cancelled during processing",
			slog.Int("worker_id", workerID),
			slog.String("file_id", job.FileID),
		)
		p.results <- Result{FileID: job.FileID, Err: fmt.Errorf("job cancelled during processing: %w", ctx.Err())}
		return
	}

	if err != nil {
		p.logger.Error("processing failed",
			slog.Int("worker_id", workerID),
			slog.String("file_id", job.FileID),
			slog.Duration("latency", latency),
			slog.String("error", err.Error()),
		)
		p.results <- Result{FileID: job.FileID, Err: err}
		return
	}

	p.logger.Info("processing completed",
		slog.Int("worker_id", workerID),
		slog.String("file_id", job.FileID),
		slog.Time("end_time", end),
		slog.Duration("latency", latency),
		slog.String("hash", meta.Hash),
		slog.Int64("size", meta.Size),
		slog.String("extension", meta.Extension),
	)

	p.results <- Result{
		FileID:    job.FileID,
		Hash:      meta.Hash,
		Size:      meta.Size,
		Extension: meta.Extension,
		Metadata:  meta.Extra,
	}
}
