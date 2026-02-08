package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	testutil.Equal(t, cfg.Server.Host, "0.0.0.0")
	testutil.Equal(t, cfg.Server.Port, 8090)
	testutil.Equal(t, cfg.Server.BodyLimit, "1MB")
	testutil.Equal(t, cfg.Server.ShutdownTimeout, 10)
	testutil.SliceLen(t, cfg.Server.CORSAllowedOrigins, 1)
	testutil.Equal(t, cfg.Server.CORSAllowedOrigins[0], "*")

	testutil.Equal(t, cfg.Database.MaxConns, 25)
	testutil.Equal(t, cfg.Database.MinConns, 2)
	testutil.Equal(t, cfg.Database.HealthCheckSecs, 30)
	testutil.Equal(t, cfg.Database.EmbeddedPort, 15432)
	testutil.Equal(t, cfg.Database.EmbeddedDataDir, "")

	testutil.Equal(t, cfg.Admin.Enabled, true)
	testutil.Equal(t, cfg.Admin.Path, "/admin")

	testutil.Equal(t, cfg.Auth.Enabled, false)
	testutil.Equal(t, cfg.Auth.JWTSecret, "")
	testutil.Equal(t, cfg.Auth.TokenDuration, 900)
	testutil.Equal(t, cfg.Auth.RefreshTokenDuration, 604800)

	testutil.Equal(t, cfg.Email.Backend, "log")
	testutil.Equal(t, cfg.Email.FromName, "AllYourBase")
	testutil.Equal(t, cfg.Email.From, "")

	testutil.Equal(t, cfg.Storage.Enabled, false)
	testutil.Equal(t, cfg.Storage.Backend, "local")
	testutil.Equal(t, cfg.Storage.LocalPath, "./ayb_storage")
	testutil.Equal(t, cfg.Storage.MaxFileSize, "10MB")
	testutil.Equal(t, cfg.Storage.S3Region, "us-east-1")
	testutil.Equal(t, cfg.Storage.S3UseSSL, true)

	testutil.Equal(t, cfg.Database.MigrationsDir, "./migrations")

	testutil.Equal(t, cfg.Logging.Level, "info")
	testutil.Equal(t, cfg.Logging.Format, "json")
}

