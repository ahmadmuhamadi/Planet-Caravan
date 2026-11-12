package api

// Integration tests: real Postgres and Redis, stubbed compute, fake object
// storage. No network beyond localhost. They are skipped unless
// TEST_DATABASE_URL and TEST_REDIS_URL are set (see `make test-integration`).

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/cache"
	computeclient "github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/compute_client"
	"github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/jobs"
	"github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/store"
)

// stubCompute fakes the compute internal API.
type stubCompute struct {
	resolveCalls, coverageCalls, processCalls atomic.Int32
	uncovered                                 atomic.Bool
	process                                   atomic.Pointer[processFunc] // nil: render a one-frame view
	lastProcess                               atomic.Pointer[computeclient.ProcessRequest]
}

type processFunc func(w http.ResponseWriter, r *http.Request, req computeclient.ProcessRequest)

func (s *stubCompute) onProcess(f processFunc) {
	if f == nil {
		s.process.Store(nil)
		return
	}
	s.process.Store(&f)
}

func (s *stubCompute) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Internal-Secret") != "s3cret" || r.Header.Get("X-Request-ID") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	switch r.URL.Path {
	case "/internal/resolve":
		s.resolveCalls.Add(1)
		var req computeclient.ResolveRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if !strings.EqualFold(req.Name, "Barnard's Star") { // SIMBAD ignores case
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprintf(w, `{"error":{"code":"not_found","message":"No object named %s.","details":{}}}`, req.Name)
			return
		}
		_, _ = w.Write([]byte(`{"name":"Barnard's Star","ra":269.4520769,"dec":4.6933649,
			"pm":{"pmra":-801.551,"pmdec":10362.394},"parallax":546.976,"type":"BYDraV*","links":[]}`))
	case "/internal/coverage":
		s.coverageCalls.Add(1)
		if s.uncovered.Load() {
			_, _ = w.Write([]byte(`{"covered":false,"epochs":[],"releases":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"covered":true,
			"epochs":[{"index":0,"date":"2025-05-10","mjd":60805.1,"detectors":[1,2]}],
			"releases":[{"name":"QR3","collection":"spherex_qr3"},{"name":"QR2","collection":"spherex_qr2"}]}`))
	case "/internal/process":
		s.processCalls.Add(1)
		var req computeclient.ProcessRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		s.lastProcess.Store(&req)
		if f := s.process.Load(); f != nil {
			(*f)(w, r, req)
			return
		}
		writeView(w, req)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func writeView(w http.ResponseWriter, req computeclient.ProcessRequest) {
	res := computeclient.ProcessResponse{
		Epochs:   []computeclient.Epoch{{Index: 0, Date: "2025-05-10", MJD: 60805.1, Detectors: []int{1}}},
		Stretch:  computeclient.Stretch{Type: "asinh", PMin: 1, PMax: 99.5},
		Warnings: []string{},
		Frames: []computeclient.Frame{{
			FrameInfo: computeclient.FrameInfo{ID: "e0-d1", Date: "2025-05-10", MJD: 60805.1, Detector: 1,
				WavelengthMinUM: 0.75, WavelengthMaxUM: 0.79, CenterRA: req.Params.Target.RA, CenterDec: req.Params.Target.Dec,
				PixelScaleArcsec: 6.2, Width: 30, Height: 30, Warnings: []string{"saturated"}},
			ObjectKey: req.OutputPrefix + "e0-d1.png",
		}},
	}
	_ = json.NewEncoder(w).Encode(res)
}

type memBlobs struct{ objects sync.Map }

func (m *memBlobs) Open(_ context.Context, key string) (io.ReadCloser, int64, error) {
	v, ok := m.objects.Load(key)
	if !ok {
		return nil, 0, store.ErrNotFound
	}
	b := v.([]byte)
	return io.NopCloser(strings.NewReader(string(b))), int64(len(b)), nil
}

func (m *memBlobs) Ping(context.Context) error { return nil }

type env struct {
	router  http.Handler
	compute *stubCompute
	srv     *httptest.Server
	store   *store.Store
	blobs   *memBlobs
	runner  *jobs.Runner
}

func newEnv(t *testing.T, cfg Config, jobTimeout time.Duration) *env {
	t.Helper()
	dbURL, redisURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_REDIS_URL")
	if dbURL == "" || redisURL == "" {
		t.Skip("TEST_DATABASE_URL and TEST_REDIS_URL not set")
	}
	db, err := store.Open(t.Context(), dbURL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.Exec(t.Context(), `TRUNCATE jobs, views, releases`); err != nil {
		t.Fatal(err)
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(opts)
	if err := rdb.FlushDB(t.Context()).Err(); err != nil {
		t.Fatal(err)
	}

	stub := &stubCompute{}
	srv := httptest.NewServer(stub)
	compute := computeclient.New(srv.URL, "s3cret", time.Second, 1)
	runner := jobs.NewRunner(db, compute, jobTimeout, "http://gateway.internal", quiet)
	blobs := &memBlobs{}
	r, err := New(cfg, Deps{Store: db, Blobs: blobs, Cache: &cache.Redis{Client: rdb, Log: quiet}, Compute: compute, Jobs: runner, Log: quiet})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = runner.Shutdown(context.Background())
		srv.Close()
		_ = rdb.Close()
		db.Close()
	})
	return &env{router: r, compute: stub, srv: srv, store: db, blobs: blobs, runner: runner}
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return v
}

type jobStatus struct {
	ID       string          `json:"id"`
	Status   string          `json:"status"`
	Progress int             `json:"progress"`
	Result   *view           `json:"result"`
	Error    *store.JobError `json:"error"`
}

// waitJob polls GET /jobs/{id} until the job ends.
func (e *env) waitJob(t *testing.T, id string) jobStatus {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rec := do(t, e.router, "GET", "/api/v1/jobs/"+id, "")
		if rec.Code != 200 {
			t.Fatalf("GET job = %d %s", rec.Code, rec.Body)
		}
		js := decode[jobStatus](t, rec)
		if js.Status == "done" || js.Status == "failed" {
			return js
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("job did not finish")
	return jobStatus{}
}

const barnard = `{"target":{"name":"barnard's   star"},"fov_arcmin":3}`

func TestViewLifecycle(t *testing.T) {
	e := newEnv(t, testConfig(), 5*time.Second)

	rec := do(t, e.router, "POST", "/api/v1/views", barnard)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST /views = %d %s", rec.Code, rec.Body)
	}
	acc := decode[map[string]string](t, rec)
	if acc["job_id"] == "" || acc["status_url"] != "/api/v1/jobs/"+acc["job_id"] || rec.Header().Get("Location") != acc["status_url"] {
		t.Fatalf("202 body = %s", rec.Body)
	}

	js := e.waitJob(t, acc["job_id"])
	if js.Status != "done" || js.Progress != 100 || js.Result == nil {
		t.Fatalf("job = %+v", js)
	}
	v := js.Result
	if v.Target.Name != "Barnard's Star" || v.FOVArcmin != 3 || v.Mode != "blink" || len(v.Frames) != 1 || v.Stretch.Type != "asinh" {
		t.Fatalf("view = %+v", v)
	}
	// Coordinates are rounded to 0.1" before processing (B5).
	if ra, _ := cache.RoundCoords(269.4520769, 0); v.Target.RA != ra {
		t.Errorf("target ra = %v, want rounded %v", v.Target.RA, ra)
	}

	// Compute got the normalised params, the release set, and a callback URL.
	pr := e.compute.lastProcess.Load()
	if strings.Join(pr.Params.Collections, ",") != "spherex_qr2,spherex_qr3" ||
		pr.CallbackURL != "http://gateway.internal/internal/jobs/"+js.ID+"/progress" ||
		!strings.HasPrefix(pr.OutputPrefix, "views/v1/") {
		t.Errorf("process request = %+v", pr)
	}

	// The stored frame does not leak its object key.
	rec = do(t, e.router, "GET", "/api/v1/views/"+v.ID, "")
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "object_key") {
		t.Fatalf("GET view = %d %s", rec.Code, rec.Body)
	}
	for _, field := range []string{`"alignment_residual_arcsec":null`, `"warnings":["saturated"]`, `"pixel_scale_arcsec":6.2`} {
		if !strings.Contains(rec.Body.String(), field) {
			t.Errorf("view JSON lacks %s", field)
		}
	}

	// Frame PNG comes from object storage.
	png := []byte("\x89PNG fake")
	e.blobs.objects.Store(pr.OutputPrefix+"e0-d1.png", png)
	rec = do(t, e.router, "GET", "/api/v1/views/"+v.ID+"/frames/e0-d1.png", "")
	if rec.Code != 200 || rec.Body.String() != string(png) || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("frame = %d %q %v", rec.Code, rec.Body, rec.Header())
	}
	for path, code := range map[string]int{
		"/api/v1/views/" + v.ID + "/frames/e0-d2.png":                         404,
		"/api/v1/views/" + v.ID + "/frames/e0-d1":                             404,
		"/api/v1/views/00000000-0000-0000-0000-000000000000/frames/e0-d1.png": 404,
		"/api/v1/views/not-a-uuid":                                            404,
		"/api/v1/jobs/not-a-uuid":                                             404,
	} {
		if rec := do(t, e.router, "GET", path, ""); rec.Code != code || errorCode(t, rec) != "not_found" {
			t.Errorf("GET %s = %d", path, rec.Code)
		}
	}

	// The same request is now a cache hit (200), from cached resolve + coverage.
	resolves, coverages := e.compute.resolveCalls.Load(), e.compute.coverageCalls.Load()
	rec = do(t, e.router, "POST", "/api/v1/views", `{"target":{"name":"Barnard's Star"}}`)
	if rec.Code != 200 || decode[view](t, rec).ID != v.ID {
		t.Fatalf("second POST = %d %s", rec.Code, rec.Body)
	}
	if e.compute.resolveCalls.Load() != resolves || e.compute.coverageCalls.Load() != coverages || e.compute.processCalls.Load() != 1 {
		t.Error("cached request called compute")
	}

	// GET /views uses the same key; a different fov is not built yet.
	q := fmt.Sprintf("/api/v1/views?ra=%v&dec=%v&fov=3&mode=blink", 269.4520769, 4.6933649)
	if rec := do(t, e.router, "GET", q, ""); rec.Code != 200 || decode[view](t, rec).ID != v.ID {
		t.Fatalf("GET %s = %d %s", q, rec.Code, rec.Body)
	}
	if rec := do(t, e.router, "GET", strings.Replace(q, "fov=3", "fov=5", 1), ""); rec.Code != 404 {
		t.Fatalf("other fov = %d", rec.Code)
	}

	// With compute gone, the cached view still loads (R5 groundwork).
	e.srv.Close()
	if rec := do(t, e.router, "POST", "/api/v1/views", barnard); rec.Code != 200 {
		t.Fatalf("POST with compute down = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, e.router, "GET", "/api/v1/views/"+v.ID+"/frames/e0-d1.png", ""); rec.Code != 200 {
		t.Fatalf("frame with compute down = %d", rec.Code)
	}
}

