package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/modex/agt-vault/common"

	"github.com/gin-gonic/gin"
)

// SecurityHeaders sets conservative security response headers on every request.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		c.Next()
	}
}

// fixedWindowLimiter is a tiny in-memory per-key fixed-window rate limiter. It is
// sufficient for protecting auth endpoints on a single instance; a multi-node
// deployment should front this with Redis.
type fixedWindowLimiter struct {
	mu     sync.Mutex
	hits   map[string]*window
	limit  int
	window time.Duration
}

type window struct {
	count int
	reset time.Time
}

func newLimiter(limit int, w time.Duration) *fixedWindowLimiter {
	return &fixedWindowLimiter{hits: map[string]*window{}, limit: limit, window: w}
}

func (l *fixedWindowLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	wnd, ok := l.hits[key]
	if !ok || now.After(wnd.reset) {
		l.hits[key] = &window{count: 1, reset: now.Add(l.window)}
		return true
	}
	if wnd.count >= l.limit {
		return false
	}
	wnd.count++
	return true
}

// RateLimit returns a middleware limiting requests per client IP. Use a strict
// limit for auth endpoints (e.g. 10 per minute) to slow credential stuffing.
func RateLimit(limit int, per time.Duration) gin.HandlerFunc {
	l := newLimiter(limit, per)
	return func(c *gin.Context) {
		if !l.allow(c.ClientIP(), time.Now()) {
			common.AbortError(c, http.StatusTooManyRequests, "too many requests, please slow down")
			return
		}
		c.Next()
	}
}
