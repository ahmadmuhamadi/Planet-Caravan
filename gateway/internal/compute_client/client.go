// Package computeclient calls the private compute service.
package computeclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var callDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "gateway_compute_request_duration_seconds",
	Help:    "Compute call latency per attempt, by endpoint and outcome.",
	Buckets: []float64{.05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60, 120, 300},
}, []string{"endpoint", "outcome"})

// ErrUnavailable means compute could not be reached or kept failing.
var ErrUnavailable = errors.New("compute unavailable")

// Error is a non-retryable error response from compute.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("compute: %d %s: %s", e.Status, e.Code, e.Message)
}

type ctxKey struct{}

// WithRequestID stores the request ID sent as X-Request-ID.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// RequestID returns the request ID stored by WithRequestID.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// Client calls compute with the shared secret, a per-call timeout, and
// retries with exponential backoff and full jitter.
type Client struct {
	BaseURL string
	Secret  string
	Timeout time.Duration // per attempt; process uses the job deadline instead
	Retries int           // extra attempts after the first
	HTTP    *http.Client
	backoff time.Duration // first backoff step; tests shorten it
}

// New returns a client for compute at baseURL.
func New(baseURL, secret string, timeout time.Duration, retries int) *Client {
	return &Client{
		BaseURL: baseURL,
		Secret:  secret,
		Timeout: timeout,
		Retries: retries,
		HTTP:    &http.Client{},
		backoff: 250 * time.Millisecond,
	}
}

// Resolve resolves an object name. Unknown names return *Error with status 404.
func (c *Client) Resolve(ctx context.Context, name string) (*ResolveResponse, error) {
	var out ResolveResponse
	return &out, c.post(ctx, "/internal/resolve", ResolveRequest{Name: name}, &out, c.Timeout)
}

// Coverage lists SPHEREx epochs and releases at a position.
func (c *Client) Coverage(ctx context.Context, ra, dec float64) (*CoverageResponse, error) {
	var out CoverageResponse
	return &out, c.post(ctx, "/internal/coverage", CoverageRequest{RA: ra, Dec: dec}, &out, c.Timeout)
}

// Process renders a view. It runs until ctx ends, which the caller bounds by the job timeout.
func (c *Client) Process(ctx context.Context, req ProcessRequest) (*ProcessResponse, error) {
	var out ProcessResponse
	return &out, c.post(ctx, "/internal/process", req, &out, 0)
}

func (c *Client) post(ctx context.Context, path string, in, out any, timeout time.Duration) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			// Full jitter: sleep a random time up to backoff * 2^(attempt-1), capped at 5s.
			limit := min(c.backoff<<(attempt-1), 5*time.Second)
			select {
			case <-ctx.Done():
				return errors.Join(lastErr, ctx.Err())
			case <-time.After(rand.N(limit) + 1):
			}
		}
		retry, err := c.attempt(ctx, path, body, out, timeout)
		if err == nil || !retry || ctx.Err() != nil {
			return err
		}
		lastErr = err
	}
	return errors.Join(ErrUnavailable, lastErr)
}

// attempt makes one call. It reports whether a failure is worth retrying.
func (c *Client) attempt(ctx context.Context, path string, body []byte, out any, timeout time.Duration) (bool, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", c.Secret)
	if id := RequestID(ctx); id != "" {
		req.Header.Set("X-Request-ID", id)
	}

	start := time.Now()
	resp, err := c.HTTP.Do(req)
	if err != nil {
		callDuration.WithLabelValues(path, "network_error").Observe(time.Since(start).Seconds())
		return true, err // post stops retrying if the caller's context ended
	}
	defer func() { _ = resp.Body.Close() }()
	callDuration.WithLabelValues(path, strconv.Itoa(resp.StatusCode)).Observe(time.Since(start).Seconds())

	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return true, err
	}
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(data, out); err != nil {
			return false, &Error{Status: resp.StatusCode, Code: "compute_error", Message: "invalid response: " + err.Error()}
		}
		return false, nil
	}

	e := &Error{Status: resp.StatusCode, Code: "compute_error", Message: http.StatusText(resp.StatusCode)}
	var eb ErrorBody
	if json.Unmarshal(data, &eb) == nil && eb.Error.Code != "" {
		e.Code, e.Message = eb.Error.Code, eb.Error.Message
	}
	switch resp.StatusCode {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true, e
	}
	return false, e
}