func TestIdenticalRequestsShareOneJob(t *testing.T) {
	e := newEnv(t, testConfig(), 5*time.Second)
	release := make(chan struct{})
	e.compute.onProcess(func(w http.ResponseWriter, _ *http.Request, req computeclient.ProcessRequest) {
		<-release
		writeView(w, req)
	})

	// Warm resolve/coverage so the requests race on the job, not on compute lookups.
	if rec := do(t, e.router, "GET", "/api/v1/coverage?ra=10&dec=20", ""); rec.Code != 200 {
		t.Fatal(rec.Body)
	}
	const n = 8
	ids := make([]string, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			// Positions differ by less than 0.05", so they share a cache key (B5).
			body := fmt.Sprintf(`{"target":{"ra":%v,"dec":20}}`, 10+float64(i)*1e-6)
			rec := do(t, e.router, "POST", "/api/v1/views", body)
			if rec.Code != http.StatusAccepted {
				t.Errorf("POST %d = %d %s", i, rec.Code, rec.Body)
				return
			}
			ids[i] = decode[map[string]string](t, rec)["job_id"]
		})
	}
	wg.Wait()
	for _, id := range ids {
		if id != ids[0] {
			t.Fatalf("job ids differ: %v", ids)
		}
	}
	close(release)
	if js := e.waitJob(t, ids[0]); js.Status != "done" {
		t.Fatalf("job = %+v", js)
	}
	if got := e.compute.processCalls.Load(); got != 1 {
		t.Fatalf("process calls = %d, want 1", got)
	}
}

