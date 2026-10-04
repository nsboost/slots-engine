package api

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// RateLimiter is a simple per-key sliding-window-ish limiter: each key
// (here, the authenticated player ID, falling back to remote IP for
// unauthenticated routes) gets up to Limit requests per Window. State is
// in-process memory.
//
// This is deliberately simple and has a known limitation: it is
// per-instance. Running more than one API server process behind a load
// balancer means each instance enforces its own limit independently, so
// the effective limit is roughly Limit * (number of instances). That's
// fine for a single-instance demo; replace with a shared store (Redis
// INCR + EXPIRE is the standard pattern) before running multiple
// instances in production.
type RateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	buckets map[string]*bucket
}

type bucket struct {
	count      int
	windowFrom time.Time
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{limit: limit, window: window, buckets: make(map[string]*bucket)}
	go rl.sweepLoop()
	return rl
}

// Allow reports whether the request for key should proceed, and advances
// that key's window bookkeeping as a side effect.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, ok := rl.buckets[key]
	if !ok || now.Sub(b.windowFrom) >= rl.window {
		rl.buckets[key] = &bucket{count: 1, windowFrom: now}
		return true
	}
	if b.count >= rl.limit {
		return false
	}
	b.count++
	return true
}

// sweepLoop periodically drops stale buckets so memory doesn't grow
// unbounded with one-off callers.
func (rl *RateLimiter) sweepLoop() {
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for k, b := range rl.buckets {
			if now.Sub(b.windowFrom) >= 2*rl.window {
				delete(rl.buckets, k)
			}
		}
		rl.mu.Unlock()
	}
}

// Middleware rate-limits by authenticated player ID when RequireAuth has
// already run (so retries against someone else's key don't help an
// attacker dodge the limit), falling back to remote IP for routes without
// auth context.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, ok := PlayerIDFromContext(r.Context())
		if !ok {
			key = clientIP(r)
		}
		if !rl.Allow(key) {
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
