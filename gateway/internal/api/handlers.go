package api

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/cache"
	computeclient "github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/compute_client"
	"github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/store"
)

// GET /resolve?name=
func (s *server) resolveName(c *gin.Context) {
	name, e := checkName("name", c.Query("name"))
	if e != nil {
		fail(c, e)
		return
	}
	res, e := s.resolve(c.Request.Context(), name)
	if e != nil {
		fail(c, e)
		return
	}
	c.JSON(http.StatusOK, res)
}

// resolve returns a resolved name, cached for ResolveTTL.
func (s *server) resolve(ctx context.Context, name string) (*computeclient.ResolveResponse, *apiError) {
	key := cache.ResolveKey(name)
	var res computeclient.ResolveResponse
	hit := s.Cache.Get(ctx, key, &res)
	cache.Hit("resolve", hit)
	if hit {
		return &res, nil
	}
	got, err := s.Compute.Resolve(ctx, name)
	if err != nil {
		return nil, s.computeFailed(ctx, "resolve", err)
	}
	if checkCoords(got.RA, got.Dec) != nil {
		s.Log.ErrorContext(ctx, "compute resolved to invalid coordinates", "name", name, "ra", got.RA, "dec", got.Dec)
		return nil, fromCompute(&computeclient.Error{Status: http.StatusOK, Code: "compute_error"})
	}
	s.Cache.Set(ctx, key, got, s.cfg.ResolveTTL)
	return got, nil
}

// GET /autocomplete?q= → featured targets whose name or slug contains q, prefix matches first.
func (s *server) autocomplete(c *gin.Context) {
	q := strings.ToLower(strings.Join(strings.Fields(c.Query("q")), " "))
	if len(q) > maxNameLen {
		fail(c, badRequest("q", "q must be at most 100 characters."))
		return
	}
	type match struct {
		t      featuredTarget
		prefix bool
	}
	var matches []match
	for _, t := range s.featured {
		name, slug := strings.ToLower(t.Name), t.Slug
		if q == "" || strings.Contains(name, q) || strings.Contains(slug, q) {
			matches = append(matches, match{t, strings.HasPrefix(name, q) || strings.HasPrefix(slug, q)})
		}
	}
	slices.SortStableFunc(matches, func(a, b match) int {
		if a.prefix != b.prefix {
			if a.prefix {
				return -1
			}
			return 1
		}
		return cmp.Compare(a.t.Name, b.t.Name)
	})
	out := make([]featuredTarget, 0, min(len(matches), 10))
	for _, m := range matches[:min(len(matches), 10)] {
		out = append(out, m.t)
	}
	c.JSON(http.StatusOK, gin.H{"suggestions": out})
}

// GET /coverage?ra=&dec= → 200 with covered:false when empty (B9).
func (s *server) coverageAt(c *gin.Context) {
	ra, dec, e := queryCoords(c.Query("ra"), c.Query("dec"))
	if e != nil {
		fail(c, e)
		return
	}
	cov, e := s.coverage(c.Request.Context(), ra, dec)
	if e != nil {
		fail(c, e)
		return
	}
	c.JSON(http.StatusOK, cov)
}

// coverage returns coverage at a position, cached for CoverageTTL, and records new releases.
func (s *server) coverage(ctx context.Context, ra, dec float64) (*computeclient.CoverageResponse, *apiError) {
	key := cache.CoverageKey(ra, dec)
	var cov computeclient.CoverageResponse
	hit := s.Cache.Get(ctx, key, &cov)
	cache.Hit("coverage", hit)
	if hit {
		return &cov, nil
	}
	ra, dec = cache.RoundCoords(ra, dec)
	got, err := s.Compute.Coverage(ctx, ra, dec)
	if err != nil {
		return nil, s.computeFailed(ctx, "coverage", err)
	}
	if got.Epochs == nil {
		got.Epochs = []computeclient.Epoch{}
	}
	if got.Releases == nil {
		got.Releases = []computeclient.Release{}
	}
	s.Cache.Set(ctx, key, got, s.cfg.CoverageTTL)
	if err := s.Store.AddReleases(ctx, got.Releases); err != nil {
		s.Log.ErrorContext(ctx, "recording releases", "err", err, "request_id", computeclient.RequestID(ctx))
	}
	return got, nil
}

func (s *server) computeFailed(ctx context.Context, call string, err error) *apiError {
	e := fromCompute(err)
	if e.status >= 500 {
		s.Log.ErrorContext(ctx, "compute call failed", "call", call, "err", err, "request_id", computeclient.RequestID(ctx))
	}
	return e
}

