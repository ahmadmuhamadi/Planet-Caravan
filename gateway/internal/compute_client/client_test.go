package computeclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(url string) *Client {
	c := New(url, "s3cret", time.Second, 2)
	c.backoff = time.Millisecond
	return c
}

// hang blocks until the client gives up. The body must be read first, or the
// server does not notice the client closing the connection.
func hang(r *http.Request) {
	_, _ = io.Copy(io.Discard, r.Body)
	<-r.Context().Done()
}

func TestSendsSecretAndRequestID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Secret") != "s3cret" || r.Header.Get("X-Request-ID") != "req-1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"covered":false,"epochs":[],"releases":[]}`))
	}))
	defer srv.Close()

	ctx := WithRequestID(t.Context(), "req-1")
	got, err := newTestClient(srv.URL).Coverage(ctx, 10, 20)
	if err != nil {
		t.Fatal(err)
	}
	if got.Covered {
		t.Fatal("covered = true, want false")
	}
}

func TestRetriesTransientErrors(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"name":"Vega","ra":279.2,"dec":38.8,"pm":null,"parallax":null,"type":null,"links":[]}`))
	}))
	defer srv.Close()

	got, err := newTestClient(srv.URL).Resolve(t.Context(), "vega")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Vega" || calls.Load() != 3 {
		t.Fatalf("name = %q after %d calls, want Vega after 3", got.Name, calls.Load())
	}
}

func TestGivesUpAfterRetries(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).Resolve(t.Context(), "vega")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("calls = %d, want 3 (1 + 2 retries)", calls.Load())
	}
}

func TestDoesNotRetryClientErrors(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"not_found","message":"No object named zzz","details":{}}}`))
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).Resolve(t.Context(), "zzz")
	var ce *Error
	if !errors.As(err, &ce) || ce.Status != 404 || ce.Code != "not_found" || ce.Message != "No object named zzz" {
		t.Fatalf("err = %v, want compute 404 not_found", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
}

func TestPerCallTimeoutIsRetried(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			hang(r)
			return
		}
		_, _ = w.Write([]byte(`{"covered":true,"epochs":[],"releases":[]}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	c.Timeout = 50 * time.Millisecond
	got, err := c.Coverage(t.Context(), 1, 2)
	if err != nil || !got.Covered {
		t.Fatalf("got %+v, %v; want covered after a retry", got, err)
	}
}

func TestStopsWhenCallerContextEnds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { hang(r) }))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := newTestClient(srv.URL).Process(ctx, ProcessRequest{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("kept retrying after the caller's deadline")
	}
}
