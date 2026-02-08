//go:build integration

package server_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

var sharedPG *testutil.PGContainer

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	sharedPG = pg
	code := m.Run()
	cleanup()
	os.Exit(code)
}

func createIntegrationTestSchema(t *testing.T, ctx context.Context) {
	t.Helper()

	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	if err != nil {
		t.Fatalf("resetting schema: %v", err)
	}

	_, err = sharedPG.Pool.Exec(ctx, `
		CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			email VARCHAR(255) UNIQUE
		)
	`)
	if err != nil {
		t.Fatalf("creating test schema: %v", err)
	}
}

func TestSchemaEndpointReturnsValidJSON(t *testing.T) {
	ctx := context.Background()
	createIntegrationTestSchema(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	err := ch.Load(ctx)
	testutil.NoError(t, err)

	cfg := config.Default()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/schema", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusOK)
	testutil.Equal(t, w.Header().Get("Content-Type"), "application/json")

	// Should be valid JSON with tables.
	var result schema.SchemaCache
	err = json.Unmarshal(w.Body.Bytes(), &result)
	testutil.NoError(t, err)
	testutil.True(t, len(result.Tables) >= 1, "expected at least 1 table")
	testutil.NotNil(t, result.Tables["public.users"])
}

// TestRealtimeSSEReceivesCreateEvent verifies the full end-to-end flow:
// connect SSE → create record via API → receive the realtime event.
func TestRealtimeSSEReceivesCreateEvent(t *testing.T) {
	ctx := context.Background()
	createIntegrationTestSchema(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	// Start a real HTTP server so SSE streaming works.
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	// Connect to SSE endpoint.
	resp, err := http.Get(ts.URL + "/api/realtime?tables=users")
	testutil.NoError(t, err)
	defer resp.Body.Close()
	testutil.Equal(t, resp.StatusCode, http.StatusOK)
	testutil.Equal(t, resp.Header.Get("Content-Type"), "text/event-stream")

	scanner := bufio.NewScanner(resp.Body)

	// Read and verify connected event.
	var connected []string
	for scanner.Scan() {
		line := scanner.Text()
		connected = append(connected, line)
		if line == "" && len(connected) > 1 {
			break
		}
	}
	testutil.Equal(t, connected[0], "event: connected")

	// Create a record via the API.
	body, _ := json.Marshal(map[string]any{"name": "Charlie", "email": "charlie@example.com"})
	createResp, err := http.Post(ts.URL+"/api/collections/users/", "application/json", bytes.NewReader(body))
	testutil.NoError(t, err)
	testutil.Equal(t, createResp.StatusCode, http.StatusCreated)
	createResp.Body.Close()

	// Read the create event from SSE with a timeout.
	eventCh := make(chan []string, 1)
	go func() {
		var lines []string
		for scanner.Scan() {
			line := scanner.Text()
			lines = append(lines, line)
			if line == "" && len(lines) > 1 {
				break
			}
		}
		eventCh <- lines
	}()

	select {
	case lines := <-eventCh:
		testutil.True(t, len(lines) >= 2, "should have event lines")
		testutil.Equal(t, lines[0], "event: create")
		testutil.Contains(t, lines[1], `"table":"users"`)
		testutil.Contains(t, lines[1], `"Charlie"`)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for SSE create event")
	}
}

// TestRealtimeSSEDoesNotReceiveUnsubscribedTable verifies that SSE clients
// only receive events for tables they subscribed to.
func TestRealtimeSSEDoesNotReceiveUnsubscribedTable(t *testing.T) {
	ctx := context.Background()

	// Reset schema with two tables.
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)
	_, err = sharedPG.Pool.Exec(ctx, `
		CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE logs (id SERIAL PRIMARY KEY, message TEXT NOT NULL);
	`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	// Subscribe only to "users".
	resp, err := http.Get(ts.URL + "/api/realtime?tables=users")
	testutil.NoError(t, err)
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	// Skip connected event.
	for scanner.Scan() {
		if scanner.Text() == "" {
			break
		}
	}

	// Create a log record (not subscribed).
	body, _ := json.Marshal(map[string]any{"message": "hello"})
	cr, _ := http.Post(ts.URL+"/api/collections/logs/", "application/json", bytes.NewReader(body))
	cr.Body.Close()

	// Create a user record (subscribed).
	body, _ = json.Marshal(map[string]any{"name": "Dave"})
	cr, _ = http.Post(ts.URL+"/api/collections/users/", "application/json", bytes.NewReader(body))
	cr.Body.Close()

	// The next event should be for users, not logs.
	eventCh := make(chan []string, 1)
	go func() {
		var lines []string
		for scanner.Scan() {
			line := scanner.Text()
			lines = append(lines, line)
			if line == "" && len(lines) > 1 {
				break
			}
		}
		eventCh <- lines
	}()

	select {
	case lines := <-eventCh:
		joined := strings.Join(lines, "\n")
		testutil.Contains(t, joined, `"table":"users"`)
		testutil.Contains(t, joined, `"Dave"`)
		// Should NOT contain logs data.
		testutil.False(t, strings.Contains(joined, `"logs"`), "should not receive logs events")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for SSE event")
	}
}