func TestAddress(t *testing.T) {
	tests := []struct {
		name string
		host string
		port int
		want string
	}{
		{name: "default", host: "0.0.0.0", port: 8090, want: "0.0.0.0:8090"},
		{name: "localhost", host: "127.0.0.1", port: 3000, want: "127.0.0.1:3000"},
		{name: "custom host", host: "myserver.local", port: 443, want: "myserver.local:443"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Server: ServerConfig{Host: tt.host, Port: tt.port}}
			testutil.Equal(t, cfg.Address(), tt.want)
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr string
	}{
		{
			name:   "valid defaults",
			modify: func(c *Config) {},
		},
		{
			name:    "port zero",
			modify:  func(c *Config) { c.Server.Port = 0 },
			wantErr: "server.port must be between 1 and 65535",
		},
		{
			name:    "port negative",
			modify:  func(c *Config) { c.Server.Port = -1 },
			wantErr: "server.port must be between 1 and 65535",
		},
		{
			name:    "port too high",
			modify:  func(c *Config) { c.Server.Port = 70000 },
			wantErr: "server.port must be between 1 and 65535",
		},
		{
			name:   "port 1 valid",
			modify: func(c *Config) { c.Server.Port = 1 },
		},
		{
			name:   "port 65535 valid",
			modify: func(c *Config) { c.Server.Port = 65535 },
		},
		{
			name:    "max_conns zero",
			modify:  func(c *Config) { c.Database.MaxConns = 0 },
			wantErr: "database.max_conns must be at least 1",
		},
		{
			name:    "min_conns negative",
			modify:  func(c *Config) { c.Database.MinConns = -1 },
			wantErr: "database.min_conns must be non-negative",
		},
		{
			name: "min_conns exceeds max_conns",
			modify: func(c *Config) {
				c.Database.MaxConns = 5
				c.Database.MinConns = 10
			},
			wantErr: "database.min_conns (10) cannot exceed database.max_conns (5)",
		},
		{
			name:   "min_conns equals max_conns",
			modify: func(c *Config) { c.Database.MinConns = 25 },
		},
		{
			name:    "invalid log level",
			modify:  func(c *Config) { c.Logging.Level = "trace" },
			wantErr: `logging.level must be one of`,
		},
		{
			name:   "debug log level",
			modify: func(c *Config) { c.Logging.Level = "debug" },
		},
		{
			name:   "warn log level",
			modify: func(c *Config) { c.Logging.Level = "warn" },
		},
		{
			name:   "error log level",
			modify: func(c *Config) { c.Logging.Level = "error" },
		},
		{
			name: "auth enabled without secret",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = ""
			},
			wantErr: "auth.jwt_secret is required when auth is enabled",
		},
		{
			name: "auth secret too short",
			modify: func(c *Config) {
				c.Auth.JWTSecret = "tooshort"
			},
			wantErr: "auth.jwt_secret must be at least 32 characters",
		},
		{
			name: "auth enabled with valid secret",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
			},
		},
		{
			name:   "auth disabled without secret is fine",
			modify: func(c *Config) { c.Auth.Enabled = false },
		},
		{
			name: "oauth enabled without auth enabled",
			modify: func(c *Config) {
				c.Auth.Enabled = false
				c.Auth.OAuth = map[string]OAuthProvider{
					"google": {Enabled: true, ClientID: "id", ClientSecret: "secret"},
				}
			},
			wantErr: "auth.enabled must be true to use OAuth provider",
		},
		{
			name: "oauth enabled without client_id",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.OAuth = map[string]OAuthProvider{
					"google": {Enabled: true, ClientID: "", ClientSecret: "secret"},
				}
			},
			wantErr: "client_id is required",
		},
		{
			name: "oauth enabled without client_secret",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.OAuth = map[string]OAuthProvider{
					"github": {Enabled: true, ClientID: "id", ClientSecret: ""},
				}
			},
			wantErr: "client_secret is required",
		},
		{
			name: "unsupported oauth provider",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.OAuth = map[string]OAuthProvider{
					"twitter": {Enabled: true, ClientID: "id", ClientSecret: "secret"},
				}
			},
			wantErr: "unsupported OAuth provider",
		},
		{
			name: "valid oauth config",
			modify: func(c *Config) {
				c.Auth.Enabled = true
				c.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
				c.Auth.OAuth = map[string]OAuthProvider{
					"google": {Enabled: true, ClientID: "id", ClientSecret: "secret"},
					"github": {Enabled: true, ClientID: "id2", ClientSecret: "secret2"},
				}
			},
		},
		{
			name: "disabled oauth provider doesn't need credentials",
			modify: func(c *Config) {
				c.Auth.OAuth = map[string]OAuthProvider{
					"google": {Enabled: false},
				}
			},
		},
		{
			name:   "email log backend valid",
			modify: func(c *Config) { c.Email.Backend = "log" },
		},
		{
			name:   "email empty backend valid (defaults to log)",
			modify: func(c *Config) { c.Email.Backend = "" },
		},
		{
			name: "email smtp valid",
			modify: func(c *Config) {
				c.Email.Backend = "smtp"
				c.Email.SMTP.Host = "smtp.resend.com"
				c.Email.From = "noreply@example.com"
			},
		},
		{
			name: "email smtp missing host",
			modify: func(c *Config) {
				c.Email.Backend = "smtp"
				c.Email.From = "noreply@example.com"
			},
			wantErr: "email.smtp.host is required",
		},
		{
			name: "email smtp missing from",
			modify: func(c *Config) {
				c.Email.Backend = "smtp"
				c.Email.SMTP.Host = "smtp.resend.com"
			},
			wantErr: "email.from is required",
		},
		{
			name: "email webhook valid",
			modify: func(c *Config) {
				c.Email.Backend = "webhook"
				c.Email.Webhook.URL = "https://example.com/webhook"
			},
		},
		{
			name: "email webhook missing url",
			modify: func(c *Config) {
				c.Email.Backend = "webhook"
			},
			wantErr: "email.webhook.url is required",
		},
		{
			name:    "email invalid backend",
			modify:  func(c *Config) { c.Email.Backend = "sendgrid" },
			wantErr: `email.backend must be "log", "smtp", or "webhook"`,
		},
		{
			name: "storage enabled with local backend",
			modify: func(c *Config) {
				c.Storage.Enabled = true
				c.Storage.Backend = "local"
				c.Storage.LocalPath = "/tmp/storage"
			},
		},
		{
			name:    "storage enabled with empty local path",
			modify: func(c *Config) {
				c.Storage.Enabled = true
				c.Storage.Backend = "local"
				c.Storage.LocalPath = ""
			},
			wantErr: "storage.local_path is required",
		},
		{
			name: "storage s3 backend valid",
			modify: func(c *Config) {
				c.Storage.Enabled = true
				c.Storage.Backend = "s3"
				c.Storage.S3Endpoint = "s3.amazonaws.com"
				c.Storage.S3Bucket = "my-bucket"
				c.Storage.S3AccessKey = "AKID"
				c.Storage.S3SecretKey = "secret"
			},
		},
		{
			name: "storage s3 missing endpoint",
			modify: func(c *Config) {
				c.Storage.Enabled = true
				c.Storage.Backend = "s3"
				c.Storage.S3Bucket = "my-bucket"
				c.Storage.S3AccessKey = "AKID"
				c.Storage.S3SecretKey = "secret"
			},
			wantErr: "s3_endpoint is required",
		},
		{
			name: "storage s3 missing bucket",
			modify: func(c *Config) {
				c.Storage.Enabled = true
				c.Storage.Backend = "s3"
				c.Storage.S3Endpoint = "s3.amazonaws.com"
				c.Storage.S3AccessKey = "AKID"
				c.Storage.S3SecretKey = "secret"
			},
			wantErr: "s3_bucket is required",
		},
		{
			name: "storage s3 missing access key",
			modify: func(c *Config) {
				c.Storage.Enabled = true
				c.Storage.Backend = "s3"
				c.Storage.S3Endpoint = "s3.amazonaws.com"
				c.Storage.S3Bucket = "my-bucket"
				c.Storage.S3SecretKey = "secret"
			},
			wantErr: "s3_access_key is required",
		},
		{
			name: "storage s3 missing secret key",
			modify: func(c *Config) {
				c.Storage.Enabled = true
				c.Storage.Backend = "s3"
				c.Storage.S3Endpoint = "s3.amazonaws.com"
				c.Storage.S3Bucket = "my-bucket"
				c.Storage.S3AccessKey = "AKID"
			},
			wantErr: "s3_secret_key is required",
		},
		{
			name:    "storage unsupported backend",
			modify: func(c *Config) {
				c.Storage.Enabled = true
				c.Storage.Backend = "gcs"
			},
			wantErr: "storage.backend must be",
		},
		{
			name:   "storage disabled ignores validation",
			modify: func(c *Config) { c.Storage.Enabled = false },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.modify(cfg)
			err := cfg.Validate()
			if tt.wantErr == "" {
				testutil.NoError(t, err)
			} else {
				testutil.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ayb.toml")

	content := `
[server]
host = "127.0.0.1"
port = 3000

[database]
url = "postgresql://localhost/mydb"
max_conns = 10

[logging]
level = "debug"
format = "text"
`
	err := os.WriteFile(tomlPath, []byte(content), 0o644)
	testutil.NoError(t, err)

	cfg, err := Load(tomlPath, nil)
	testutil.NoError(t, err)

	testutil.Equal(t, cfg.Server.Host, "127.0.0.1")
	testutil.Equal(t, cfg.Server.Port, 3000)
	testutil.Equal(t, cfg.Database.URL, "postgresql://localhost/mydb")
	testutil.Equal(t, cfg.Database.MaxConns, 10)
	testutil.Equal(t, cfg.Logging.Level, "debug")
	testutil.Equal(t, cfg.Logging.Format, "text")

	// Defaults preserved for unset fields.
	testutil.Equal(t, cfg.Database.MinConns, 2)
	testutil.Equal(t, cfg.Admin.Enabled, true)
}

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	// Point to a non-existent file — should silently use defaults.
	cfg, err := Load("/nonexistent/ayb.toml", nil)
	testutil.NoError(t, err)
	testutil.Equal(t, cfg.Server.Port, 8090)
	testutil.Equal(t, cfg.Server.Host, "0.0.0.0")
}