func TestJobFailureAndRetry(t *testing.T) {
	e := newEnv(t, testConfig(), 5*time.Second)
	e.compute.onProcess(func(w http.ResponseWriter, _ *http.Request, _ computeclient.ProcessRequest) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":{"code":"no_epochs","message":"No epochs in this date range.","details":{}}}`))
	})
	body := `{"target":{"ra":10,"dec":20}}`
	id := decode[map[string]string](t, do(t, e.router, "POST", "/api/v1/views", body))["job_id"]
	js := e.waitJob(t, id)
	if js.Status != "failed" || js.Error == nil || js.Error.Code != "no_epochs" || js.Error.Message != "No epochs in this date range." {
		t.Fatalf("job = %+v", js)
	}

	// A failed job is retried by the next identical request (same job id).
	e.compute.onProcess(nil)
	rec := do(t, e.router, "POST", "/api/v1/views", body)
	if rec.Code != 202 || decode[map[string]string](t, rec)["job_id"] != id {
		t.Fatalf("retry = %d %s", rec.Code, rec.Body)
	}
	if js := e.waitJob(t, id); js.Status != "done" {
		t.Fatalf("retried job = %+v", js)
	}
}

func TestJobTimeout(t *testing.T) {
	e := newEnv(t, testConfig(), 200*time.Millisecond)
	e.compute.onProcess(func(_ http.ResponseWriter, r *http.Request, _ computeclient.ProcessRequest) {
		<-r.Context().Done()
	})
	id := decode[map[string]string](t, do(t, e.router, "POST", "/api/v1/views", `{"target":{"ra":10,"dec":20}}`))["job_id"]
	js := e.waitJob(t, id)
	if js.Status != "failed" || js.Error == nil || js.Error.Code != "job_timeout" {
		t.Fatalf("job = %+v", js)
	}
}

