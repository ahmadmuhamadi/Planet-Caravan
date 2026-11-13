// Command gateway is the public HTTP API for Planet Caravan.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/api"
	"github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/cache"
	computeclient "github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/compute_client"
	"github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/jobs"
	"github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("gateway stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := loadConfig()
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel(cfg.LogLevel)})))
	if err != nil {
		return err
	}
	log := slog.Default()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return err
	}
	rdb := redis.NewClient(redisOpts)
	defer func() { _ = rdb.Close() }()

	compute := computeclient.New(cfg.ComputeURL, cfg.ComputeSecret, cfg.ComputeTimeout, cfg.ComputeRetries)
	runner := jobs.NewRunner(db, compute, cfg.JobTimeout, cfg.CallbackBaseURL, log)

	gateway, err := api.New(api.Config{
		ResolveTTL:       cfg.ResolveTTL,
		CoverageTTL:      cfg.CoverageTTL,
		StaleCoverageTTL: cfg.StaleCoverageTTL,
		CORSOrigins:      cfg.CORSOrigins,
		TrustedProxies:   cfg.TrustedProxies,
		MaxBodyBytes:     cfg.MaxBodyBytes,
		ComputeSecret:    cfg.ComputeSecret,
		FramesBaseURL:    cfg.FramesBaseURL,
		RatePerSecond:    cfg.RatePerSecond,
		RateBurst:        cfg.RateBurst,
		JobsPerMinute:    cfg.JobsPerMinute,
		JobBurst:         cfg.JobBurst,
		MaxJobsPerIP:     cfg.MaxJobsPerIP,
	}, api.Deps{
		Store: db, Cache: &cache.Redis{Client: rdb, Log: log}, Compute: compute, Jobs: runner, Log: log,
	})
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           gateway,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.ListenAndServe() }()
	log.Info("gateway listening", "addr", cfg.Addr)

	featuredDone := make(chan struct{})
	go func() {
		defer close(featuredDone)
		gateway.KeepFeaturedFresh(ctx, cfg.FeaturedRefresh)
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err = srv.Shutdown(shutdownCtx)
	err = errors.Join(err, runner.Shutdown(shutdownCtx)) // also cancels a featured job in progress
	select {
	case <-featuredDone:
	case <-shutdownCtx.Done():
		err = errors.Join(err, shutdownCtx.Err())
	}
	return err
}

func logLevel(s string) slog.Level {
	var l slog.Level
	if l.UnmarshalText([]byte(s)) != nil {
		return slog.LevelInfo
	}
	return l
}
