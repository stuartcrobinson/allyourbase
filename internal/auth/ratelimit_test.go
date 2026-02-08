package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestRateLimiterAllow(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)
	defer rl.Stop()

	testutil.True(t, rl.Allow("1.2.3.4"), "first request should be allowed")
	testutil.True(t, rl.Allow("1.2.3.4"), "second request should be allowed")
	testutil.True(t, rl.Allow("1.2.3.4"), "third request should be allowed")
	testutil.False(t, rl.Allow("1.2.3.4"), "fourth request should be rejected")

	// Different IP should still be allowed.
	testutil.True(t, rl.Allow("5.6.7.8"), "different IP should be allowed")
}

func TestRateLimiterWindowExpiry(t *testing.T) {
	rl := NewRateLimiter(2, 20*time.Millisecond)
	defer rl.Stop()

	testutil.True(t, rl.Allow("1.2.3.4"), "first request")
	testutil.True(t, rl.Allow("1.2.3.4"), "second request")
	testutil.False(t, rl.Allow("1.2.3.4"), "third request rejected")

	// Sleep well past the window to avoid CI flakes.
	time.Sleep(50 * time.Millisecond)

	testutil.True(t, rl.Allow("1.2.3.4"), "should be allowed after window expires")
}

func TestRateLimiterMiddleware(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)
	defer rl.Stop()

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First two requests succeed.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.RemoteAddr = "1.2.3.4:12345"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		testutil.Equal(t, w.Code, http.StatusOK)
	}

	// Third request is rate limited.
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.RemoteAddr = "1.2.3.4:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	testutil.Equal(t, w.Code, http.StatusTooManyRequests)
	testutil.True(t, w.Header().Get("Retry-After") != "", "should have Retry-After header")
}