func TestComputeUnavailableJob(t *testing.T) {
	e := newEnv(t, testConfig(), 5*time.Second)
	e.compute.onProcess(func(w http.ResponseWriter, _ *http.Request, _ computeclient.ProcessRequest) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	id := decode[map[string]string](t, do(t, e.router, "POST", "/api/v1/views", `{"target":{"ra":10,"dec":20}}`))["job_id"]
	if js := e.waitJob(t, id); js.Error == nil || js.Error.Code != "compute_unavailable" {
		t.Fatalf("job = %+v", js)
	}
	if got := e.compute.processCalls.Load(); got != 2 {
		t.Fatalf("process calls = %d, want 2 (1 retry)", got)
	}
}

func TestPerIPJobCap(t *testing.T) {
	cfg := testConfig()
	cfg.MaxJobsPerIP = 1
	e := newEnv(t, cfg, 5*time.Second)
	release := make(chan struct{})
	e.compute.onProcess(func(w http.ResponseWriter, _ *http.Request, req computeclient.ProcessRequest) {
		<-release
		writeView(w, req)
	})
	defer close(release)

	first := do(t, e.router, "POST", "/api/v1/views", `{"target":{"ra":10,"dec":20}}`)
	if first.Code != 202 {
		t.Fatal(first.Body)
	}
	// Joining the running job is fine; a second new job is not.
	if rec := do(t, e.router, "POST", "/api/v1/views", `{"target":{"ra":10,"dec":20}}`); rec.Code != 202 {
		t.Fatalf("join = %d", rec.Code)
	}
	rec := do(t, e.router, "POST", "/api/v1/views", `{"target":{"ra":11,"dec":20}}`)
	if rec.Code != 429 || errorCode(t, rec) != "too_many_jobs" || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("second job = %d %s", rec.Code, rec.Body)
	}
}

func TestJobRateLimit(t *testing.T) {
	cfg := testConfig()
	cfg.JobsPerMinute, cfg.JobBurst = 1, 1
	e := newEnv(t, cfg, 5*time.Second)
	if rec := do(t, e.router, "POST", "/api/v1/views", `{"target":{"ra":10,"dec":20}}`); rec.Code != 202 {
		t.Fatal(rec.Body)
	}
	rec := do(t, e.router, "POST", "/api/v1/views", `{"target":{"ra":11,"dec":20}}`)
	if rec.Code != 429 || errorCode(t, rec) != "rate_limited" {
		t.Fatalf("second new job = %d %s", rec.Code, rec.Body)
	}
}

