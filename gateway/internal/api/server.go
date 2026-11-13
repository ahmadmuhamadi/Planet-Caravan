// Package api is the public HTTP API (CONTRACT.md, prefix /api/v1).
package api

import (
	_ "embed"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/cache"
	computeclient "github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/compute_client"
	"github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/jobs"
	"github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/store"
)

// Config holds the API settings.
type Config struct {
	ResolveTTL     time.Duration
	CoverageTTL    time.Duration
	CORSOrigins    []string
	TrustedProxies []string // empty: use the connection address as the client IP
	MaxBodyBytes   int64
	ComputeSecret  string // also guards the progress callback
	FramesBaseURL  string // public base URL of the frame images (object storage or CDN)

	RatePerSecond float64 // all API requests, per IP
	RateBurst     int
	JobsPerMinute float64 // new jobs, per IP
	JobBurst      int
	MaxJobsPerIP  int // jobs running at once, per IP
}

// Deps are the services the API uses.
type Deps struct {
	Store   *store.Store
	Cache   *cache.Redis
	Compute *computeclient.Client
	Jobs    *jobs.Runner
	Log     *slog.Logger
}

type server struct {
	Deps
	cfg      Config
	featured []featuredTarget
	jobLimit *ipLimiter
}

// featuredTarget is one of the fixed demo targets frozen in Phase 0 (G4).
type featuredTarget struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// featured.json stays empty until G4 names the featured targets.
//
//go:embed featured.json
var featuredJSON []byte

// New builds the router.
func New(cfg Config, d Deps) (*gin.Engine, error) {
	s := &server{Deps: d, cfg: cfg, jobLimit: newIPLimiter(cfg.JobsPerMinute/60, cfg.JobBurst)}
	if err := json.Unmarshal(featuredJSON, &s.featured); err != nil {
		return nil, err
	}

	r := gin.New()
	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		return nil, err
	}
	r.HandleMethodNotAllowed = true
	r.Use(requestID, accessLog(d.Log), recovery(d.Log), cors(cfg.CORSOrigins), bodyLimit(cfg.MaxBodyBytes))
	r.NoRoute(func(c *gin.Context) { fail(c, notFound("No such endpoint.")) })
	r.NoMethod(func(c *gin.Context) {
		fail(c, &apiError{status: http.StatusMethodNotAllowed, code: "method_not_allowed", message: "Method not allowed."})
	})

	v1 := r.Group("/api/v1")
	v1.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	v1.GET("/readyz", s.readyz)
	v1.GET("/metrics", gin.WrapH(promhttp.Handler()))

	pub := v1.Group("", rateLimit(newIPLimiter(cfg.RatePerSecond, cfg.RateBurst)))
	pub.GET("/resolve", s.resolveName)
	pub.GET("/autocomplete", s.autocomplete)
	pub.GET("/coverage", s.coverageAt)
	pub.POST("/views", s.createView)
	pub.GET("/views", s.findView)
	pub.GET("/views/:id", s.getView)
	pub.GET("/jobs/:id", s.getJob)

	// Compute reports job progress here (CONTRACT: callback URL).
	r.POST("/internal/jobs/:id/progress", internalAuth(cfg.ComputeSecret), s.jobProgress)
	return r, nil
}
