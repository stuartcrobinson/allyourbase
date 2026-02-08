# Configuration

AYB uses a layered configuration system: **defaults &rarr; ayb.toml &rarr; environment variables &rarr; CLI flags**.

## Config file

Create `ayb.toml` in the working directory:

```toml
[server]
host = "0.0.0.0"
port = 8090
cors_allowed_origins = ["*"]
body_limit = "1MB"
shutdown_timeout = 10

[database]
url = "postgresql://user:pass@localhost:5432/mydb?sslmode=disable"
max_conns = 25
min_conns = 2
health_check_interval = 30
migrations_dir = "./migrations"
# Embedded PostgreSQL (used when url is empty):
# embedded_port = 15432
# embedded_data_dir = ""

[admin]
enabled = true
path = "/admin"
# password = "your-admin-password"

[auth]
enabled = false
# jwt_secret = ""           # Required when enabled, min 32 chars
token_duration = 900         # 15 minutes
refresh_token_duration = 604800  # 7 days
# oauth_redirect_url = "http://localhost:5173/oauth-callback"

# [auth.oauth.google]
# enabled = true
# client_id = ""
# client_secret = ""

# [auth.oauth.github]
# enabled = true
# client_id = ""
# client_secret = ""

[email]
backend = "log"              # "log", "smtp", or "webhook"
# from = "noreply@example.com"
from_name = "AllYourBase"

# [email.smtp]
# host = "smtp.resend.com"
# port = 465
# username = ""
# password = ""
# auth_method = "PLAIN"
# tls = true

# [email.webhook]
# url = "https://your-app.com/email-hook"
# secret = "hmac-signing-secret"
# timeout = 10

[storage]
enabled = false
backend = "local"            # "local" or "s3"
local_path = "./ayb_storage"
max_file_size = "10MB"

# S3-compatible (AWS S3, Cloudflare R2, MinIO, DigitalOcean Spaces):
# s3_endpoint = "s3.amazonaws.com"
# s3_bucket = "my-ayb-bucket"
# s3_region = "us-east-1"
# s3_access_key = ""
# s3_secret_key = ""
# s3_use_ssl = true

[logging]
level = "info"               # debug, info, warn, error
format = "json"              # json or text
```

## Environment variables

Every config value can be overridden with `AYB_` prefixed environment variables:

| Variable | Config field |
|----------|-------------|
| `AYB_SERVER_HOST` | `server.host` |
| `AYB_SERVER_PORT` | `server.port` |
| `AYB_DATABASE_URL` | `database.url` |
| `AYB_DATABASE_EMBEDDED_PORT` | `database.embedded_port` |
| `AYB_DATABASE_EMBEDDED_DATA_DIR` | `database.embedded_data_dir` |
| `AYB_DATABASE_MIGRATIONS_DIR` | `database.migrations_dir` |
| `AYB_ADMIN_PASSWORD` | `admin.password` |
| `AYB_AUTH_ENABLED` | `auth.enabled` |
| `AYB_AUTH_JWT_SECRET` | `auth.jwt_secret` |
| `AYB_AUTH_REFRESH_TOKEN_DURATION` | `auth.refresh_token_duration` |
| `AYB_AUTH_OAUTH_REDIRECT_URL` | `auth.oauth_redirect_url` |
| `AYB_AUTH_OAUTH_GOOGLE_CLIENT_ID` | `auth.oauth.google.client_id` |
| `AYB_AUTH_OAUTH_GOOGLE_CLIENT_SECRET` | `auth.oauth.google.client_secret` |
| `AYB_AUTH_OAUTH_GOOGLE_ENABLED` | `auth.oauth.google.enabled` |
| `AYB_AUTH_OAUTH_GITHUB_CLIENT_ID` | `auth.oauth.github.client_id` |
| `AYB_AUTH_OAUTH_GITHUB_CLIENT_SECRET` | `auth.oauth.github.client_secret` |
| `AYB_AUTH_OAUTH_GITHUB_ENABLED` | `auth.oauth.github.enabled` |
| `AYB_EMAIL_BACKEND` | `email.backend` |
| `AYB_EMAIL_FROM` | `email.from` |
| `AYB_EMAIL_FROM_NAME` | `email.from_name` |
| `AYB_EMAIL_SMTP_HOST` | `email.smtp.host` |
| `AYB_EMAIL_SMTP_PORT` | `email.smtp.port` |
| `AYB_EMAIL_SMTP_USERNAME` | `email.smtp.username` |
| `AYB_EMAIL_SMTP_PASSWORD` | `email.smtp.password` |
| `AYB_EMAIL_SMTP_AUTH_METHOD` | `email.smtp.auth_method` |
| `AYB_EMAIL_SMTP_TLS` | `email.smtp.tls` |
| `AYB_EMAIL_WEBHOOK_URL` | `email.webhook.url` |
| `AYB_EMAIL_WEBHOOK_SECRET` | `email.webhook.secret` |
| `AYB_EMAIL_WEBHOOK_TIMEOUT` | `email.webhook.timeout` |
| `AYB_STORAGE_ENABLED` | `storage.enabled` |
| `AYB_STORAGE_BACKEND` | `storage.backend` |
| `AYB_STORAGE_LOCAL_PATH` | `storage.local_path` |
| `AYB_STORAGE_MAX_FILE_SIZE` | `storage.max_file_size` |
| `AYB_STORAGE_S3_ENDPOINT` | `storage.s3_endpoint` |
| `AYB_STORAGE_S3_BUCKET` | `storage.s3_bucket` |
| `AYB_STORAGE_S3_REGION` | `storage.s3_region` |
| `AYB_STORAGE_S3_ACCESS_KEY` | `storage.s3_access_key` |
| `AYB_STORAGE_S3_SECRET_KEY` | `storage.s3_secret_key` |
| `AYB_STORAGE_S3_USE_SSL` | `storage.s3_use_ssl` |
| `AYB_CORS_ORIGINS` | `server.cors_allowed_origins` (comma-separated) |
| `AYB_LOG_LEVEL` | `logging.level` |

## CLI flags

```bash
ayb start --database-url URL --port 3000 --host 127.0.0.1
```

CLI flags override everything else.

## CLI commands

```
ayb start      [--database-url] [--port] [--host]   Start the server
ayb stop                                             Stop the server
ayb status                                           Show server status
ayb config     [--config path]                       Print resolved config
ayb migrate    [up|down|status]                      Run database migrations
ayb admin      [create-password]                     Admin utilities
ayb version                                          Print version info
```

## Generate a default config

```bash
ayb config > ayb.toml
```

This prints the full default configuration with comments.