func TestProgressCallback(t *testing.T) {
	e := newEnv(t, testConfig(), 5*time.Second)
	reported := make(chan struct{})
	release := make(chan struct{})
	e.compute.onProcess(func(w http.ResponseWriter, _ *http.Request, req computeclient.ProcessRequest) {
		// Like compute: POST progress to the callback URL (here: straight into the router).
		path := strings.TrimPrefix(req.CallbackURL, "http://gateway.internal")
		for _, p := range []int{40, 20} { // progress never goes back
			if rec := do(t, e.router, "POST", path, fmt.Sprintf(`{"progress":%d}`, p), "X-Internal-Secret", "s3cret"); rec.Code != 204 {
				t.Errorf("progress = %d %s", rec.Code, rec.Body)
			}
		}
		close(reported)
		<-release
		writeView(w, req)
	})
	id := decode[map[string]string](t, do(t, e.router, "POST", "/api/v1/views", `{"target":{"ra":10,"dec":20}}`))["job_id"]
	<-reported
	js := decode[jobStatus](t, do(t, e.router, "GET", "/api/v1/jobs/"+id, ""))
	if js.Status != "running" || js.Progress != 40 {
		t.Fatalf("job = %+v, want running at 40", js)
	}
	rec := do(t, e.router, "POST", "/internal/jobs/"+id+"/progress", `{"progress":101}`, "X-Internal-Secret", "s3cret")
	if rec.Code != 400 {
		t.Fatalf("progress 101 = %d", rec.Code)
	}
	close(release)
	e.waitJob(t, id)
	rec = do(t, e.router, "POST", "/internal/jobs/"+id+"/progress", `{"progress":50}`, "X-Internal-Secret", "s3cret")
	if rec.Code != 404 {
		t.Fatalf("progress on finished job = %d", rec.Code)
	}
}

func TestResolveAndCoverage(t *testing.T) {
	e := newEnv(t, testConfig(), 5*time.Second)

	for range 2 {
		rec := do(t, e.router, "GET", "/api/v1/resolve?name=Barnard's%20Star", "")
		if rec.Code != 200 || decode[computeclient.ResolveResponse](t, rec).PM == nil {
			t.Fatalf("resolve = %d %s", rec.Code, rec.Body)
		}
	}
	if got := e.compute.resolveCalls.Load(); got != 1 {
		t.Errorf("resolve calls = %d, want 1 (cached)", got)
	}
	if rec := do(t, e.router, "GET", "/api/v1/resolve?name=zzz", ""); rec.Code != 404 || errorCode(t, rec) != "not_found" {
		t.Errorf("unknown name = %d %s", rec.Code, rec.Body)
	}

	rec := do(t, e.router, "GET", "/api/v1/coverage?ra=10&dec=20", "")
	if rec.Code != 200 || !decode[computeclient.CoverageResponse](t, rec).Covered {
		t.Fatalf("coverage = %d %s", rec.Code, rec.Body)
	}
	var n int
	if err := e.store.DB.QueryRow(t.Context(), `SELECT count(*) FROM releases`).Scan(&n); err != nil || n != 2 {
		t.Errorf("releases recorded = %d, %v", n, err)
	}

	// No coverage is a normal answer (B9), and POST /views says so.
	e.compute.uncovered.Store(true)
	rec = do(t, e.router, "GET", "/api/v1/coverage?ra=11&dec=20", "")
	if rec.Code != 200 || rec.Body.String() != `{"covered":false,"epochs":[],"releases":[]}` {
		t.Fatalf("no coverage = %d %s", rec.Code, rec.Body)
	}
	rec = do(t, e.router, "POST", "/api/v1/views", `{"target":{"ra":11,"dec":20}}`)
	if rec.Code != 422 || errorCode(t, rec) != "no_coverage" {
		t.Fatalf("view without coverage = %d %s", rec.Code, rec.Body)
	}

	e.srv.Close()
	rec = do(t, e.router, "GET", "/api/v1/coverage?ra=12&dec=20", "")
	if rec.Code != 503 || errorCode(t, rec) != "compute_unavailable" {
		t.Fatalf("compute down = %d %s", rec.Code, rec.Body)
	}

	if rec := do(t, e.router, "GET", "/api/v1/readyz", ""); rec.Code != 200 {
		t.Fatalf("readyz = %d %s", rec.Code, rec.Body)
	}
}
