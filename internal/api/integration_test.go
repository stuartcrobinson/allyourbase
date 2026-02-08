//go:build integration

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

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

// resetAndSeedDB drops the public schema and recreates the test tables with seed data.
func resetAndSeedDB(t *testing.T, ctx context.Context) {
	t.Helper()

	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	if err != nil {
		t.Fatalf("resetting schema: %v", err)
	}

	_, err = sharedPG.Pool.Exec(ctx, `
		CREATE TABLE authors (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL
		);
		CREATE TABLE posts (
			id SERIAL PRIMARY KEY,
			title TEXT NOT NULL,
			body TEXT,
			author_id INTEGER REFERENCES authors(id),
			status TEXT DEFAULT 'draft',
			created_at TIMESTAMPTZ DEFAULT now()
		);
		CREATE TABLE tags (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL UNIQUE
		);

		INSERT INTO authors (name) VALUES ('Alice'), ('Bob');
		INSERT INTO posts (title, body, author_id, status) VALUES
			('First Post', 'Hello world', 1, 'published'),
			('Second Post', 'Another post', 1, 'draft'),
			('Bob Post', 'By Bob', 2, 'published');
		INSERT INTO tags (name) VALUES ('go'), ('api'), ('test');
	`)
	if err != nil {
		t.Fatalf("creating test schema: %v", err)
	}
}

func setupTestServer(t *testing.T, ctx context.Context) (*server.Server, *testutil.PGContainer) {
	t.Helper()

	resetAndSeedDB(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}

	cfg := config.Default()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	return srv, sharedPG
}

func doRequest(t *testing.T, srv *server.Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	return w
}

func parseJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("parsing JSON response: %v\nbody: %s", err, w.Body.String())
	}
	return result
}

// jsonNum extracts a float64 from a JSON-decoded map value.
func jsonNum(t *testing.T, v any) float64 {
	t.Helper()
	f, ok := v.(float64)
	if !ok {
		t.Fatalf("expected float64, got %T: %v", v, v)
	}
	return f
}

func jsonStr(t *testing.T, v any) string {
	t.Helper()
	s, ok := v.(string)
	if !ok {
		t.Fatalf("expected string, got %T: %v", v, v)
	}
	return s
}

func jsonItems(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()
	raw, ok := body["items"].([]any)
	if !ok {
		t.Fatalf("expected items array, got %T", body["items"])
	}
	items := make([]map[string]any, len(raw))
	for i, v := range raw {
		items[i] = v.(map[string]any)
	}
	return items
}

// --- List tests ---

func TestListRecords(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/", nil)
	testutil.Equal(t, w.Code, http.StatusOK)

	body := parseJSON(t, w)
	testutil.Equal(t, jsonNum(t, body["page"]), 1.0)
	testutil.Equal(t, jsonNum(t, body["perPage"]), 20.0)
	testutil.Equal(t, jsonNum(t, body["totalItems"]), 3.0)

	items := jsonItems(t, body)
	testutil.Equal(t, len(items), 3)
}

func TestListPagination(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?page=1&perPage=2", nil)
	testutil.Equal(t, w.Code, http.StatusOK)

	body := parseJSON(t, w)
	testutil.Equal(t, jsonNum(t, body["page"]), 1.0)
	testutil.Equal(t, jsonNum(t, body["perPage"]), 2.0)
	testutil.Equal(t, jsonNum(t, body["totalItems"]), 3.0)
	testutil.Equal(t, jsonNum(t, body["totalPages"]), 2.0)

	items := jsonItems(t, body)
	testutil.Equal(t, len(items), 2)
}

func TestListPaginationPage2(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?page=2&perPage=2", nil)
	testutil.Equal(t, w.Code, http.StatusOK)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, len(items), 1)
}

func TestListSkipTotal(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?skipTotal=true", nil)
	testutil.Equal(t, w.Code, http.StatusOK)

	body := parseJSON(t, w)
	testutil.Equal(t, jsonNum(t, body["totalItems"]), -1.0)
	testutil.Equal(t, jsonNum(t, body["totalPages"]), -1.0)
}

func TestListWithSort(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?sort=-id", nil)
	testutil.Equal(t, w.Code, http.StatusOK)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, jsonNum(t, items[0]["id"]), 3.0) // highest ID first
}

func TestListWithFields(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?fields=id,title", nil)
	testutil.Equal(t, w.Code, http.StatusOK)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	first := items[0]
	testutil.NotNil(t, first["id"])
	testutil.NotNil(t, first["title"])
	_, hasBody := first["body"]
	testutil.False(t, hasBody, "body field should not be present")
}

func TestListWithFilter(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?filter=status%3D'published'", nil)
	testutil.Equal(t, w.Code, http.StatusOK)

	body := parseJSON(t, w)
	testutil.Equal(t, jsonNum(t, body["totalItems"]), 2.0)
}

