package realtime_test

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/golang-jwt/jwt/v5"
)

const testJWTSecret = "test-secret-that-is-at-least-32-characters!!"

func testSchemaCache(tables ...string) *schema.CacheHolder {
	sc := &schema.SchemaCache{
		Tables: make(map[string]*schema.Table),
	}
	for _, name := range tables {
		sc.Tables["public."+name] = &schema.Table{
			Schema: "public",
			Name:   name,
			Kind:   "table",
		}
	}
	ch := schema.NewCacheHolder(nil, testutil.DiscardLogger())
	ch.SetForTesting(sc)
	return ch
}

func testAuthService() *auth.Service {
	return auth.NewService(nil, testJWTSecret, time.Hour, 7*24*time.Hour, testutil.DiscardLogger())
}

func validToken() string {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":   "user-123",
		"email": "test@example.com",
		"iat":   jwt.NewNumericDate(now),
		"exp":   jwt.NewNumericDate(now.Add(time.Hour)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := token.SignedString([]byte(testJWTSecret))
	return signed
}

func expiredToken() string {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":   "user-123",
		"email": "test@example.com",
		"iat":   jwt.NewNumericDate(now.Add(-2 * time.Hour)),
		"exp":   jwt.NewNumericDate(now.Add(-time.Hour)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := token.SignedString([]byte(testJWTSecret))
	return signed
}

// TestSSEMissingTablesParam tests that the handler returns 400 when tables param is missing.
func TestSSEMissingTablesParam(t *testing.T) {
	hub := realtime.NewHub(testutil.DiscardLogger())
	h := realtime.NewHandler(hub, nil, nil, testSchemaCache("posts"), testutil.DiscardLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/realtime", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusBadRequest)
	testutil.Contains(t, w.Body.String(), "tables parameter is required")
}

// TestSSEUnknownTable tests that the handler returns 400 for unknown table names.
func TestSSEUnknownTable(t *testing.T) {
	hub := realtime.NewHub(testutil.DiscardLogger())
	h := realtime.NewHandler(hub, nil, nil, testSchemaCache("posts"), testutil.DiscardLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/realtime?tables=nonexistent", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusBadRequest)
	testutil.Contains(t, w.Body.String(), "unknown table")
}

// TestSSEAuthRequired tests that auth is enforced when authSvc is non-nil.
func TestSSEAuthRequired(t *testing.T) {
	hub := realtime.NewHub(testutil.DiscardLogger())
	h := realtime.NewHandler(hub, nil, testAuthService(), testSchemaCache("posts"), testutil.DiscardLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/realtime?tables=posts", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusUnauthorized)
	testutil.Contains(t, w.Body.String(), "authentication required")
}

// TestSSEExpiredToken tests that expired tokens are rejected.
func TestSSEExpiredToken(t *testing.T) {
	hub := realtime.NewHub(testutil.DiscardLogger())
	h := realtime.NewHandler(hub, nil, testAuthService(), testSchemaCache("posts"), testutil.DiscardLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/realtime?tables=posts&token="+expiredToken(), nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusUnauthorized)
	testutil.Contains(t, w.Body.String(), "invalid or expired token")
}

// TestSSENoAuthWhenDisabled tests that no auth is required when authSvc is nil.
func TestSSENoAuthWhenDisabled(t *testing.T) {
	hub := realtime.NewHub(testutil.DiscardLogger())
	h := realtime.NewHandler(hub, nil, nil, testSchemaCache("posts"), testutil.DiscardLogger())

	// Use a real HTTP server to get proper flushing/streaming.
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "?tables=posts")
	testutil.NoError(t, err)
	defer resp.Body.Close()

	testutil.Equal(t, resp.StatusCode, http.StatusOK)
	testutil.Equal(t, resp.Header.Get("Content-Type"), "text/event-stream")
	testutil.Equal(t, resp.Header.Get("Cache-Control"), "no-cache")

	// Read the connected event.
	scanner := bufio.NewScanner(resp.Body)
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		if line == "" && len(lines) > 1 {
			break // End of first event (empty line separator).
		}
	}
	testutil.True(t, len(lines) >= 2, "should have at least event + data lines")
	testutil.Equal(t, lines[0], "event: connected")
	testutil.True(t, strings.HasPrefix(lines[1], "data: "), "second line should be data")
	testutil.Contains(t, lines[1], "clientId")
}

// TestSSETokenInHeader tests auth via Authorization header.
func TestSSETokenInHeader(t *testing.T) {
	hub := realtime.NewHub(testutil.DiscardLogger())
	authSvc := testAuthService()
	h := realtime.NewHandler(hub, nil, authSvc, testSchemaCache("posts"), testutil.DiscardLogger())

	srv := httptest.NewServer(h)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"?tables=posts", nil)
	req.Header.Set("Authorization", "Bearer "+validToken())

	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	defer resp.Body.Close()

	testutil.Equal(t, resp.StatusCode, http.StatusOK)
	testutil.Equal(t, resp.Header.Get("Content-Type"), "text/event-stream")
}

// TestSSETokenInQueryParam tests auth via token query parameter.
func TestSSETokenInQueryParam(t *testing.T) {
	hub := realtime.NewHub(testutil.DiscardLogger())
	authSvc := testAuthService()
	h := realtime.NewHandler(hub, nil, authSvc, testSchemaCache("posts"), testutil.DiscardLogger())

	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "?tables=posts&token=" + validToken())
	testutil.NoError(t, err)
	defer resp.Body.Close()

	testutil.Equal(t, resp.StatusCode, http.StatusOK)
}

// TestSSEReceivesPublishedEvents tests that events published to the hub
// are delivered to connected SSE clients.
func TestSSEReceivesPublishedEvents(t *testing.T) {
	hub := realtime.NewHub(testutil.DiscardLogger())
	h := realtime.NewHandler(hub, nil, nil, testSchemaCache("posts"), testutil.DiscardLogger())

	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "?tables=posts")
	testutil.NoError(t, err)
	defer resp.Body.Close()

	testutil.Equal(t, resp.StatusCode, http.StatusOK)

	scanner := bufio.NewScanner(resp.Body)

	// Skip the connected event.
	for scanner.Scan() {
		if scanner.Text() == "" {
			break
		}
	}

	// Publish an event.
	hub.Publish(&realtime.Event{
		Action: "create",
		Table:  "posts",
		Record: map[string]any{"id": 1, "title": "Hello"},
	})

	// Read the published event.
	var eventLines []string
	for scanner.Scan() {
		line := scanner.Text()
		eventLines = append(eventLines, line)
		if line == "" && len(eventLines) > 1 {
			break
		}
	}

	testutil.True(t, len(eventLines) >= 2, "should have event lines")
	testutil.Equal(t, eventLines[0], "event: create")
	testutil.Contains(t, eventLines[1], `"table":"posts"`)
	testutil.Contains(t, eventLines[1], `"title":"Hello"`)
}

// TestSSEMultipleTables tests subscribing to multiple tables.
func TestSSEMultipleTables(t *testing.T) {
	hub := realtime.NewHub(testutil.DiscardLogger())
	h := realtime.NewHandler(hub, nil, nil, testSchemaCache("posts", "comments"), testutil.DiscardLogger())

	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "?tables=posts,comments")
	testutil.NoError(t, err)
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)

	// Skip connected event.
	for scanner.Scan() {
		if scanner.Text() == "" {
			break
		}
	}

	// Publish to posts.
	hub.Publish(&realtime.Event{Action: "create", Table: "posts", Record: map[string]any{"id": 1}})

	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		if line == "" && len(lines) > 1 {
			break
		}
	}
	testutil.Contains(t, lines[1], `"table":"posts"`)

	// Publish to comments.
	hub.Publish(&realtime.Event{Action: "create", Table: "comments", Record: map[string]any{"id": 2}})

	lines = nil
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		if line == "" && len(lines) > 1 {
			break
		}
	}
	testutil.Contains(t, lines[1], `"table":"comments"`)
}

// TestSSEClientCleanupOnDisconnect tests that disconnecting cleans up the client.
func TestSSEClientCleanupOnDisconnect(t *testing.T) {
	hub := realtime.NewHub(testutil.DiscardLogger())
	h := realtime.NewHandler(hub, nil, nil, testSchemaCache("posts"), testutil.DiscardLogger())

	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "?tables=posts")
	testutil.NoError(t, err)

	// Wait for the client to be registered.
	deadline := time.Now().Add(time.Second)
	for hub.ClientCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	testutil.Equal(t, hub.ClientCount(), 1)

	// Disconnect.
	resp.Body.Close()

	// Wait for cleanup.
	deadline = time.Now().Add(time.Second)
	for hub.ClientCount() > 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	testutil.Equal(t, hub.ClientCount(), 0)
}
