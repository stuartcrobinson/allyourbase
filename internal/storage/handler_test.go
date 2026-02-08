package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
)

// fakeBackend is a simple in-memory Backend for handler tests.
type fakeBackend struct {
	files map[string][]byte
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{files: make(map[string][]byte)}
}

func (f *fakeBackend) Put(_ context.Context, bucket, name string, r io.Reader) (int64, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	f.files[bucket+"/"+name] = data
	return int64(len(data)), nil
}

func (f *fakeBackend) Get(_ context.Context, bucket, name string) (io.ReadCloser, error) {
	data, ok := f.files[bucket+"/"+name]
	if !ok {
		return nil, ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (f *fakeBackend) Delete(_ context.Context, bucket, name string) error {
	delete(f.files, bucket+"/"+name)
	return nil
}

func (f *fakeBackend) Exists(_ context.Context, bucket, name string) (bool, error) {
	_, ok := f.files[bucket+"/"+name]
	return ok, nil
}

func newTestService() *Service {
	return &Service{backend: newFakeBackend(), signKey: []byte("test-key"), logger: testutil.DiscardLogger()}
}

// testRouter creates a chi router with the handler mounted, matching the server's mount pattern.
func testRouter(h *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Route("/api/storage", func(r chi.Router) {
		r.Mount("/", h.Routes())
	})
	return r
}

func TestHandleUploadMissingFile(t *testing.T) {
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20)
	router := testRouter(h)

	// Empty multipart form — no "file" field.
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/storage/images", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	testutil.Equal(t, rec.Code, http.StatusBadRequest)
	testutil.True(t, strings.Contains(rec.Body.String(), "file"), "should mention file")
}

func TestHandleUploadInvalidBucket(t *testing.T) {
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20)
	router := testRouter(h)

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, _ := w.CreateFormFile("file", "test.txt")
	fw.Write([]byte("data"))
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/storage/INVALID", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	testutil.Equal(t, rec.Code, http.StatusBadRequest)
}

func TestHandleSignedURLInvalid(t *testing.T) {
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20)
	router := testRouter(h)

	// Request with invalid signature — rejected before hitting DB.
	req := httptest.NewRequest(http.MethodGet, "/api/storage/images/photo.jpg?sig=invalid&exp=9999999999", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	testutil.Equal(t, rec.Code, http.StatusForbidden)

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	testutil.True(t, strings.Contains(resp["message"].(string), "invalid"), "should say invalid")
}

func TestHandleSignedURLExpired(t *testing.T) {
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger(), 10<<20)
	router := testRouter(h)

	// Generate a signed URL that already expired.
	token := svc.SignURL("images", "photo.jpg", -time.Second)
	req := httptest.NewRequest(http.MethodGet, "/api/storage/images/photo.jpg?"+token, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	testutil.Equal(t, rec.Code, http.StatusForbidden)
}

func TestHandleUploadNoContentType(t *testing.T) {
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20)
	router := testRouter(h)

	// Non-multipart request body.
	req := httptest.NewRequest(http.MethodPost, "/api/storage/images", strings.NewReader("not multipart"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	testutil.Equal(t, rec.Code, http.StatusBadRequest)
}

// Note: Tests that exercise full upload/serve/delete/list flows (which require
// database metadata operations) belong in integration tests with a real DB.
// See storage_integration_test.go (requires TEST_DATABASE_URL).