func TestLoadInvalidTOML(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ayb.toml")
	err := os.WriteFile(tomlPath, []byte("this is not valid toml [[["), 0o644)
	testutil.NoError(t, err)

	_, err = Load(tomlPath, nil)
	testutil.ErrorContains(t, err, "parsing")
}

func TestLoadEnvOverrides(t *testing.T) {
	// Set env vars, then clean up.
	t.Setenv("AYB_SERVER_HOST", "envhost")
	t.Setenv("AYB_SERVER_PORT", "9999")
	t.Setenv("AYB_DATABASE_URL", "postgresql://envdb")
	t.Setenv("AYB_ADMIN_PASSWORD", "secret123")
	t.Setenv("AYB_LOG_LEVEL", "warn")
	t.Setenv("AYB_CORS_ORIGINS", "http://a.com,http://b.com")
	t.Setenv("AYB_AUTH_ENABLED", "true")
	t.Setenv("AYB_AUTH_JWT_SECRET", "this-is-a-secret-that-is-at-least-32-characters-long")

	cfg, err := Load("/nonexistent/ayb.toml", nil)
	testutil.NoError(t, err)

	testutil.Equal(t, cfg.Server.Host, "envhost")
	testutil.Equal(t, cfg.Server.Port, 9999)
	testutil.Equal(t, cfg.Database.URL, "postgresql://envdb")
	testutil.Equal(t, cfg.Admin.Password, "secret123")
	testutil.Equal(t, cfg.Logging.Level, "warn")
	testutil.SliceLen(t, cfg.Server.CORSAllowedOrigins, 2)
	testutil.Equal(t, cfg.Server.CORSAllowedOrigins[0], "http://a.com")
	testutil.Equal(t, cfg.Server.CORSAllowedOrigins[1], "http://b.com")
	testutil.Equal(t, cfg.Auth.Enabled, true)
	testutil.Equal(t, cfg.Auth.JWTSecret, "this-is-a-secret-that-is-at-least-32-characters-long")
}

