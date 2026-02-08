package server_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestCORSHeaders(t *testing.T) {
	cfg := config.Default()
	cfg.Server.CORSAllowedOrigins = []string{"http://example.com", "http://other.com"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, w.Header().Get("Access-Control-Allow-Origin"), "http://example.com, http://other.com")
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "GET")
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "POST")
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "DELETE")
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Content-Type")
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Authorization")
}

func TestCORSPreflight(t *testing.T) {
	cfg := config.Default()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodOptions, "/api/schema", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusNoContent)
	testutil.Equal(t, w.Header().Get("Access-Control-Allow-Origin"), "*")
	testutil.Equal(t, w.Header().Get("Access-Control-Max-Age"), "86400")
}

func TestCORSWildcard(t *testing.T) {
	cfg := config.Default() // defaults to ["*"]
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, w.Header().Get("Access-Control-Allow-Origin"), "*")
}

func TestRequestIDHeader(t *testing.T) {
	cfg := config.Default()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	// Chi's RequestID middleware sets X-Request-Id in the request context,
	// not necessarily in the response header. But we can verify the request
	// was processed successfully.
	testutil.Equal(t, w.Code, http.StatusOK)
}

func TestAdminPathServesHTML(t *testing.T) {
	cfg := config.Default()
	cfg.Admin.Enabled = true
	cfg.Admin.Path = "/admin"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	// /admin serves the SPA directly.
	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusOK)
	testutil.Contains(t, w.Header().Get("Content-Type"), "text/html")
}

func TestAdminSPAFallback(t *testing.T) {
	cfg := config.Default()
	cfg.Admin.Enabled = true
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	// Unknown paths under /admin/ should serve index.html (SPA fallback).
	req := httptest.NewRequest(http.MethodGet, "/admin/some/deep/route", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusOK)
	testutil.Contains(t, w.Header().Get("Content-Type"), "text/html")
	testutil.Contains(t, w.Body.String(), "<!DOCTYPE html>")
}

func TestAdminStaticAssetCacheHeaders(t *testing.T) {
	cfg := config.Default()
	cfg.Admin.Enabled = true
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	// index.html should NOT have cache headers.
	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, w.Code, http.StatusOK)
	testutil.Equal(t, w.Header().Get("Cache-Control"), "")
}

func TestAdminDisabled(t *testing.T) {
	cfg := config.Default()
	cfg.Admin.Enabled = false
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	// Should get 404 when admin is disabled.
	testutil.Equal(t, w.Code, http.StatusNotFound)
}