// viewParams completes normalised params with the target position and the
// release set covering it, so the cache key is final (B5).
func (s *server) viewParams(ctx context.Context, req *viewRequest) (computeclient.ViewParams, *apiError) {
	p, e := req.normalize()
	if e != nil {
		return p, e
	}
	name, ra, dec, e := req.targetOf()
	if e != nil {
		return p, e
	}
	if name != "" {
		res, e := s.resolve(ctx, name)
		if e != nil {
			return p, e
		}
		name, ra, dec = res.Name, res.RA, res.Dec
	}
	cov, e := s.coverage(ctx, ra, dec)
	if e != nil {
		return p, e
	}
	if !cov.Covered {
		return p, &apiError{status: http.StatusUnprocessableEntity, code: "no_coverage", message: "No SPHEREx data here yet."}
	}
	return withTarget(p, name, ra, dec, cov.Releases), nil
}

// POST /views → 200 with the cached view, or 202 with a job (B2). Identical
// requests share one job (B4).
func (s *server) createView(c *gin.Context) {
	ctx := c.Request.Context()
	var req viewRequest
	if e := decodeJSON(c.Request.Body, &req); e != nil {
		fail(c, e)
		return
	}
	p, e := s.viewParams(ctx, &req)
	if e != nil {
		fail(c, e)
		return
	}
	key := cache.ViewKey(p)

	if s.serveCachedView(c, key) {
		return
	}
	job, err := s.Store.ActiveJob(ctx, key, s.Jobs.Stale())
	if err == nil {
		accepted(c, job.ID)
		return
	}
	if !errors.Is(err, store.ErrNotFound) {
		s.internalError(c, "find active job", err)
		return
	}

	// A new job: the strict per-IP limits apply (R2).
	ip := c.ClientIP()
	if ok, wait := s.jobLimit.allow(ip, time.Now()); !ok {
		tooMany(c, wait, "rate_limited", "Too many new views. Please wait before starting another.")
		return
	}
	if !s.Jobs.Reserve(ip, s.cfg.MaxJobsPerIP) {
		tooMany(c, 30*time.Second, "too_many_jobs", "You already have views being built. Please wait for one to finish.")
		return
	}
	job, err = s.Store.ClaimJob(ctx, key, p, s.Jobs.Stale())
	if err != nil || job == nil {
		s.Jobs.Release(ip)
	}
	switch {
	case err != nil:
		s.internalError(c, "claim job", err)
	case job != nil:
		s.Jobs.Start(job, ip, reqID(c))
		accepted(c, job.ID)
	default:
		// Another request claimed the key between our lookups; join its job.
		if job, err = s.Store.ActiveJob(ctx, key, s.Jobs.Stale()); err == nil {
			accepted(c, job.ID)
		} else if !s.serveCachedView(c, key) {
			fail(c, &apiError{status: http.StatusConflict, code: "conflict", message: "This view changed while starting. Please retry."})
		}
	}
}

func accepted(c *gin.Context, jobID string) {
	url := "/api/v1/jobs/" + jobID
	c.Header("Location", url)
	c.JSON(http.StatusAccepted, gin.H{"job_id": jobID, "status_url": url})
}

// serveCachedView writes the view stored under key, if any.
func (s *server) serveCachedView(c *gin.Context, key string) bool {
	v, err := s.Store.ViewByKey(c.Request.Context(), key)
	cache.Hit("view", err == nil)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, viewJSON(v))
		return true
	case !errors.Is(err, store.ErrNotFound):
		s.internalError(c, "find view", err)
		return true
	}
	return false
}

// GET /views?ra=&dec=&fov=&mode= → the cached view with the POST /views cache key, or 404.
func (s *server) findView(c *gin.Context) {
	ra, dec, e := queryCoords(c.Query("ra"), c.Query("dec"))
	if e != nil {
		fail(c, e)
		return
	}
	req := viewRequest{Mode: c.Query("mode"), Target: &viewTarget{RA: &ra, Dec: &dec}}
	if f := c.Query("fov"); f != "" {
		fov, err := strconv.ParseFloat(f, 64)
		if err != nil {
			fail(c, badRequest("fov", "fov must be a number of arcminutes."))
			return
		}
		req.FOVArcmin = &fov
	}
	p, e := s.viewParams(c.Request.Context(), &req)
	if e != nil {
		fail(c, e)
		return
	}
	if !s.serveCachedView(c, cache.ViewKey(p)) && !c.IsAborted() {
		fail(c, notFound("This view has not been built yet. Create it with POST /views."))
	}
}

// GET /views/{id}
func (s *server) getView(c *gin.Context) {
	v, ok := s.viewByID(c)
	if ok {
		c.JSON(http.StatusOK, viewJSON(v))
	}
}

func (s *server) viewByID(c *gin.Context) (*store.View, bool) {
	v, err := s.Store.ViewByID(c.Request.Context(), c.Param("id"))
	switch {
	case errors.Is(err, store.ErrNotFound):
		fail(c, notFound("No view with this id."))
		return nil, false
	case err != nil:
		s.internalError(c, "get view", err)
		return nil, false
	}
	return v, true
}

// view is the public view shape. Phase 2 adds release_set and newer_data_available (B10).
type view struct {
	ID        string                    `json:"id"`
	Target    computeclient.Target      `json:"target"`
	FOVArcmin float64                   `json:"fov_arcmin"`
	Mode      string                    `json:"mode"`
	Epochs    []computeclient.Epoch     `json:"epochs"`
	Frames    []computeclient.FrameInfo `json:"frames"`
	Stretch   computeclient.Stretch     `json:"stretch"`
	Warnings  []string                  `json:"warnings"`
	CreatedAt time.Time                 `json:"created_at"`
}

