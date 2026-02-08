package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Config is the top-level AYB configuration.
type Config struct {
	Server   ServerConfig   `toml:"server"`
	Database DatabaseConfig `toml:"database"`
	Admin    AdminConfig    `toml:"admin"`
	Auth     AuthConfig     `toml:"auth"`
	Email    EmailConfig    `toml:"email"`
	Storage  StorageConfig  `toml:"storage"`
	Logging  LoggingConfig  `toml:"logging"`
}


type ServerConfig struct {
	Host            string   `toml:"host"`
	Port            int      `toml:"port"`
	CORSAllowedOrigins []string `toml:"cors_allowed_origins"`
	BodyLimit       string   `toml:"body_limit"`
	ShutdownTimeout int      `toml:"shutdown_timeout"`
}

type DatabaseConfig struct {
	URL             string `toml:"url"`
	MaxConns        int    `toml:"max_conns"`
	MinConns        int    `toml:"min_conns"`
	HealthCheckSecs int    `toml:"health_check_interval"`
	EmbeddedPort    int    `toml:"embedded_port"`
	EmbeddedDataDir string `toml:"embedded_data_dir"`
	MigrationsDir   string `toml:"migrations_dir"`
}

type AdminConfig struct {
	Enabled  bool   `toml:"enabled"`
	Path     string `toml:"path"`
	Password string `toml:"password"`
}

type AuthConfig struct {
	Enabled              bool                     `toml:"enabled"`
	JWTSecret            string                   `toml:"jwt_secret"`
	TokenDuration        int                      `toml:"token_duration"`
	RefreshTokenDuration int                      `toml:"refresh_token_duration"`
	OAuth                map[string]OAuthProvider `toml:"oauth"`
	OAuthRedirectURL     string                   `toml:"oauth_redirect_url"`
}

// OAuthProvider configures a single OAuth2 provider (e.g. google, github).
type OAuthProvider struct {
	Enabled      bool   `toml:"enabled"`
	ClientID     string `toml:"client_id"`
	ClientSecret string `toml:"client_secret"`
}

// EmailConfig controls how AYB sends transactional emails (verification, password reset).
// When Backend is "" or "log", emails are printed to the console (dev mode).
type EmailConfig struct {
	Backend  string              `toml:"backend"` // "log" (default), "smtp", "webhook"
	From     string              `toml:"from"`
	FromName string              `toml:"from_name"`
	SMTP     EmailSMTPConfig     `toml:"smtp"`
	Webhook  EmailWebhookConfig  `toml:"webhook"`
}

type EmailSMTPConfig struct {
	Host       string `toml:"host"`
	Port       int    `toml:"port"`
	Username   string `toml:"username"`
	Password   string `toml:"password"`
	AuthMethod string `toml:"auth_method"` // PLAIN, LOGIN, CRAM-MD5
	TLS        bool   `toml:"tls"`
}

type EmailWebhookConfig struct {
	URL     string `toml:"url"`
	Secret  string `toml:"secret"`
	Timeout int    `toml:"timeout"` // seconds, default 10
}

type StorageConfig struct {
	Enabled     bool   `toml:"enabled"`
	Backend     string `toml:"backend"`
	LocalPath   string `toml:"local_path"`
	MaxFileSize string `toml:"max_file_size"`
	S3Endpoint  string `toml:"s3_endpoint"`
	S3Bucket    string `toml:"s3_bucket"`
	S3Region    string `toml:"s3_region"`
	S3AccessKey string `toml:"s3_access_key"`
	S3SecretKey string `toml:"s3_secret_key"`
	S3UseSSL    bool   `toml:"s3_use_ssl"`
}

type LoggingConfig struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
}

