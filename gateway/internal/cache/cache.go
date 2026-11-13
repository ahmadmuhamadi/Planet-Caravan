// Package cache builds cache keys and stores JSON in Redis.
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redis/go-redis/v9"

	computeclient "github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/compute_client"
)

var lookups = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "gateway_cache_lookups_total",
	Help: "Cache lookups by kind (resolve, coverage, view) and result (hit, miss).",
}, []string{"kind", "result"})

// Hit records a cache lookup result for the hit-rate metric.
func Hit(kind string, hit bool) {
	result := "miss"
	if hit {
		result = "hit"
	}
	lookups.WithLabelValues(kind, result).Inc()
}

// unitsPerDegree is the coordinate rounding step: 0.1 arcsec (B5).
const unitsPerDegree = 36000

// RoundCoords rounds a position to 0.1 arcsec and wraps RA into [0, 360).
func RoundCoords(ra, dec float64) (float64, float64) {
	r, d := units(ra, dec)
	return float64(r) / unitsPerDegree, float64(d) / unitsPerDegree
}

func units(ra, dec float64) (int64, int64) {
	r := int64(math.Round(ra*unitsPerDegree)) % (360 * unitsPerDegree)
	if r < 0 {
		r += 360 * unitsPerDegree
	}
	return r, int64(math.Round(dec * unitsPerDegree))
}

// ViewKey is the cache key of a view (B5). p must already be normalised
// (coordinates rounded, defaults filled, detectors and collections sorted);
// the target name does not change the view, so it is left out.
func ViewKey(p computeclient.ViewParams) string {
	p.Target.Name = ""
	b, err := json.Marshal(p)
	if err != nil {
		panic(err) // plain struct of numbers, strings and slices
	}
	sum := sha256.Sum256(b)
	return "v1:" + hex.EncodeToString(sum[:])
}

// ResolveKey is the Redis key for a resolved name.
func ResolveKey(name string) string {
	return "resolve:" + strings.ToLower(strings.Join(strings.Fields(name), " "))
}

// CoverageKey is the Redis key for coverage at a position, rounded like view keys.
func CoverageKey(ra, dec float64) string {
	r, d := units(ra, dec)
	return fmt.Sprintf("coverage:%d:%d", r, d)
}

// StaleKey is where the last good copy of a value lives, kept longer than the
// value itself and used only when its source is unavailable (R5).
func StaleKey(key string) string { return "stale:" + key }

// Redis stores JSON values with a TTL. Redis errors are logged and treated as
// misses, so a Redis outage slows requests down instead of failing them.
type Redis struct {
	Client *redis.Client
	Log    *slog.Logger
}

// Get loads key into out and reports whether it was found.
func (r *Redis) Get(ctx context.Context, key string, out any) bool {
	b, err := r.Client.Get(ctx, key).Bytes()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			r.Log.WarnContext(ctx, "redis get failed", "key", key, "err", err)
		}
		return false
	}
	if err := json.Unmarshal(b, out); err != nil {
		r.Log.WarnContext(ctx, "redis value unreadable", "key", key, "err", err)
		return false
	}
	return true
}

// Set stores v under key for ttl.
func (r *Redis) Set(ctx context.Context, key string, v any, ttl time.Duration) {
	b, err := json.Marshal(v)
	if err == nil {
		err = r.Client.Set(ctx, key, b, ttl).Err()
	}
	if err != nil {
		r.Log.WarnContext(ctx, "redis set failed", "key", key, "err", err)
	}
}
