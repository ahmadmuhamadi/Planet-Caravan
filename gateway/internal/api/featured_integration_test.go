package api

import (
	"fmt"
	"slices"
	"testing"
	"time"

	computeclient "github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/compute_client"
)

var (
	qr2 = computeclient.Release{Name: "QR2", Collection: "spherex_qr2"}
	qr3 = computeclient.Release{Name: "QR3", Collection: "spherex_qr3"}
	dr1 = computeclient.Release{Name: "DR1", Collection: "spherex_dr1"}
)

// TestFeaturedTargets covers Phase 2: precomputed featured views (B11), their
// release set (B10), and serving them while compute is unreachable (R5).
func TestFeaturedTargets(t *testing.T) {
	cfg := testConfig()
	cfg.Featured = testFeatured[:1]
	// Fresh coverage expires at once, so every lookup asks compute and the
	// stale copy is all that is left once compute is down.
	cfg.CoverageTTL = time.Millisecond
	e := newEnv(t, cfg, 5*time.Second)
	b := testFeatured[0]
	// The request the viewer makes for a featured target.
	opened := fmt.Sprintf(`{"target":{"ra":%v,"dec":%v},"fov_arcmin":3,"mode":"blink"}`, b.RA, b.Dec)

	// B11: the view is built ahead of time, once.
	e.router.RefreshFeatured(t.Context())
	e.router.RefreshFeatured(t.Context())
	if n := e.compute.processCalls.Load(); n != 1 {
		t.Fatalf("process calls = %d, want 1", n)
	}
	rec := do(t, e.router, "POST", "/api/v1/views", opened)
	if rec.Code != 200 {
		t.Fatalf("featured view = %d %s, want a cached 200", rec.Code, rec.Body)
	}
	first := decode[view](t, rec)
	if !slices.Equal(first.ReleaseSet, []computeclient.Release{qr2, qr3}) || first.NewerDataAvailable {
		t.Fatalf("view releases = %+v, newer = %v", first.ReleaseSet, first.NewerDataAvailable)
	}

	// A new release: the next refresh builds a new view, and links to the old
	// one still open it, saying newer data exists (B10, F8).
	e.compute.newRelease.Store(true)
	e.router.RefreshFeatured(t.Context())
	if n := e.compute.processCalls.Load(); n != 2 {
		t.Fatalf("process calls after new release = %d, want 2", n)
	}
	old := decode[view](t, do(t, e.router, "GET", "/api/v1/views/"+first.ID, ""))
	if old.ID != first.ID || !old.NewerDataAvailable || !slices.Equal(old.ReleaseSet, first.ReleaseSet) {
		t.Fatalf("old view = id %s, releases %+v, newer %v", old.ID, old.ReleaseSet, old.NewerDataAvailable)
	}
	rec = do(t, e.router, "POST", "/api/v1/views", opened)
	latest := decode[view](t, rec)
	if rec.Code != 200 || latest.ID == first.ID || latest.NewerDataAvailable ||
		!slices.Equal(latest.ReleaseSet, []computeclient.Release{dr1, qr2, qr3}) {
		t.Fatalf("latest view = %d %+v", rec.Code, latest)
	}

	// R5: with compute unreachable, the featured target opens from cache and storage.
	e.srv.Close()
	for _, req := range []struct{ method, path, body string }{
		{"GET", "/api/v1/objects/" + b.Slug, ""},
		{"GET", fmt.Sprintf("/api/v1/coverage?ra=%v&dec=%v", b.RA, b.Dec), ""},
		{"POST", "/api/v1/views", opened},
		{"GET", "/api/v1/views/" + latest.ID, ""},
	} {
		start := time.Now()
		rec := do(t, e.router, req.method, req.path, req.body)
		if rec.Code != 200 {
			t.Fatalf("%s %s with compute down = %d %s", req.method, req.path, rec.Code, rec.Body)
		}
		if took := time.Since(start); took > newerDataTimeout+time.Second {
			t.Errorf("%s %s took %s with compute down", req.method, req.path, took)
		}
	}
	if got := decode[view](t, do(t, e.router, "POST", "/api/v1/views", opened)); got.ID != latest.ID {
		t.Fatalf("compute down served view %s, want %s", got.ID, latest.ID)
	}

	// A position never seen before still fails plainly: there is no stale copy.
	rec = do(t, e.router, "GET", "/api/v1/coverage?ra=1&dec=1", "")
	if rec.Code != 503 || errorCode(t, rec) != "compute_unavailable" {
		t.Fatalf("unseen position with compute down = %d %s", rec.Code, rec.Body)
	}
}
