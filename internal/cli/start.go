package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/mailer"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/pgmanager"
	"github.com/allyourbase/ayb/internal/postgres"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the AYB server",
	Long: `Start the AllYourBase server. If no database URL is configured,
AYB starts an embedded PostgreSQL instance automatically.

With external database:
  ayb start --database-url postgresql://user:pass@localhost:5432/mydb`,
	RunE: runStart,
}

func init() {
	startCmd.Flags().String("database-url", "", "PostgreSQL connection URL")
	startCmd.Flags().Int("port", 0, "Server port (default 8090)")
	startCmd.Flags().String("host", "", "Server host (default 0.0.0.0)")
	startCmd.Flags().String("config", "", "Path to ayb.toml config file")
}

func runStart(cmd *cobra.Command, args []string) error {
	// Collect CLI flag overrides.
	flags := make(map[string]string)
	if v, _ := cmd.Flags().GetString("database-url"); v != "" {
		flags["database-url"] = v
	}
	if v, _ := cmd.Flags().GetInt("port"); v != 0 {
		flags["port"] = fmt.Sprintf("%d", v)
	}
	if v, _ := cmd.Flags().GetString("host"); v != "" {
		flags["host"] = v
	}

	configPath, _ := cmd.Flags().GetString("config")

	// Load config (defaults → file → env → flags).
	cfg, err := config.Load(configPath, flags)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Set up logger.
	logger := newLogger(cfg.Logging.Level, cfg.Logging.Format)

	logger.Info("starting AllYourBase",
		"version", buildVersion,
		"address", cfg.Address(),
	)

	// Auto-generate config file if it doesn't exist.
	if configPath == "" {
		if _, err := os.Stat("ayb.toml"); os.IsNotExist(err) {
			if err := config.GenerateDefault("ayb.toml"); err != nil {
				logger.Warn("could not generate default ayb.toml", "error", err)
			} else {
				logger.Info("generated default ayb.toml")
			}
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start embedded PostgreSQL if no database URL is configured.
	var pgMgr *pgmanager.Manager
	if cfg.Database.URL == "" {
		logger.Info("no database URL configured, starting embedded PostgreSQL")
		pgMgr = pgmanager.New(pgmanager.Config{
			Port:    uint32(cfg.Database.EmbeddedPort),
			DataDir: cfg.Database.EmbeddedDataDir,
			Logger:  logger,
		})
		connURL, err := pgMgr.Start(ctx)
		if err != nil {
			return fmt.Errorf("starting embedded postgres: %w", err)
		}
		cfg.Database.URL = connURL
	}

	// Connect to PostgreSQL.
	pool, err := postgres.New(ctx, postgres.Config{
		URL:             cfg.Database.URL,
		MaxConns:        int32(cfg.Database.MaxConns),
		MinConns:        int32(cfg.Database.MinConns),
		HealthCheckSecs: cfg.Database.HealthCheckSecs,
	}, logger)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	// Run system migrations.
	migRunner := migrations.NewRunner(pool.DB(), logger)
	if err := migRunner.Bootstrap(ctx); err != nil {
		return fmt.Errorf("bootstrapping migrations: %w", err)
	}
	applied, err := migRunner.Run(ctx)
	if err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}
	if applied > 0 {
		logger.Info("applied system migrations", "count", applied)
	}

	// Apply user migrations if the directory exists.
	if cfg.Database.MigrationsDir != "" {
		if _, err := os.Stat(cfg.Database.MigrationsDir); err == nil {
			userRunner := migrations.NewUserRunner(pool.DB(), cfg.Database.MigrationsDir, logger)
			if err := userRunner.Bootstrap(ctx); err != nil {
				return fmt.Errorf("bootstrapping user migrations: %w", err)
			}
			userApplied, err := userRunner.Up(ctx)
			if err != nil {
				return fmt.Errorf("running user migrations: %w", err)
			}
			if userApplied > 0 {
				logger.Info("applied user migrations", "count", userApplied)
			}
		}
	}

	// Initialize schema cache and start watcher.
	schemaCache := schema.NewCacheHolder(pool.DB(), logger)
	watcher := schema.NewWatcher(schemaCache, pool.DB(), cfg.Database.URL, logger)

	watcherCtx, watcherCancel := context.WithCancel(ctx)
	defer watcherCancel()

	watcherErrCh := make(chan error, 1)
	go func() {
		watcherErrCh <- watcher.Start(watcherCtx)
	}()

	// Wait for initial schema load before starting HTTP server.
	// The watcher's Start() loads the cache synchronously, then enters
	// the background listen/poll loop. We give it a moment to complete.
	select {
	case err := <-watcherErrCh:
		// Watcher returned early — this means initial load failed.
		return fmt.Errorf("schema watcher: %w", err)
	case <-schemaCache.Ready():
		logger.Info("schema cache ready")
	}

	// Conditionally create auth service.
	var authSvc *auth.Service
	if cfg.Auth.Enabled {
		authSvc = auth.NewService(
			pool.DB(),
			cfg.Auth.JWTSecret,
			time.Duration(cfg.Auth.TokenDuration)*time.Second,
			time.Duration(cfg.Auth.RefreshTokenDuration)*time.Second,
			logger,
		)

		// Build mailer and inject into auth service.
		m := buildMailer(cfg, logger)
		baseURL := fmt.Sprintf("http://%s/api", cfg.Address())
		authSvc.SetMailer(m, cfg.Email.FromName, baseURL)
		logger.Info("auth enabled", "email_backend", cfg.Email.Backend)
	}

	// Conditionally create storage service.
	var storageSvc *storage.Service
	if cfg.Storage.Enabled {
		var storageBackend storage.Backend
		switch cfg.Storage.Backend {
		case "s3":
			s3b, err := storage.NewS3Backend(ctx, storage.S3Config{
				Endpoint:  cfg.Storage.S3Endpoint,
				Bucket:    cfg.Storage.S3Bucket,
				Region:    cfg.Storage.S3Region,
				AccessKey: cfg.Storage.S3AccessKey,
				SecretKey: cfg.Storage.S3SecretKey,
				UseSSL:    cfg.Storage.S3UseSSL,
			})
			if err != nil {
				return fmt.Errorf("initializing S3 storage backend: %w", err)
			}
			storageBackend = s3b
			logger.Info("storage enabled", "backend", "s3", "endpoint", cfg.Storage.S3Endpoint, "bucket", cfg.Storage.S3Bucket)
		default:
			lb, err := storage.NewLocalBackend(cfg.Storage.LocalPath)
			if err != nil {
				return fmt.Errorf("initializing local storage backend: %w", err)
			}
			storageBackend = lb
			logger.Info("storage enabled", "backend", "local", "path", cfg.Storage.LocalPath)
		}
		signKey := cfg.Auth.JWTSecret
		if signKey == "" {
			b := make([]byte, 32)
			if _, err := rand.Read(b); err != nil {
				return fmt.Errorf("generating storage sign key: %w", err)
			}
			signKey = hex.EncodeToString(b)
			logger.Info("generated random storage sign key (signed URLs will not survive restarts)")
		}
		storageSvc = storage.NewService(pool.DB(), storageBackend, signKey, logger)
	}

	// Create and start HTTP server.
	srv := server.New(cfg, logger, schemaCache, pool.DB(), authSvc, storageSvc)

	// Graceful shutdown on SIGTERM/SIGINT.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	select {
	case err := <-errCh:
		if pgMgr != nil {
			if stopErr := pgMgr.Stop(); stopErr != nil {
				logger.Error("error stopping embedded postgres", "error", stopErr)
			}
		}
		return err
	case sig := <-sigCh:
		logger.Info("received signal, shutting down", "signal", sig)
		watcherCancel()
		if err := srv.Shutdown(ctx); err != nil {
			logger.Error("shutdown error", "error", err)
		}
		if pgMgr != nil {
			if stopErr := pgMgr.Stop(); stopErr != nil {
				logger.Error("error stopping embedded postgres", "error", stopErr)
			}
		}
		return nil
	}
}