func TestListWithFilterAnd(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?filter=status%3D'published'+AND+author_id%3D1", nil)
	testutil.Equal(t, w.Code, http.StatusOK)

	body := parseJSON(t, w)
	testutil.Equal(t, jsonNum(t, body["totalItems"]), 1.0)
}

func TestListInvalidFilter(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?filter=nonexistent%3D'x'", nil)
	testutil.Equal(t, w.Code, http.StatusBadRequest)
}

func TestListCollectionNotFound(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/nonexistent/", nil)
	testutil.Equal(t, w.Code, http.StatusNotFound)
}

// --- Read single record tests ---

func TestReadRecord(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/1", nil)
	testutil.Equal(t, w.Code, http.StatusOK)

	body := parseJSON(t, w)
	testutil.Equal(t, jsonNum(t, body["id"]), 1.0)
	testutil.Equal(t, jsonStr(t, body["title"]), "First Post")
}

func TestReadRecordNotFound(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/999", nil)
	testutil.Equal(t, w.Code, http.StatusNotFound)
}

func TestReadRecordWithFields(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/1?fields=id,title", nil)
	testutil.Equal(t, w.Code, http.StatusOK)

	body := parseJSON(t, w)
	testutil.NotNil(t, body["id"])
	testutil.NotNil(t, body["title"])
	_, hasBody := body["body"]
	testutil.False(t, hasBody, "body should not be present")
}

// --- Create tests ---

func TestCreateRecord(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	data := map[string]any{"name": "Charlie"}
	w := doRequest(t, srv, "POST", "/api/collections/authors/", data)
	testutil.Equal(t, w.Code, http.StatusCreated)

	body := parseJSON(t, w)
	testutil.Equal(t, jsonStr(t, body["name"]), "Charlie")
	testutil.NotNil(t, body["id"])
}

func TestCreateRecordInvalidJSON(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	req := httptest.NewRequest("POST", "/api/collections/authors/", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, w.Code, http.StatusBadRequest)
}

func TestCreateRecordEmptyBody(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "POST", "/api/collections/authors/", map[string]any{})
	testutil.Equal(t, w.Code, http.StatusBadRequest)
}

func TestCreateRecordNotNullViolation(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// authors.name is NOT NULL.
	data := map[string]any{"id": 100}
	w := doRequest(t, srv, "POST", "/api/collections/authors/", data)
	testutil.Equal(t, w.Code, http.StatusBadRequest)

	body := parseJSON(t, w)
	testutil.Contains(t, jsonStr(t, body["message"]), "missing required")
}

func TestCreateRecordUniqueViolation(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// tags.name has UNIQUE constraint.
	data := map[string]any{"name": "go"} // already exists
	w := doRequest(t, srv, "POST", "/api/collections/tags/", data)
	testutil.Equal(t, w.Code, http.StatusConflict)
}

// --- Update tests ---

func TestUpdateRecord(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	data := map[string]any{"title": "Updated Title"}
	w := doRequest(t, srv, "PATCH", "/api/collections/posts/1", data)
	testutil.Equal(t, w.Code, http.StatusOK)

	body := parseJSON(t, w)
	testutil.Equal(t, jsonStr(t, body["title"]), "Updated Title")
	testutil.Equal(t, jsonNum(t, body["id"]), 1.0)
}

func TestUpdateRecordNotFound(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	data := map[string]any{"title": "nope"}
	w := doRequest(t, srv, "PATCH", "/api/collections/posts/999", data)
	testutil.Equal(t, w.Code, http.StatusNotFound)
}

func TestUpdateRecordEmptyBody(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "PATCH", "/api/collections/posts/1", map[string]any{})
	testutil.Equal(t, w.Code, http.StatusBadRequest)
}

// --- Delete tests ---

func TestDeleteRecord(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "DELETE", "/api/collections/tags/3", nil)
	testutil.Equal(t, w.Code, http.StatusNoContent)

	// Verify it's gone.
	w = doRequest(t, srv, "GET", "/api/collections/tags/3", nil)
	testutil.Equal(t, w.Code, http.StatusNotFound)
}

func TestDeleteRecordNotFound(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "DELETE", "/api/collections/tags/999", nil)
	testutil.Equal(t, w.Code, http.StatusNotFound)
}

// --- Expand tests ---

func TestReadWithExpand(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Test expand by FK column name (author_id).
	w := doRequest(t, srv, "GET", "/api/collections/posts/1?expand=author_id", nil)
	testutil.Equal(t, w.Code, http.StatusOK)

	body := parseJSON(t, w)
	testutil.Equal(t, jsonStr(t, body["title"]), "First Post")

	expandData, ok := body["expand"]
	if !ok {
		t.Fatal("expand key not present in response")
	}

	expandMap := expandData.(map[string]any)
	author, ok := expandMap["author"].(map[string]any)
	if !ok {
		t.Fatalf("expected expand.author to be a map, got %T", expandMap["author"])
	}
	testutil.Equal(t, jsonStr(t, author["name"]), "Alice")
}