// Default returns a Config with all defaults applied.
func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Host:            "0.0.0.0",
			Port:            8090,
			CORSAllowedOrigins: []string{"*"},
			BodyLimit:       "1MB",
			ShutdownTimeout: 10,
		},
		Database: DatabaseConfig{
			MaxConns:        25,
			MinConns:        2,
			HealthCheckSecs: 30,
			EmbeddedPort:    15432,
			MigrationsDir:   "./migrations",
		},
		Admin: AdminConfig{
			Enabled: true,
			Path:    "/admin",
		},
		Auth: AuthConfig{
			TokenDuration:        900,    // 15 minutes
			RefreshTokenDuration: 604800, // 7 days
		},
		Email: EmailConfig{
			Backend:  "log",
			FromName: "AllYourBase",
		},
		Storage: StorageConfig{
			Backend:     "local",
			LocalPath:   "./ayb_storage",
			MaxFileSize: "10MB",
			S3Region:    "us-east-1",
			S3UseSSL:    true,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

// Load reads configuration with priority: defaults → ayb.toml → env vars → CLI flags.
// The flags parameter allows CLI flag overrides to be passed in.
func Load(configPath string, flags map[string]string) (*Config, error) {
	cfg := Default()

	// Load from TOML file if it exists.
	if configPath == "" {
		configPath = "ayb.toml"
	}
	if data, err := os.ReadFile(configPath); err == nil {
		if err := toml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", configPath, err)
		}
	}

	// Apply environment variables.
	if err := applyEnv(cfg); err != nil {
		return nil, err
	}

	// Apply CLI flag overrides.
	applyFlags(cfg, flags)

	// Validate.
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return cfg, nil
}

// Validate checks the configuration for invalid values.
func (c *Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535, got %d", c.Server.Port)
	}
	if c.Database.MaxConns < 1 {
		return fmt.Errorf("database.max_conns must be at least 1, got %d", c.Database.MaxConns)
	}
	if c.Database.MinConns < 0 {
		return fmt.Errorf("database.min_conns must be non-negative, got %d", c.Database.MinConns)
	}
	if c.Database.MinConns > c.Database.MaxConns {
		return fmt.Errorf("database.min_conns (%d) cannot exceed database.max_conns (%d)", c.Database.MinConns, c.Database.MaxConns)
	}
	if c.Database.URL == "" && (c.Database.EmbeddedPort < 1 || c.Database.EmbeddedPort > 65535) {
		return fmt.Errorf("database.embedded_port must be between 1 and 65535, got %d", c.Database.EmbeddedPort)
	}
	if c.Auth.Enabled && c.Auth.JWTSecret == "" {
		return fmt.Errorf("auth.jwt_secret is required when auth is enabled")
	}
	if c.Auth.JWTSecret != "" && len(c.Auth.JWTSecret) < 32 {
		return fmt.Errorf("auth.jwt_secret must be at least 32 characters, got %d", len(c.Auth.JWTSecret))
	}
	for name, p := range c.Auth.OAuth {
		if p.Enabled {
			if !c.Auth.Enabled {
				return fmt.Errorf("auth.enabled must be true to use OAuth provider %q", name)
			}
			if p.ClientID == "" {
				return fmt.Errorf("auth.oauth.%s.client_id is required when enabled", name)
			}
			if p.ClientSecret == "" {
				return fmt.Errorf("auth.oauth.%s.client_secret is required when enabled", name)
			}
			switch name {
			case "google", "github":
			default:
				return fmt.Errorf("unsupported OAuth provider %q (supported: google, github)", name)
			}
		}
	}
	switch c.Email.Backend {
	case "", "log":
	case "smtp":
		if c.Email.SMTP.Host == "" {
			return fmt.Errorf("email.smtp.host is required when email backend is \"smtp\"")
		}
		if c.Email.From == "" {
			return fmt.Errorf("email.from is required when email backend is \"smtp\"")
		}
	case "webhook":
		if c.Email.Webhook.URL == "" {
			return fmt.Errorf("email.webhook.url is required when email backend is \"webhook\"")
		}
	default:
		return fmt.Errorf("email.backend must be \"log\", \"smtp\", or \"webhook\", got %q", c.Email.Backend)
	}
	if c.Storage.Enabled {
		switch c.Storage.Backend {
		case "local":
			if c.Storage.LocalPath == "" {
				return fmt.Errorf("storage.local_path is required when storage backend is \"local\"")
			}
		case "s3":
			if c.Storage.S3Endpoint == "" {
				return fmt.Errorf("storage.s3_endpoint is required when storage backend is \"s3\"")
			}
			if c.Storage.S3Bucket == "" {
				return fmt.Errorf("storage.s3_bucket is required when storage backend is \"s3\"")
			}
			if c.Storage.S3AccessKey == "" {
				return fmt.Errorf("storage.s3_access_key is required when storage backend is \"s3\"")
			}
			if c.Storage.S3SecretKey == "" {
				return fmt.Errorf("storage.s3_secret_key is required when storage backend is \"s3\"")
			}
		default:
			return fmt.Errorf("storage.backend must be \"local\" or \"s3\", got %q", c.Storage.Backend)
		}
	}
	if c.Logging.Level != "" {
		switch c.Logging.Level {
		case "debug", "info", "warn", "error":
		default:
			return fmt.Errorf("logging.level must be one of: debug, info, warn, error; got %q", c.Logging.Level)
		}
	}
	return nil
}