func buildMailer(cfg *config.Config, logger *slog.Logger) mailer.Mailer {
	switch cfg.Email.Backend {
	case "smtp":
		port := cfg.Email.SMTP.Port
		if port == 0 {
			port = 587
		}
		return mailer.NewSMTPMailer(mailer.SMTPConfig{
			Host:       cfg.Email.SMTP.Host,
			Port:       port,
			Username:   cfg.Email.SMTP.Username,
			Password:   cfg.Email.SMTP.Password,
			From:       cfg.Email.From,
			FromName:   cfg.Email.FromName,
			TLS:        cfg.Email.SMTP.TLS,
			AuthMethod: cfg.Email.SMTP.AuthMethod,
		})
	case "webhook":
		timeout := time.Duration(cfg.Email.Webhook.Timeout) * time.Second
		if timeout == 0 {
			timeout = 10 * time.Second
		}
		return mailer.NewWebhookMailer(mailer.WebhookConfig{
			URL:     cfg.Email.Webhook.URL,
			Secret:  cfg.Email.Webhook.Secret,
			Timeout: timeout,
		})
	default:
		return mailer.NewLogMailer(logger)
	}
}

func newLogger(level, format string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}

	var handler slog.Handler
	if format == "text" {
		handler = slog.NewTextHandler(os.Stderr, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	}

	return slog.New(handler)
}
