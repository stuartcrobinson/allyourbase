package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/allyourbase/ayb/internal/api"
	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Server is the main HTTP server for AYB.
type Server struct {
	cfg       *config.Config
	router    *chi.Mux
	http      *http.Server
	logger    *slog.Logger
	schema    *schema.CacheHolder
	pool      *pgxpool.Pool
	authRL    *auth.RateLimiter // nil when auth disabled
	hub       *realtime.Hub
	adminAuth *adminAuth // nil when admin.password not set
}

// New creates a new Server with middleware and routes configured.
// authSvc and storageSvc may be nil when their features are disabled.
func New(cfg *config.Config, logger *slog.Logger, schemaCache *schema.CacheHolder, pool *pgxpool.Pool, authSvc *auth.Service, storageSvc *storage.Service) *Server {
	r := chi.NewRouter()

	// Global middleware (applies to all routes including admin SPA).
	r.Use(middleware.RequestID)
	r.Use(requestLogger(logger))
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware(cfg.Server.CORSAllowedOrigins))

	hub := realtime.NewHub(logger)

	s := &Server{
		cfg:    cfg,
		router: r,
		logger: logger,
		schema: schemaCache,
		pool:   pool,
		hub:    hub,
	}
	if cfg.Admin.Password != "" {
		s.adminAuth = newAdminAuth(cfg.Admin.Password)
	}

	// Health check (no content-type restriction).
	r.Get("/health", s.handleHealth)

	r.Route("/api", func(r chi.Router) {
		// Admin auth endpoints (no content-type enforcement — login needs JSON, status is GET).
		r.Get("/admin/status", s.handleAdminStatus)
		r.Post("/admin/auth", s.handleAdminLogin)

		// Storage routes accept multipart/form-data, mounted outside JSON content-type enforcement.
		if storageSvc != nil {
			storageHandler := storage.NewHandler(storageSvc, logger, cfg.Storage.MaxFileSizeBytes())
			r.Route("/storage", func(r chi.Router) {
				if authSvc != nil {
					r.Use(auth.OptionalAuth(authSvc))
				}
				r.Mount("/", storageHandler.Routes())
			})
		}

		// JSON API routes get content-type enforcement.
		r.Group(func(r chi.Router) {
			r.Use(middleware.AllowContentType("application/json"))

			// Auth endpoints (public, rate-limited).
			if authSvc != nil {
				authHandler := auth.NewHandler(authSvc, logger)
				// Configure OAuth providers from config.
				for name, p := range cfg.Auth.OAuth {
					if p.Enabled {
						authHandler.SetOAuthProvider(name, auth.OAuthClientConfig{
							ClientID:     p.ClientID,
							ClientSecret: p.ClientSecret,
						})
					}
				}
				if cfg.Auth.OAuthRedirectURL != "" {
					authHandler.SetOAuthRedirectURL(cfg.Auth.OAuthRedirectURL)
				}
				s.authRL = auth.NewRateLimiter(10, time.Minute)
				r.Route("/auth", func(r chi.Router) {
					r.Use(s.authRL.Middleware)
					r.Mount("/", authHandler.Routes())
				})
			}

			r.Get("/schema", s.handleSchema)

			// Realtime SSE (handles its own auth for EventSource compatibility).
			rtHandler := realtime.NewHandler(hub, pool, authSvc, schemaCache, logger)
			r.Get("/realtime", rtHandler.ServeHTTP)

			// Mount auto-generated CRUD API.
			if pool != nil {
				apiHandler := api.NewHandler(pool, schemaCache, logger, hub)
				if authSvc != nil {
					r.Group(func(r chi.Router) {
						r.Use(auth.RequireAuth(authSvc))
						r.Mount("/", apiHandler.Routes())
					})
				} else {
					r.Mount("/", apiHandler.Routes())
				}
			}
		})
	})

	// Admin SPA (served from embedded UI assets).
	if cfg.Admin.Enabled {
		adminPath := cfg.Admin.Path
		if adminPath == "" {
			adminPath = "/admin"
		}
		spa := staticSPAHandler()
		// Mount under a Route group to avoid chi wildcard/redirect conflicts.
		r.Route(adminPath, func(sub chi.Router) {
			sub.Get("/", spa)
			sub.Get("/*", spa)
		})
	}

	return s
}

// Router returns the chi router for registering additional routes.
func (s *Server) Router() *chi.Mux {
	return s.router
}

// Start begins listening for HTTP requests.
func (s *Server) Start() error {
	s.http = &http.Server{
		Addr:    s.cfg.Address(),
		Handler: s.router,
	}

	s.logger.Info("server starting", "address", s.cfg.Address())
	if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	timeout := time.Duration(s.cfg.Server.ShutdownTimeout) * time.Second
	shutdownCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	s.logger.Info("shutting down server", "timeout", timeout)
	if s.authRL != nil {
		s.authRL.Stop()
	}
	s.hub.Close()
	return s.http.Shutdown(shutdownCtx)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleSchema(w http.ResponseWriter, r *http.Request) {
	sc := s.schema.Get()
	if sc == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "schema cache not ready")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, sc)
}