// Address returns the host:port string for the server to listen on.
func (c *Config) Address() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

// GenerateDefault writes a commented default ayb.toml to the given path.
func GenerateDefault(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(defaultTOML), 0o644)
}

// ToTOML returns the config serialized as TOML.
func (c *Config) ToTOML() (string, error) {
	data, err := toml.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// envInt reads an integer from the named environment variable.
// Returns an error if the value is set but not a valid integer.
func envInt(name string, dest *int) error {
	v := os.Getenv(name)
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fmt.Errorf("invalid value for %s: %q is not an integer", name, v)
	}
	*dest = n
	return nil
}

func applyEnv(cfg *Config) error {
	if v := os.Getenv("AYB_SERVER_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if err := envInt("AYB_SERVER_PORT", &cfg.Server.Port); err != nil {
		return err
	}
	if v := os.Getenv("AYB_DATABASE_URL"); v != "" {
		cfg.Database.URL = v
	}
	if err := envInt("AYB_DATABASE_EMBEDDED_PORT", &cfg.Database.EmbeddedPort); err != nil {
		return err
	}
	if v := os.Getenv("AYB_DATABASE_EMBEDDED_DATA_DIR"); v != "" {
		cfg.Database.EmbeddedDataDir = v
	}
	if v := os.Getenv("AYB_DATABASE_MIGRATIONS_DIR"); v != "" {
		cfg.Database.MigrationsDir = v
	}
	if v := os.Getenv("AYB_ADMIN_PASSWORD"); v != "" {
		cfg.Admin.Password = v
	}
	if v := os.Getenv("AYB_LOG_LEVEL"); v != "" {
		cfg.Logging.Level = v
	}
	if v := os.Getenv("AYB_CORS_ORIGINS"); v != "" {
		cfg.Server.CORSAllowedOrigins = strings.Split(v, ",")
	}
	if v := os.Getenv("AYB_AUTH_ENABLED"); v != "" {
		cfg.Auth.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("AYB_AUTH_JWT_SECRET"); v != "" {
		cfg.Auth.JWTSecret = v
	}
	if err := envInt("AYB_AUTH_REFRESH_TOKEN_DURATION", &cfg.Auth.RefreshTokenDuration); err != nil {
		return err
	}
	if v := os.Getenv("AYB_AUTH_OAUTH_REDIRECT_URL"); v != "" {
		cfg.Auth.OAuthRedirectURL = v
	}
	// Email config.
	if v := os.Getenv("AYB_EMAIL_BACKEND"); v != "" {
		cfg.Email.Backend = v
	}
	if v := os.Getenv("AYB_EMAIL_FROM"); v != "" {
		cfg.Email.From = v
	}
	if v := os.Getenv("AYB_EMAIL_FROM_NAME"); v != "" {
		cfg.Email.FromName = v
	}
	if v := os.Getenv("AYB_EMAIL_SMTP_HOST"); v != "" {
		cfg.Email.SMTP.Host = v
	}
	if err := envInt("AYB_EMAIL_SMTP_PORT", &cfg.Email.SMTP.Port); err != nil {
		return err
	}
	if v := os.Getenv("AYB_EMAIL_SMTP_USERNAME"); v != "" {
		cfg.Email.SMTP.Username = v
	}
	if v := os.Getenv("AYB_EMAIL_SMTP_PASSWORD"); v != "" {
		cfg.Email.SMTP.Password = v
	}
	if v := os.Getenv("AYB_EMAIL_SMTP_AUTH_METHOD"); v != "" {
		cfg.Email.SMTP.AuthMethod = v
	}
	if v := os.Getenv("AYB_EMAIL_SMTP_TLS"); v != "" {
		cfg.Email.SMTP.TLS = v == "true" || v == "1"
	}
	if v := os.Getenv("AYB_EMAIL_WEBHOOK_URL"); v != "" {
		cfg.Email.Webhook.URL = v
	}
	if v := os.Getenv("AYB_EMAIL_WEBHOOK_SECRET"); v != "" {
		cfg.Email.Webhook.Secret = v
	}
	if err := envInt("AYB_EMAIL_WEBHOOK_TIMEOUT", &cfg.Email.Webhook.Timeout); err != nil {
		return err
	}
	if v := os.Getenv("AYB_STORAGE_ENABLED"); v != "" {
		cfg.Storage.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("AYB_STORAGE_BACKEND"); v != "" {
		cfg.Storage.Backend = v
	}
	if v := os.Getenv("AYB_STORAGE_LOCAL_PATH"); v != "" {
		cfg.Storage.LocalPath = v
	}
	if v := os.Getenv("AYB_STORAGE_MAX_FILE_SIZE"); v != "" {
		cfg.Storage.MaxFileSize = v
	}
	if v := os.Getenv("AYB_STORAGE_S3_ENDPOINT"); v != "" {
		cfg.Storage.S3Endpoint = v
	}
	if v := os.Getenv("AYB_STORAGE_S3_BUCKET"); v != "" {
		cfg.Storage.S3Bucket = v
	}
	if v := os.Getenv("AYB_STORAGE_S3_REGION"); v != "" {
		cfg.Storage.S3Region = v
	}
	if v := os.Getenv("AYB_STORAGE_S3_ACCESS_KEY"); v != "" {
		cfg.Storage.S3AccessKey = v
	}
	if v := os.Getenv("AYB_STORAGE_S3_SECRET_KEY"); v != "" {
		cfg.Storage.S3SecretKey = v
	}
	if v := os.Getenv("AYB_STORAGE_S3_USE_SSL"); v != "" {
		cfg.Storage.S3UseSSL = v == "true" || v == "1"
	}
	applyOAuthEnv(cfg, "google")
	applyOAuthEnv(cfg, "github")
	return nil
}

func applyOAuthEnv(cfg *Config, provider string) {
	prefix := "AYB_AUTH_OAUTH_" + strings.ToUpper(provider) + "_"
	id := os.Getenv(prefix + "CLIENT_ID")
	secret := os.Getenv(prefix + "CLIENT_SECRET")
	enabled := os.Getenv(prefix + "ENABLED")
	if id == "" && secret == "" && enabled == "" {
		return
	}
	if cfg.Auth.OAuth == nil {
		cfg.Auth.OAuth = make(map[string]OAuthProvider)
	}
	p := cfg.Auth.OAuth[provider]
	if id != "" {
		p.ClientID = id
	}
	if secret != "" {
		p.ClientSecret = secret
	}
	if enabled != "" {
		p.Enabled = enabled == "true" || enabled == "1"
	}
	cfg.Auth.OAuth[provider] = p
}

func applyFlags(cfg *Config, flags map[string]string) {
	if flags == nil {
		return
	}
	if v, ok := flags["database-url"]; ok && v != "" {
		cfg.Database.URL = v
	}
	if v, ok := flags["port"]; ok && v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = port
		}
	}
	if v, ok := flags["host"]; ok && v != "" {
		cfg.Server.Host = v
	}
}

