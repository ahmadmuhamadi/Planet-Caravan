package api

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/cache"
	computeclient "github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/compute_client"
	"github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/store"
)

// FeaturedTarget is one of the fixed demo targets frozen in Phase 0 (G4).
// Readable names map to these (B8); their views are built ahead of time (B11).
type FeaturedTarget struct {
	Slug string  `json:"slug"`
	Name string  `json:"name"`
	Type string  `json:"type"`
	RA   float64 `json:"ra"`  // ICRS degrees
	Dec  float64 `json:"dec"` // ICRS degrees
}

// featured.json stays empty until G4 names the featured targets.
//
//go:embed featured.json
var featuredJSON []byte

var validSlug = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// DefaultFeatured returns the embedded featured list.
func DefaultFeatured() ([]FeaturedTarget, error) {
	var ts []FeaturedTarget
	if err := json.Unmarshal(featuredJSON, &ts); err != nil {
		return nil, fmt.Errorf("featured.json: %w", err)
	}
	return ts, checkFeatured(ts)
}

func checkFeatured(ts []FeaturedTarget) error {
	seen := map[string]bool{}
	for _, t := range ts {
		switch {
		case !validSlug.MatchString(t.Slug) || seen[t.Slug]:
			return fmt.Errorf("featured target %q: slug must be unique, lowercase words joined by -", t.Slug)
		case t.Name == "":
			return fmt.Errorf("featured target %q: name is required", t.Slug)
		case checkCoords(t.RA, t.Dec) != nil:
			return fmt.Errorf("featured target %q: %s", t.Slug, checkCoords(t.RA, t.Dec).message)
		}
		seen[t.Slug] = true
	}
	return nil
}

// GET /objects/{slug} → the featured target (B8).
func (s *server) getObject(c *gin.Context) {
	for _, t := range s.featured {
		if t.Slug == c.Param("slug") {
			c.JSON(http.StatusOK, t)
			return
		}
	}
	fail(c, notFound("No featured target with this name."))
}

// KeepFeaturedFresh builds the featured views now and then every `every`,
// until ctx ends (B11). Run it in its own goroutine.
func (a *API) KeepFeaturedFresh(ctx context.Context, every time.Duration) {
	if len(a.s.featured) == 0 {
		return
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		a.RefreshFeatured(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// RefreshFeatured makes sure each featured target has a view for its current
// release set, building missing ones one at a time. A new release changes the
// coverage and so the cache key, which makes its view missing and rebuilt (B11).
func (a *API) RefreshFeatured(ctx context.Context) {
	for _, t := range a.s.featured {
		if ctx.Err() != nil {
			return
		}
		ctx := computeclient.WithRequestID(ctx, newRequestID())
		if err := a.s.precompute(ctx, t); err != nil {
			a.s.Log.ErrorContext(ctx, "precomputing featured target", "slug", t.Slug, "err", err,
				"request_id", computeclient.RequestID(ctx))
		}
	}
}

// precompute builds the default view of t (the one the viewer opens) if it is missing.
func (s *server) precompute(ctx context.Context, t FeaturedTarget) error {
	req := viewRequest{Target: &viewTarget{RA: &t.RA, Dec: &t.Dec}}
	p, e := s.viewParams(ctx, &req)
	if e != nil {
		return e
	}
	key := cache.ViewKey(p)
	switch _, err := s.Store.ViewByKey(ctx, key); {
	case err == nil:
		return nil
	case !errors.Is(err, store.ErrNotFound):
		return err
	}
	job, err := s.Store.ClaimJob(ctx, key, p, s.Jobs.Stale())
	if err != nil || job == nil { // nil: a running job already builds it
		return err
	}
	s.Log.InfoContext(ctx, "building featured view", "slug", t.Slug, "job_id", job.ID)
	s.Jobs.Run(job, computeclient.RequestID(ctx))
	return nil
}