func TestReadWithExpandFriendlyName(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Test expand by friendly name (author, derived from author_id).
	w := doRequest(t, srv, "GET", "/api/collections/posts/1?expand=author", nil)
	testutil.Equal(t, w.Code, http.StatusOK)

	body := parseJSON(t, w)
	expandData, ok := body["expand"]
	if !ok {
		t.Fatal("expand key not present in response")
	}

	expandMap := expandData.(map[string]any)
	author, ok := expandMap["author"].(map[string]any)
	if !ok {
		t.Fatalf("expected expand.author to be a map, got %T", expandMap["author"])
	}
	testutil.Equal(t, jsonStr(t, author["name"]), "Alice")
}

func TestListWithExpand(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?expand=author", nil)
	testutil.Equal(t, w.Code, http.StatusOK)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.True(t, len(items) > 0, "expected items")

	// Every post with an author_id should have an expand.author entry.
	for _, item := range items {
		if item["author_id"] == nil {
			continue
		}
		expandData, ok := item["expand"]
		if !ok {
			t.Fatal("expand key not present on item with author_id")
		}
		expandMap := expandData.(map[string]any)
		author, ok := expandMap["author"].(map[string]any)
		if !ok {
			t.Fatalf("expected expand.author to be a map, got %T", expandMap["author"])
		}
		testutil.NotNil(t, author["name"])
	}
}

// --- One-to-many expand test ---

func TestListWithOneToManyExpand(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Expand posts from an author (one-to-many).
	w := doRequest(t, srv, "GET", "/api/collections/authors/1?expand=posts", nil)
	testutil.Equal(t, w.Code, http.StatusOK)

	body := parseJSON(t, w)
	testutil.Equal(t, jsonStr(t, body["name"]), "Alice")

	expandData, ok := body["expand"]
	if !ok {
		t.Fatal("expand key not present — one-to-many expand failed")
	}
	expandMap := expandData.(map[string]any)
	posts, ok := expandMap["posts"].([]any)
	if !ok {
		t.Fatalf("expected expand.posts to be an array, got %T", expandMap["posts"])
	}
	testutil.Equal(t, len(posts), 2) // Alice has 2 posts
}

// --- Validation tests ---

func TestCreateAllUnknownColumns(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	data := map[string]any{"nonexistent_col": "value", "also_fake": 123}
	w := doRequest(t, srv, "POST", "/api/collections/authors/", data)
	testutil.Equal(t, w.Code, http.StatusBadRequest)

	body := parseJSON(t, w)
	testutil.Contains(t, jsonStr(t, body["message"]), "no recognized columns")
}

func TestUpdateAllUnknownColumns(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	data := map[string]any{"nonexistent_col": "value"}
	w := doRequest(t, srv, "PATCH", "/api/collections/posts/1", data)
	testutil.Equal(t, w.Code, http.StatusBadRequest)

	body := parseJSON(t, w)
	testutil.Contains(t, jsonStr(t, body["message"]), "no recognized columns")
}

// --- Edge case tests ---

func TestViewReadOnly(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Create a view.
	_, err := pg.Pool.Exec(ctx, `CREATE VIEW active_posts AS SELECT * FROM posts WHERE status = 'published'`)
	if err != nil {
		t.Fatalf("creating view: %v", err)
	}

	// Reload schema to pick up the view.
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("reloading schema: %v", err)
	}
	cfg := config.Default()
	srv = server.New(cfg, logger, ch, pg.Pool, nil, nil)

	// GET should work.
	w := doRequest(t, srv, "GET", "/api/collections/active_posts/", nil)
	testutil.Equal(t, w.Code, http.StatusOK)

	// POST should be rejected.
	data := map[string]any{"title": "test"}
	w = doRequest(t, srv, "POST", "/api/collections/active_posts/", data)
	testutil.Equal(t, w.Code, http.StatusMethodNotAllowed)
}

// --- Error format tests ---

func TestErrorResponseFormat(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/nonexistent/", nil)
	testutil.Equal(t, w.Code, http.StatusNotFound)

	body := parseJSON(t, w)
	testutil.Equal(t, jsonNum(t, body["code"]), 404.0)
	testutil.True(t, body["message"] != nil, "expected message")
}

// --- Combined sort + filter + pagination ---

func TestCombinedFilterSortPagination(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Filter published, sort by id desc, page 1 perPage 1.
	w := doRequest(t, srv, "GET", "/api/collections/posts/?filter=status%3D'published'&sort=-id&page=1&perPage=1", nil)
	testutil.Equal(t, w.Code, http.StatusOK)

	body := parseJSON(t, w)
	testutil.Equal(t, jsonNum(t, body["totalItems"]), 2.0)
	testutil.Equal(t, jsonNum(t, body["totalPages"]), 2.0)

	items := jsonItems(t, body)
	testutil.Equal(t, len(items), 1)
	testutil.Equal(t, jsonNum(t, items[0]["id"]), 3.0) // Bob Post, highest published ID
}