func TestLoadFlagOverrides(t *testing.T) {
	flags := map[string]string{
		"database-url": "postgresql://flagdb",
		"port":         "7777",
		"host":         "flaghost",
	}

	cfg, err := Load("/nonexistent/ayb.toml", flags)
	testutil.NoError(t, err)

	testutil.Equal(t, cfg.Database.URL, "postgresql://flagdb")
	testutil.Equal(t, cfg.Server.Port, 7777)
	testutil.Equal(t, cfg.Server.Host, "flaghost")
}

func TestLoadPriority(t *testing.T) {
	// File sets port=3000, env sets port=4000, flag sets port=5000.
	// Expected priority: flag > env > file > default.
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ayb.toml")
	err := os.WriteFile(tomlPath, []byte("[server]\nport = 3000\n"), 0o644)
	testutil.NoError(t, err)

	t.Setenv("AYB_SERVER_PORT", "4000")
	flags := map[string]string{"port": "5000"}

	cfg, err := Load(tomlPath, flags)
	testutil.NoError(t, err)
	testutil.Equal(t, cfg.Server.Port, 5000)

	// Without flag, env wins over file.
	cfg, err = Load(tomlPath, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, cfg.Server.Port, 4000)
}

func TestLoadEnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ayb.toml")
	err := os.WriteFile(tomlPath, []byte("[server]\nhost = \"filehost\"\n"), 0o644)
	testutil.NoError(t, err)

	t.Setenv("AYB_SERVER_HOST", "envhost")

	cfg, err := Load(tomlPath, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, cfg.Server.Host, "envhost")
}

func TestGenerateDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "ayb.toml")

	err := GenerateDefault(path)
	testutil.NoError(t, err)

	data, err := os.ReadFile(path)
	testutil.NoError(t, err)
	content := string(data)

	testutil.Contains(t, content, "[server]")
	testutil.Contains(t, content, "[database]")
	testutil.Contains(t, content, "[admin]")
	testutil.Contains(t, content, "[auth]")
	testutil.Contains(t, content, "[email]")
	testutil.Contains(t, content, "[storage]")
	testutil.Contains(t, content, "[logging]")
	testutil.Contains(t, content, "port = 8090")
	testutil.Contains(t, content, "token_duration = 900")
	testutil.Contains(t, content, "refresh_token_duration = 604800")
}

func TestToTOML(t *testing.T) {
	cfg := Default()
	s, err := cfg.ToTOML()
	testutil.NoError(t, err)
	testutil.Contains(t, s, "host = '0.0.0.0'")
	testutil.Contains(t, s, "port = 8090")
}

func TestApplyFlagsNilSafe(t *testing.T) {
	cfg := Default()
	// Should not panic with nil flags.
	applyFlags(cfg, nil)
	testutil.Equal(t, cfg.Server.Port, 8090)
}

