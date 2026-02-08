//go:build integration

package storage_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/testutil"
)

var (
	sharedPG      *testutil.PGContainer
	sharedCleanup func()
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	sharedPG = pg
	sharedCleanup = cleanup
	code := m.Run()
	sharedCleanup()
	os.Exit(code)
}

func setupServer(t *testing.T) *httptest.Server {
	t.Helper()

	ctx := context.Background()
	pool := sharedPG.Pool
	logger := testutil.DiscardLogger()

	// Run migrations.
	runner := migrations.NewRunner(pool, logger)
	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if _, err := runner.Run(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Create local storage backend.
	dir := t.TempDir()
	backend, err := storage.NewLocalBackend(dir)
	if err != nil {
		t.Fatalf("backend: %v", err)
	}

	storageSvc := storage.NewService(pool, backend, "test-sign-key-at-least-32-chars!!", logger)

	cfg := config.Default()
	cfg.Storage.Enabled = true
	ch := schema.NewCacheHolder(pool, logger)

	srv := server.New(cfg, logger, ch, pool, nil, storageSvc)
	return httptest.NewServer(srv.Router())
}

func TestStorageUploadAndServe(t *testing.T) {
	ts := setupServer(t)
	defer ts.Close()

	// Upload a file.
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, _ := w.CreateFormFile("file", "hello.txt")
	fw.Write([]byte("Hello, Storage!"))
	w.Close()

	resp, err := http.Post(ts.URL+"/api/storage/testbucket", w.FormDataContentType(), body)
	testutil.NoError(t, err)
	testutil.Equal(t, resp.StatusCode, http.StatusCreated)

	var obj map[string]any
	json.NewDecoder(resp.Body).Decode(&obj)
	resp.Body.Close()

	testutil.Equal(t, obj["bucket"], "testbucket")
	testutil.Equal(t, obj["name"], "hello.txt")
	testutil.Equal(t, obj["size"].(float64), float64(15))

	// Serve the file.
	resp, err = http.Get(ts.URL + "/api/storage/testbucket/hello.txt")
	testutil.NoError(t, err)
	testutil.Equal(t, resp.StatusCode, http.StatusOK)
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	testutil.Equal(t, string(got), "Hello, Storage!")
}

func TestStorageDelete(t *testing.T) {
	ts := setupServer(t)
	defer ts.Close()

	// Upload.
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, _ := w.CreateFormFile("file", "delete-me.txt")
	fw.Write([]byte("bye"))
	w.Close()

	resp, _ := http.Post(ts.URL+"/api/storage/testbucket", w.FormDataContentType(), body)
	resp.Body.Close()
	testutil.Equal(t, resp.StatusCode, http.StatusCreated)

	// Delete.
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/storage/testbucket/delete-me.txt", nil)
	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	testutil.Equal(t, resp.StatusCode, http.StatusNoContent)
	resp.Body.Close()

	// Serve should 404.
	resp, _ = http.Get(ts.URL + "/api/storage/testbucket/delete-me.txt")
	testutil.Equal(t, resp.StatusCode, http.StatusNotFound)
	resp.Body.Close()
}

func TestStorageList(t *testing.T) {
	ts := setupServer(t)
	defer ts.Close()

	// Upload 3 files.
	for i := 0; i < 3; i++ {
		body := &bytes.Buffer{}
		w := multipart.NewWriter(body)
		fw, _ := w.CreateFormFile("file", fmt.Sprintf("file%d.txt", i))
		fw.Write([]byte(fmt.Sprintf("content %d", i)))
		w.Close()
		resp, _ := http.Post(ts.URL+"/api/storage/listbucket", w.FormDataContentType(), body)
		resp.Body.Close()
	}

	resp, err := http.Get(ts.URL + "/api/storage/listbucket")
	testutil.NoError(t, err)
	testutil.Equal(t, resp.StatusCode, http.StatusOK)

	var list map[string]any
	json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()

	testutil.Equal(t, list["totalItems"].(float64), float64(3))
	items := list["items"].([]any)
	testutil.Equal(t, len(items), 3)
}

func TestStorageSignedURL(t *testing.T) {
	ts := setupServer(t)
	defer ts.Close()

	// Upload.
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, _ := w.CreateFormFile("file", "signed.txt")
	fw.Write([]byte("signed content"))
	w.Close()

	resp, _ := http.Post(ts.URL+"/api/storage/signbucket", w.FormDataContentType(), body)
	resp.Body.Close()

	// Generate signed URL.
	signBody := bytes.NewReader([]byte(`{"expiresIn": 3600}`))
	resp, err := http.Post(ts.URL+"/api/storage/signbucket/signed.txt/sign", "application/json", signBody)
	testutil.NoError(t, err)
	testutil.Equal(t, resp.StatusCode, http.StatusOK)

	var signResp map[string]string
	json.NewDecoder(resp.Body).Decode(&signResp)
	resp.Body.Close()

	signedURL := signResp["url"]
	testutil.True(t, signedURL != "", "should have a URL")

	// Fetch via signed URL.
	resp, err = http.Get(ts.URL + signedURL)
	testutil.NoError(t, err)
	testutil.Equal(t, resp.StatusCode, http.StatusOK)
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	testutil.Equal(t, string(got), "signed content")
}
