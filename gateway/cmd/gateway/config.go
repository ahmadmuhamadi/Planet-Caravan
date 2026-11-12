package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// config comes from the environment only (12-factor, H10). See README.
type config struct {
	Addr     string
	LogLevel string

	DatabaseURL string
	RedisURL    string

	S3Endpoint  string
	S3Region    string
	S3Bucket    string
	S3AccessKey string
	S3SecretKey string

	ComputeURL      string
	ComputeSecret   string
	ComputeTimeout  time.Duration
	ComputeRetries  int
	CallbackBaseURL string

	JobTimeout  time.Duration
	ResolveTTL  time.Duration
	CoverageTTL time.Duration

	CORSOrigins    []string
	TrustedProxies []string
	MaxBodyBytes   int64

	RatePerSecond float64
	RateBurst     int
	JobsPerMinute float64
	JobBurst      int
	MaxJobsPerIP  int
}

func loadConfig() (config, error) {
	e := &env{}
	c := config{
		Addr:     e.str("ADDR", ":8080"),
		LogLevel: e.str("LOG_LEVEL", "info"),

		DatabaseURL: e.required("DATABASE_URL"),
		RedisURL:    e.required("REDIS_URL"),

		S3Endpoint:  e.required("S3_ENDPOINT"),
		S3Region:    e.str("S3_REGION", "us-east-1"),
		S3Bucket:    e.required("S3_BUCKET"),
		S3AccessKey: e.required("S3_ACCESS_KEY"),
		S3SecretKey: e.required("S3_SECRET_KEY"),

		ComputeURL:      strings.TrimRight(e.required("COMPUTE_URL"), "/"),
		ComputeSecret:   e.required("COMPUTE_SECRET"),
		ComputeTimeout:  e.duration("COMPUTE_TIMEOUT", 30*time.Second),
		ComputeRetries:  e.integer("COMPUTE_RETRIES", 2),
		CallbackBaseURL: e.str("CALLBACK_BASE_URL", ""),

		JobTimeout:  e.duration("JOB_TIMEOUT", 5*time.Minute),
		ResolveTTL:  e.duration("RESOLVE_TTL", 30*24*time.Hour),
		CoverageTTL: e.duration("COVERAGE_TTL", 6*time.Hour),

		CORSOrigins:    e.list("CORS_ORIGINS"),
		TrustedProxies: e.list("TRUSTED_PROXIES"),
		MaxBodyBytes:   int64(e.integer("MAX_BODY_BYTES", 64<<10)),

		RatePerSecond: e.float("RATE_LIMIT_RPS", 10),
		RateBurst:     e.integer("RATE_LIMIT_BURST", 100),
		JobsPerMinute: e.float("JOB_RATE_PER_MINUTE", 6),
		JobBurst:      e.integer("JOB_RATE_BURST", 3),
		MaxJobsPerIP:  e.integer("MAX_JOBS_PER_IP", 2),
	}
	if c.ComputeRetries < 0 || c.MaxBodyBytes <= 0 || c.RatePerSecond <= 0 || c.RateBurst <= 0 ||
		c.JobsPerMinute <= 0 || c.JobBurst <= 0 || c.MaxJobsPerIP <= 0 {
		e.errs = append(e.errs, errors.New("retries must be >= 0 and limits > 0"))
	}
	return c, errors.Join(e.errs...)
}

// env reads variables and collects every problem, so one run reports them all.
type env struct{ errs []error }

func (e *env) str(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func (e *env) required(key string) string {
	v := os.Getenv(key)
	if v == "" {
		e.errs = append(e.errs, fmt.Errorf("%s is required", key))
	}
	return v
}

func (e *env) list(key string) []string {
	var out []string
	for v := range strings.SplitSeq(os.Getenv(key), ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func parse[T any](e *env, key string, def T, parse func(string) (T, error)) T {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	out, err := parse(v)
	if err != nil {
		e.errs = append(e.errs, fmt.Errorf("%s: %w", key, err))
	}
	return out
}

func (e *env) duration(key string, def time.Duration) time.Duration {
	d := parse(e, key, def, time.ParseDuration)
	if d <= 0 {
		e.errs = append(e.errs, fmt.Errorf("%s must be positive", key))
	}
	return d
}

func (e *env) integer(key string, def int) int { return parse(e, key, def, strconv.Atoi) }

func (e *env) float(key string, def float64) float64 {
	return parse(e, key, def, func(s string) (float64, error) { return strconv.ParseFloat(s, 64) })
}