func TestApplyFlagsEmptyValues(t *testing.T) {
	cfg := Default()
	flags := map[string]string{
		"database-url": "",
		"port":         "",
		"host":         "",
	}
	applyFlags(cfg, flags)
	// Empty values should not override defaults.
	testutil.Equal(t, cfg.Server.Host, "0.0.0.0")
	testutil.Equal(t, cfg.Server.Port, 8090)
}

func TestApplyEnvInvalidPort(t *testing.T) {
	t.Setenv("AYB_SERVER_PORT", "notanumber")
	cfg := Default()
	applyEnv(cfg)
	// Invalid port should be silently ignored, keeping the default.
	testutil.Equal(t, cfg.Server.Port, 8090)
}

func TestStorageMaxFileSizeBytes(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"10MB", 10 << 20},
		{"5MB", 5 << 20},
		{"1MB", 1 << 20},
		{"", 10 << 20},       // default
		{"invalid", 10 << 20}, // default on parse failure
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			cfg := &StorageConfig{MaxFileSize: tt.input}
			testutil.Equal(t, cfg.MaxFileSizeBytes(), tt.want)
		})
	}
}

func TestApplyStorageEnvVars(t *testing.T) {
	t.Setenv("AYB_STORAGE_ENABLED", "true")
	t.Setenv("AYB_STORAGE_BACKEND", "local")
	t.Setenv("AYB_STORAGE_LOCAL_PATH", "/tmp/custom")
	t.Setenv("AYB_STORAGE_MAX_FILE_SIZE", "50MB")

	cfg := Default()
	applyEnv(cfg)

	testutil.Equal(t, cfg.Storage.Enabled, true)
	testutil.Equal(t, cfg.Storage.Backend, "local")
	testutil.Equal(t, cfg.Storage.LocalPath, "/tmp/custom")
	testutil.Equal(t, cfg.Storage.MaxFileSize, "50MB")
}

func TestApplyS3StorageEnvVars(t *testing.T) {
	t.Setenv("AYB_STORAGE_S3_ENDPOINT", "s3.amazonaws.com")
	t.Setenv("AYB_STORAGE_S3_BUCKET", "test-bucket")
	t.Setenv("AYB_STORAGE_S3_REGION", "eu-west-1")
	t.Setenv("AYB_STORAGE_S3_ACCESS_KEY", "AKID123")
	t.Setenv("AYB_STORAGE_S3_SECRET_KEY", "secret456")
	t.Setenv("AYB_STORAGE_S3_USE_SSL", "false")

	cfg := Default()
	applyEnv(cfg)

	testutil.Equal(t, cfg.Storage.S3Endpoint, "s3.amazonaws.com")
	testutil.Equal(t, cfg.Storage.S3Bucket, "test-bucket")
	testutil.Equal(t, cfg.Storage.S3Region, "eu-west-1")
	testutil.Equal(t, cfg.Storage.S3AccessKey, "AKID123")
	testutil.Equal(t, cfg.Storage.S3SecretKey, "secret456")
	testutil.Equal(t, cfg.Storage.S3UseSSL, false)
}