func viewJSON(v *store.View) view {
	frames := make([]computeclient.FrameInfo, len(v.Frames))
	for i, f := range v.Frames {
		frames[i] = f.FrameInfo
	}
	return view{
		ID: v.ID, Target: v.Target, FOVArcmin: v.Params.FOVArcmin, Mode: v.Params.Mode,
		Epochs: v.Epochs, Frames: frames, Stretch: v.Stretch, Warnings: v.Warnings, CreatedAt: v.CreatedAt,
	}
}

// GET /views/{id}/frames/{frame_id}.png, streamed from object storage.
func (s *server) getFrame(c *gin.Context) {
	frameID, ok := strings.CutSuffix(c.Param("file"), ".png")
	if !ok {
		fail(c, notFound("Frames are served as {frame_id}.png."))
		return
	}
	v, ok := s.viewByID(c)
	if !ok {
		return
	}
	i := slices.IndexFunc(v.Frames, func(f computeclient.Frame) bool { return f.ID == frameID })
	if i < 0 {
		fail(c, notFound("No frame with this id in this view."))
		return
	}
	obj, size, err := s.Blobs.Open(c.Request.Context(), v.Frames[i].ObjectKey)
	if errors.Is(err, store.ErrNotFound) {
		s.Log.WarnContext(c, "frame image missing", "view_id", v.ID, "key", v.Frames[i].ObjectKey, "request_id", reqID(c))
		fail(c, notFound("This frame's image is no longer stored."))
		return
	}
	if err != nil {
		s.internalError(c, "open frame", err)
		return
	}
	defer func() { _ = obj.Close() }()
	// A view id and frame id never change what they point to.
	c.DataFromReader(http.StatusOK, size, "image/png", obj, map[string]string{
		"Cache-Control": "public, max-age=31536000, immutable",
	})
}

// GET /jobs/{id} → status, progress, and the view or the error.
func (s *server) getJob(c *gin.Context) {
	ctx := c.Request.Context()
	job, err := s.Store.JobByID(ctx, c.Param("id"))
	if errors.Is(err, store.ErrNotFound) {
		fail(c, notFound("No job with this id."))
		return
	}
	if err != nil {
		s.internalError(c, "get job", err)
		return
	}
	out := gin.H{"id": job.ID, "status": job.Status, "progress": job.Progress}
	switch job.Status {
	case store.StatusQueued, store.StatusRunning:
		if time.Since(job.UpdatedAt) > s.Jobs.Stale() {
			// Its gateway died; the next POST /views for this view restarts it.
			out["status"] = store.StatusFailed
			out["error"] = store.JobError{Code: "interrupted", Message: "The job stopped unexpectedly. Please try again."}
		}
	case store.StatusDone:
		v, err := s.Store.ViewByID(ctx, job.ViewID)
		if err != nil {
			s.internalError(c, "get job view", err)
			return
		}
		out["result"] = viewJSON(v)
	case store.StatusFailed:
		out["error"] = job.Error
	}
	c.JSON(http.StatusOK, out)
}

// POST /internal/jobs/{id}/progress {"progress": 0-100}, from compute.
func (s *server) jobProgress(c *gin.Context) {
	var body struct {
		Progress *int `json:"progress"`
	}
	if e := decodeJSON(c.Request.Body, &body); e != nil {
		fail(c, e)
		return
	}
	if body.Progress == nil || *body.Progress < 0 || *body.Progress > 100 {
		fail(c, badRequest("progress", "progress must be a number from 0 to 100."))
		return
	}
	ok, err := s.Store.SetProgress(c.Request.Context(), c.Param("id"), *body.Progress)
	switch {
	case err != nil:
		s.internalError(c, "set progress", err)
	case !ok:
		fail(c, notFound("No running job with this id."))
	default:
		c.Status(http.StatusNoContent)
	}
}

// GET /readyz checks Postgres, Redis and object storage.
func (s *server) readyz(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	failed := map[string]any{}
	for name, ping := range map[string]func(context.Context) error{
		"postgres":       s.Store.Ping,
		"redis":          func(ctx context.Context) error { return s.Cache.Client.Ping(ctx).Err() },
		"object_storage": s.Blobs.Ping,
	} {
		if err := ping(ctx); err != nil {
			s.Log.ErrorContext(ctx, "not ready", "dependency", name, "err", err)
			failed[name] = "unavailable"
		}
	}
	if len(failed) > 0 {
		fail(c, &apiError{status: http.StatusServiceUnavailable, code: "not_ready", message: "A dependency is unavailable.", details: failed})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *server) internalError(c *gin.Context, what string, err error) {
	s.Log.ErrorContext(c, what, "err", err, "request_id", reqID(c))
	fail(c, errInternal)
}
