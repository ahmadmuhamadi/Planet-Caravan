package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func testConfig() Config {
	return Config{
		ResolveTTL: time.Hour, CoverageTTL: time.Hour, MaxBodyBytes: 1 << 10, ComputeSecret: "s3cret",
		CORSOrigins:   []string{"https://planet-caravan.example"},
		RatePerSecond: 100, RateBurst: 100, JobsPerMinute: 600, JobBurst: 100, MaxJobsPerIP: 10,
	}
}

func do(t *testing.T, h http.Handler, method, path, body string, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// errorCode checks the CONTRACT error shape (B13) and returns its code.
func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error *struct {
			Code    string         `json:"code"`
			Message string         `json:"message"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Error == nil ||
		body.Error.Code == "" || body.Error.Message == "" || body.Error.Details == nil {
		t.Fatalf("status %d body %s is not {error: {code, message, details}}", rec.Code, rec.Body)
	}
	return body.Error.Code
}

func bareRouter(t *testing.T, cfg Config) http.Handler {
	t.Helper()
	r, err := New(cfg, Deps{Log: quiet})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestErrorFormat(t *testing.T) {
	r := bareRouter(t, testConfig())
	for _, tc := range []struct {
		method, path, body string
		status             int
		code               string
	}{
		{"GET", "/nope", "", 404, "not_found"},
		{"DELETE", "/api/v1/views", "", 405, "method_not_allowed"},
		{"GET", "/api/v1/coverage?ra=400&dec=0", "", 400, "invalid_request"},
		{"GET", "/api/v1/resolve", "", 400, "invalid_request"},
		{"GET", "/api/v1/views?ra=1", "", 400, "invalid_request"},
		{"POST", "/api/v1/views", `{"target":{"ra":1,"dec":1},"fov_arcmin":31}`, 400, "invalid_request"},
		{"POST", "/api/v1/views", `{"target":{"name":"` + strings.Repeat("x", 2000) + `"}}`, 413, "payload_too_large"},
		{"POST", "/internal/jobs/x/progress", `{"progress":5}`, 401, "unauthorized"},
	} {
		rec := do(t, r, tc.method, tc.path, tc.body)
		if rec.Code != tc.status {
			t.Errorf("%s %s = %d, want %d", tc.method, tc.path, rec.Code, tc.status)
			continue
		}
		if got := errorCode(t, rec); got != tc.code {
			t.Errorf("%s %s code = %s, want %s", tc.method, tc.path, got, tc.code)
		}
	}
}

func TestHealthzAndRequestID(t *testing.T) {
	r := bareRouter(t, testConfig())
	rec := do(t, r, "GET", "/api/v1/healthz", "", "X-Request-ID", "abc-123")
	if rec.Code != 200 || rec.Header().Get("X-Request-ID") != "abc-123" {
		t.Fatalf("status %d, request id %q", rec.Code, rec.Header().Get("X-Request-ID"))
	}
	rec = do(t, r, "GET", "/api/v1/healthz", "", "X-Request-ID", "bad id with spaces")
	if id := rec.Header().Get("X-Request-ID"); len(id) != 32 {
		t.Fatalf("invalid request id was not replaced: %q", id)
	}
}

func TestCORS(t *testing.T) {
	r := bareRouter(t, testConfig())
	rec := do(t, r, "OPTIONS", "/api/v1/views", "",
		"Origin", "https://planet-caravan.example", "Access-Control-Request-Method", "POST")
	if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Origin") != "https://planet-caravan.example" ||
		!strings.Contains(rec.Header().Get("Access-Control-Allow-Methods"), "POST") {
		t.Fatalf("allowed preflight: %d %v", rec.Code, rec.Header())
	}
	rec = do(t, r, "OPTIONS", "/api/v1/views", "", "Origin", "https://evil.example", "Access-Control-Request-Method", "POST")
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("other origin was allowed")
	}
	rec = do(t, r, "GET", "/api/v1/healthz", "", "Origin", "https://evil.example")
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("other origin was allowed on GET")
	}
}

func TestRateLimit(t *testing.T) {
	cfg := testConfig()
	cfg.RatePerSecond, cfg.RateBurst = 1, 2
	r := bareRouter(t, cfg)
	path := "/api/v1/coverage?ra=400&dec=0" // fails validation, but only after the limiter
	for i := range 2 {
		if rec := do(t, r, "GET", path, ""); rec.Code != 400 {
			t.Fatalf("request %d = %d, want 400", i, rec.Code)
		}
	}
	rec := do(t, r, "GET", path, "")
	if rec.Code != 429 || errorCode(t, rec) != "rate_limited" || rec.Header().Get("Retry-After") != "1" {
		t.Fatalf("third request = %d, Retry-After %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	// Health checks are not limited.
	if rec := do(t, r, "GET", "/api/v1/healthz", ""); rec.Code != 200 {
		t.Fatalf("healthz = %d", rec.Code)
	}
}

func TestIPLimiterRefillsAndSeparatesIPs(t *testing.T) {
	l := newIPLimiter(1, 1)
	now := time.Now()
	if ok, _ := l.allow("a", now); !ok {
		t.Fatal("first request denied")
	}
	if ok, wait := l.allow("a", now); ok || wait <= 0 || wait > time.Second {
		t.Fatalf("second request: ok=%v wait=%v", ok, wait)
	}
	if ok, _ := l.allow("b", now); !ok {
		t.Fatal("other IP shares the bucket")
	}
	if ok, _ := l.allow("a", now.Add(time.Second)); !ok {
		t.Fatal("bucket did not refill")
	}
	l.allow("c", now.Add(time.Hour)) // triggers a sweep
	if len(l.buckets) != 1 {
		t.Fatalf("idle buckets kept: %d", len(l.buckets))
	}
}