func TestValidateEmbeddedPort(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		port    int
		wantErr string
	}{
		{"valid default port, no URL", "", 15432, ""},
		{"valid custom port, no URL", "", 9999, ""},
		{"invalid port zero, no URL", "", 0, "database.embedded_port must be between 1 and 65535"},
		{"invalid port too high, no URL", "", 99999, "database.embedded_port must be between 1 and 65535"},
		{"invalid port ignored when URL set", "postgresql://localhost/db", 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			cfg.Database.URL = tt.url
			cfg.Database.EmbeddedPort = tt.port
			err := cfg.Validate()
			if tt.wantErr == "" {
				testutil.NoError(t, err)
			} else {
				testutil.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestApplyEmbeddedEnvVars(t *testing.T) {
	t.Setenv("AYB_DATABASE_EMBEDDED_PORT", "19999")
	t.Setenv("AYB_DATABASE_EMBEDDED_DATA_DIR", "/custom/data")

	cfg := Default()
	applyEnv(cfg)

	testutil.Equal(t, cfg.Database.EmbeddedPort, 19999)
	testutil.Equal(t, cfg.Database.EmbeddedDataDir, "/custom/data")
}

func TestApplyEmbeddedPortInvalidEnv(t *testing.T) {
	t.Setenv("AYB_DATABASE_EMBEDDED_PORT", "notanumber")
	cfg := Default()
	applyEnv(cfg)
	testutil.Equal(t, cfg.Database.EmbeddedPort, 15432) // unchanged
}

func TestGenerateDefaultContainsEmbedded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ayb.toml")
	err := GenerateDefault(path)
	testutil.NoError(t, err)

	data, err := os.ReadFile(path)
	testutil.NoError(t, err)
	testutil.Contains(t, string(data), "embedded_port")
	testutil.Contains(t, string(data), "embedded_data_dir")
}

func TestApplyOAuthEnvVars(t *testing.T) {
	t.Setenv("AYB_AUTH_OAUTH_GOOGLE_CLIENT_ID", "env-google-id")
	t.Setenv("AYB_AUTH_OAUTH_GOOGLE_CLIENT_SECRET", "env-google-secret")
	t.Setenv("AYB_AUTH_OAUTH_GOOGLE_ENABLED", "true")
	t.Setenv("AYB_AUTH_OAUTH_GITHUB_CLIENT_ID", "env-github-id")
	t.Setenv("AYB_AUTH_OAUTH_REDIRECT_URL", "http://myapp.com/callback")

	cfg := Default()
	applyEnv(cfg)

	testutil.Equal(t, cfg.Auth.OAuthRedirectURL, "http://myapp.com/callback")
	testutil.NotNil(t, cfg.Auth.OAuth)

	g := cfg.Auth.OAuth["google"]
	testutil.Equal(t, g.ClientID, "env-google-id")
	testutil.Equal(t, g.ClientSecret, "env-google-secret")
	testutil.True(t, g.Enabled, "google should be enabled")

	gh := cfg.Auth.OAuth["github"]
	testutil.Equal(t, gh.ClientID, "env-github-id")
	testutil.False(t, gh.Enabled, "github should not be enabled (no ENABLED env)")
}

func TestApplyEmailEnvVars(t *testing.T) {
	t.Setenv("AYB_EMAIL_BACKEND", "smtp")
	t.Setenv("AYB_EMAIL_FROM", "noreply@example.com")
	t.Setenv("AYB_EMAIL_FROM_NAME", "MyApp")
	t.Setenv("AYB_EMAIL_SMTP_HOST", "smtp.resend.com")
	t.Setenv("AYB_EMAIL_SMTP_PORT", "465")
	t.Setenv("AYB_EMAIL_SMTP_USERNAME", "apikey")
	t.Setenv("AYB_EMAIL_SMTP_PASSWORD", "re_secret")
	t.Setenv("AYB_EMAIL_SMTP_AUTH_METHOD", "LOGIN")
	t.Setenv("AYB_EMAIL_SMTP_TLS", "true")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)

	testutil.Equal(t, cfg.Email.Backend, "smtp")
	testutil.Equal(t, cfg.Email.From, "noreply@example.com")
	testutil.Equal(t, cfg.Email.FromName, "MyApp")
	testutil.Equal(t, cfg.Email.SMTP.Host, "smtp.resend.com")
	testutil.Equal(t, cfg.Email.SMTP.Port, 465)
	testutil.Equal(t, cfg.Email.SMTP.Username, "apikey")
	testutil.Equal(t, cfg.Email.SMTP.Password, "re_secret")
	testutil.Equal(t, cfg.Email.SMTP.AuthMethod, "LOGIN")
	testutil.Equal(t, cfg.Email.SMTP.TLS, true)
}

func TestApplyEmailWebhookEnvVars(t *testing.T) {
	t.Setenv("AYB_EMAIL_BACKEND", "webhook")
	t.Setenv("AYB_EMAIL_WEBHOOK_URL", "https://hooks.example.com/email")
	t.Setenv("AYB_EMAIL_WEBHOOK_SECRET", "whsec_abc123")
	t.Setenv("AYB_EMAIL_WEBHOOK_TIMEOUT", "30")

	cfg := Default()
	err := applyEnv(cfg)
	testutil.NoError(t, err)

	testutil.Equal(t, cfg.Email.Backend, "webhook")
	testutil.Equal(t, cfg.Email.Webhook.URL, "https://hooks.example.com/email")
	testutil.Equal(t, cfg.Email.Webhook.Secret, "whsec_abc123")
	testutil.Equal(t, cfg.Email.Webhook.Timeout, 30)
}
