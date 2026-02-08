package auth

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
)

// RateLimiter is a simple in-memory per-IP sliding window rate limiter.
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	limit    int
	window   time.Duration
	stop     chan struct{}
}

type visitor struct {
	timestamps []time.Time
}

// NewRateLimiter creates a rate limiter that allows limit requests per window per IP.
// It starts a background goroutine to clean up stale entries.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		limit:    limit,
		window:   window,
		stop:     make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

// Stop terminates the background cleanup goroutine.
func (rl *RateLimiter) Stop() {
	close(rl.stop)
}

// Allow checks whether the given IP is within the rate limit.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	v, ok := rl.visitors[ip]
	if !ok {
		v = &visitor{}
		rl.visitors[ip] = v
	}

	pruneTimestamps(v, cutoff)

	if len(v.timestamps) >= rl.limit {
		return false
	}

	v.timestamps = append(v.timestamps, now)
	return true
}

// Middleware returns HTTP middleware that rate-limits by client IP.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !rl.Allow(ip) {
			w.Header().Set("Retry-After", strconv.Itoa(int(rl.window.Seconds())))
			httputil.WriteError(w, http.StatusTooManyRequests, "too many requests")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// pruneTimestamps removes timestamps older than cutoff from a visitor in place.
func pruneTimestamps(v *visitor, cutoff time.Time) {
	valid := v.timestamps[:0]
	for _, ts := range v.timestamps {
		if ts.After(cutoff) {
			valid = append(valid, ts)
		}
	}
	v.timestamps = valid
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			cutoff := time.Now().Add(-rl.window)
			for ip, v := range rl.visitors {
				pruneTimestamps(v, cutoff)
				if len(v.timestamps) == 0 {
					delete(rl.visitors, ip)
				}
			}
			rl.mu.Unlock()
		case <-rl.stop:
			return
		}
	}
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP (closest to client).
		if idx := net.ParseIP(xff); idx != nil {
			return idx.String()
		}
		// X-Forwarded-For can be comma-separated.
		parts := splitFirst(xff, ",")
		return parts
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func splitFirst(s, sep string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == sep[0] {
			return s[:i]
		}
	}
	return s
}
