package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"regexp"
	"runtime/debug"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	computeclient "github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/compute_client"
)

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// requestID takes X-Request-ID from the caller if sane, or makes one, and
// carries it in the request context so compute calls propagate it (R7).
func requestID(c *gin.Context) {
	id := c.GetHeader("X-Request-ID")
	if !validRequestID.MatchString(id) {
		id = newRequestID()
	}
	c.Header("X-Request-ID", id)
	c.Request = c.Request.WithContext(computeclient.WithRequestID(c.Request.Context(), id))
	c.Next()
}

func newRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func reqID(c *gin.Context) string { return computeclient.RequestID(c.Request.Context()) }

// accessLog writes one JSON line per request.
func accessLog(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		level := slog.LevelInfo
		if c.Writer.Status() >= 500 {
			level = slog.LevelError
		}
		log.Log(c.Request.Context(), level, "request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"ip", c.ClientIP(),
			"request_id", reqID(c),
		)
	}
}

// recovery turns a panic into a logged 500 in the error format.
func recovery(log *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, err any) {
		log.Error("panic", "err", fmt.Sprint(err), "request_id", reqID(c), "stack", string(debug.Stack()))
		fail(c, errInternal)
	})
}

// bodyLimit caps request bodies (R3).
func bodyLimit(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, n)
		c.Next()
	}
}

// cors allows only the client's deployed origins (H6). Preflights end here.
func cors(origins []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		allowed := origin != "" && slices.Contains(origins, origin)
		if allowed {
			h := c.Writer.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Add("Vary", "Origin")
			h.Set("Access-Control-Expose-Headers", "X-Request-ID, Retry-After, Location")
		}
		if c.Request.Method == http.MethodOptions && c.GetHeader("Access-Control-Request-Method") != "" {
			if allowed {
				h := c.Writer.Header()
				h.Set("Access-Control-Allow-Methods", "GET, POST")
				h.Set("Access-Control-Allow-Headers", "Content-Type, X-Request-ID")
				h.Set("Access-Control-Max-Age", "600")
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// internalAuth guards endpoints that only compute may call.
func internalAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		got := c.GetHeader("X-Internal-Secret")
		if subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
			fail(c, &apiError{status: http.StatusUnauthorized, code: "unauthorized", message: "Missing or wrong internal secret."})
			return
		}
		c.Next()
	}
}

// ipLimiter is a token bucket per client IP (R2).
type ipLimiter struct {
	limit rate.Limit
	burst int

	mu        sync.Mutex
	buckets   map[string]*bucket
	lastSweep time.Time
}

type bucket struct {
	*rate.Limiter
	seen time.Time
}

func newIPLimiter(perSecond float64, burst int) *ipLimiter {
	return &ipLimiter{limit: rate.Limit(perSecond), burst: burst, buckets: map[string]*bucket{}}
}

// allow takes a token for ip, or says how long until one is free.
func (l *ipLimiter) allow(ip string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.lastSweep) > time.Minute {
		// Idle buckets are full again, so dropping them changes nothing.
		for k, b := range l.buckets {
			if now.Sub(b.seen) > 10*time.Minute {
				delete(l.buckets, k)
			}
		}
		l.lastSweep = now
	}
	b, ok := l.buckets[ip]
	if !ok {
		b = &bucket{Limiter: rate.NewLimiter(l.limit, l.burst)}
		l.buckets[ip] = b
	}
	b.seen = now
	r := b.ReserveN(now, 1)
	if !r.OK() {
		return false, time.Minute
	}
	if d := r.DelayFrom(now); d > 0 {
		r.CancelAt(now)
		return false, d
	}
	return true, 0
}

func tooMany(c *gin.Context, wait time.Duration, code, message string) {
	c.Header("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
	fail(c, &apiError{status: http.StatusTooManyRequests, code: code, message: message})
}

func rateLimit(l *ipLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if ok, wait := l.allow(c.ClientIP(), time.Now()); !ok {
			tooMany(c, wait, "rate_limited", "Too many requests. Please slow down.")
			return
		}
		c.Next()
	}
}