// MaxFileSize returns the max file size in bytes, parsed from the config string.
// Supports "10MB", "5MB", etc. Defaults to 10MB if unparseable.
func (c *StorageConfig) MaxFileSizeBytes() int64 {
	s := strings.TrimSpace(strings.ToUpper(c.MaxFileSize))
	s = strings.TrimSuffix(s, "B")
	s = strings.TrimSuffix(s, "M")
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 10 << 20 // 10MB default
	}
	return n << 20
}

const defaultTOML = `# AllYourBase (AYB) Configuration
# Documentation: https://allyourbase.io/docs/config

[server]
# Address to listen on.
host = "0.0.0.0"
port = 8090

# CORS allowed origins. Use ["*"] to allow all.
cors_allowed_origins = ["*"]

# Maximum request body size.
body_limit = "1MB"

# Seconds to wait for in-flight requests during shutdown.
shutdown_timeout = 10

[database]
# PostgreSQL connection URL.
# Leave empty for embedded mode (AYB manages its own PostgreSQL).
# url = "postgresql://user:password@localhost:5432/mydb?sslmode=disable"

# Connection pool settings.
max_conns = 25
min_conns = 2

# Seconds between health check pings.
health_check_interval = 30

# Directory for user SQL migrations (applied by 'ayb migrate up').
migrations_dir = "./migrations"

# Embedded PostgreSQL settings (used when url is not set).
# Port for embedded PostgreSQL.
# embedded_port = 15432
#
# Data directory for embedded PostgreSQL (default: ~/.ayb/data).
# embedded_data_dir = ""

[admin]
# Enable the admin dashboard.
enabled = true

# URL path for the admin dashboard.
path = "/admin"

# Admin dashboard password. Set this to protect the admin UI.
# password = ""

[auth]
# Enable authentication. When true, API endpoints require a valid JWT.
enabled = false

# Secret key for signing JWTs. Must be at least 32 characters.
# Required when auth is enabled.
# jwt_secret = ""

# Access token duration in seconds (default: 15 minutes).
token_duration = 900

# Refresh token duration in seconds (default: 7 days).
refresh_token_duration = 604800

# URL to redirect to after OAuth login (tokens appended as hash fragment).
# oauth_redirect_url = "http://localhost:5173/oauth-callback"

# OAuth providers. Supported: google, github.
# [auth.oauth.google]
# enabled = false
# client_id = ""
# client_secret = ""

# [auth.oauth.github]
# enabled = false
# client_id = ""
# client_secret = ""

[email]
# Email backend: "log" (default, prints to console), "smtp", or "webhook".
# In log mode, verification/reset links are printed to stdout — no setup needed.
backend = "log"

# Sender address and display name.
# from = "noreply@example.com"
from_name = "AllYourBase"

# SMTP settings (backend = "smtp").
# Provider presets — just paste your API key as the password:
#   Resend:  host = "smtp.resend.com", port = 465, tls = true
#   Brevo:   host = "smtp-relay.brevo.com", port = 587
#   AWS SES: host = "email-smtp.us-east-1.amazonaws.com", port = 465, tls = true
# [email.smtp]
# host = ""
# port = 587
# username = ""
# password = ""
# auth_method = "PLAIN"
# tls = false

# Webhook settings (backend = "webhook").
# AYB POSTs JSON {to, subject, html, text} to your URL.
# Signed with HMAC-SHA256 in X-AYB-Signature header if secret is set.
# [email.webhook]
# url = ""
# secret = ""
# timeout = 10

[storage]
# Enable file storage. When true, upload/serve/delete endpoints are available.
enabled = false

# Storage backend: "local" (filesystem) or "s3" (S3-compatible).
backend = "local"

# Directory for local file storage (backend = "local").
local_path = "./ayb_storage"

# Maximum upload file size.
max_file_size = "10MB"

# S3-compatible storage settings (backend = "s3").
# Works with AWS S3, Cloudflare R2, MinIO, DigitalOcean Spaces.
# s3_endpoint = "s3.amazonaws.com"
# s3_bucket = "my-ayb-bucket"
# s3_region = "us-east-1"
# s3_access_key = ""
# s3_secret_key = ""
# s3_use_ssl = true

[logging]
# Log level: debug, info, warn, error.
level = "info"

# Log format: json or text.
format = "json"
`
