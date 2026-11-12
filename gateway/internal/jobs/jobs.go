// Package jobs runs view jobs: one compute /internal/process call each, bounded
// by the global job timeout (B3).
package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	computeclient "github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/compute_client"
	"github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/store"
)

var duration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "gateway_job_duration_seconds",
	Help:    "View job duration by final status.",
	Buckets: []float64{1, 5, 10, 30, 60, 120, 180, 240, 300, 600},
}, []string{"status"})

// Runner runs jobs in the background and tracks running jobs per client IP (R2).
type Runner struct {
	store        *store.Store
	compute      *computeclient.Client
	timeout      time.Duration
	callbackBase string
	log          *slog.Logger

	ctx    context.Context // cancelled on shutdown
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu    sync.Mutex
	perIP map[string]int
}

// NewRunner returns a runner. callbackBase is the gateway URL compute posts
// progress to; empty disables progress callbacks.
func NewRunner(s *store.Store, c *computeclient.Client, timeout time.Duration, callbackBase string, log *slog.Logger) *Runner {
	ctx, cancel := context.WithCancel(context.Background())
	return &Runner{
		store: s, compute: c, timeout: timeout, callbackBase: strings.TrimRight(callbackBase, "/"), log: log,
		ctx: ctx, cancel: cancel, perIP: map[string]int{},
	}
}

// Stale is how long a queued or running job may go without an update before
// another request may take it over (its gateway probably died).
func (r *Runner) Stale() time.Duration { return r.timeout + time.Minute }

// Reserve takes one of ip's job slots, if fewer than limit are in use.
// Pass the reservation to Start, or give it back with Release.
func (r *Runner) Reserve(ip string, limit int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.perIP[ip] >= limit {
		return false
	}
	r.perIP[ip]++
	return true
}

// Release gives back a slot taken with Reserve.
func (r *Runner) Release(ip string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.perIP[ip]--; r.perIP[ip] <= 0 {
		delete(r.perIP, ip)
	}
}

// Start runs a claimed job in the background and releases ip's slot when it ends.
func (r *Runner) Start(job *store.Job, ip, requestID string) {
	r.wg.Go(func() {
		defer r.Release(ip)
		r.run(job, requestID)
	})
}

// Shutdown cancels running jobs (they fail as "interrupted") and waits for them.
func (r *Runner) Shutdown(ctx context.Context) error {
	r.cancel()
	done := make(chan struct{})
	go func() { r.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Runner) run(job *store.Job, requestID string) {
	start := time.Now()
	log := r.log.With("job_id", job.ID, "request_id", requestID)
	ctx, cancel := context.WithTimeout(computeclient.WithRequestID(r.ctx, requestID), r.timeout)
	defer cancel()

	status := store.StatusDone
	if err := r.process(ctx, job); err != nil {
		status = store.StatusFailed
		je := r.classify(ctx, err)
		log.Error("job failed", "code", je.Code, "err", err)
		// The job context may be over; saving the failure needs its own.
		saveCtx, cancelSave := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancelSave()
		if err := r.store.FailJob(saveCtx, job.ID, je); err != nil {
			log.Error("saving job failure", "err", err)
		}
	}
	elapsed := time.Since(start)
	duration.WithLabelValues(status).Observe(elapsed.Seconds())
	log.Info("job finished", "status", status, "duration_ms", elapsed.Milliseconds())
}

func (r *Runner) process(ctx context.Context, job *store.Job) error {
	if err := r.store.StartJob(ctx, job.ID); err != nil {
		return err
	}
	req := computeclient.ProcessRequest{
		Params:       job.Params,
		OutputPrefix: "views/" + strings.ReplaceAll(job.CacheKey, ":", "/") + "/",
	}
	if r.callbackBase != "" {
		req.CallbackURL = r.callbackBase + "/internal/jobs/" + job.ID + "/progress"
	}
	res, err := r.compute.Process(ctx, req)
	if err != nil {
		return err
	}
	if err := checkFrames(res.Frames); err != nil {
		return err
	}
	_, err = r.store.FinishJob(ctx, job.ID, &store.View{
		CacheKey: job.CacheKey,
		Target:   job.Params.Target,
		Params:   job.Params,
		Epochs:   res.Epochs,
		Frames:   res.Frames,
		Stretch:  res.Stretch,
		Warnings: res.Warnings,
	})
	return err
}

var errBadResponse = errors.New("compute returned an invalid view")

// checkFrames makes sure every frame can be served: a URL-safe unique ID and an object key.
func checkFrames(frames []computeclient.Frame) error {
	seen := map[string]bool{}
	for _, f := range frames {
		if f.ID == "" || strings.ContainsAny(f.ID, "/?#%") || f.ObjectKey == "" || seen[f.ID] {
			return fmt.Errorf("%w: frame %q", errBadResponse, f.ID)
		}
		seen[f.ID] = true
	}
	return nil
}

// classify turns a job error into the stored code and plain message (B3).
func (r *Runner) classify(ctx context.Context, err error) store.JobError {
	var ce *computeclient.Error
	switch {
	case r.ctx.Err() != nil:
		return store.JobError{Code: "interrupted", Message: "The gateway restarted while the job ran. Please try again."}
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return store.JobError{Code: "job_timeout", Message: fmt.Sprintf("The job took longer than %s and was stopped.", r.timeout)}
	case errors.As(err, &ce) && ce.Status >= 400 && ce.Status < 500 && ce.Status != http.StatusTooManyRequests:
		return store.JobError{Code: ce.Code, Message: ce.Message}
	case errors.Is(err, computeclient.ErrUnavailable):
		return store.JobError{Code: "compute_unavailable", Message: "The processing service is unavailable. Please try again later."}
	case errors.Is(err, errBadResponse), errors.As(err, &ce):
		return store.JobError{Code: "compute_error", Message: "The processing service failed to build this view."}
	}
	return store.JobError{Code: "internal", Message: "Something went wrong while building this view."}
}
