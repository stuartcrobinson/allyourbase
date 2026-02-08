# AllYourBase

**Backend-as-a-Service for PostgreSQL.** Single binary. One config file. Auto-generated REST API + admin dashboard.

AYB connects to your PostgreSQL database, introspects the schema, and gives you a full REST API with filtering, sorting, pagination, and foreign key expansion — plus an admin dashboard to manage your data. No code generation. No ORM. Just point it at Postgres and go.

## Quick Start

```bash
# Download (macOS/Linux)
curl -fsSL https://allyourbase.io/install.sh | sh

# Point at your Postgres and start
ayb start --database-url postgresql://user:pass@localhost:5432/mydb

# That's it. API is live at :8090/api, admin at :8090/admin
```

Or use the embedded PostgreSQL (no external DB needed):

```bash
ayb start   # starts embedded Postgres automatically
```

## What You Get

- **Auto-generated REST API** — CRUD endpoints for every table, with filtering, sorting, pagination, and FK expansion
- **Admin dashboard** — Browse tables, create/edit/delete records, inspect schema
- **Built-in auth** — Email/password registration, JWT sessions, OAuth (Google, GitHub), email verification, password reset
- **File storage** — Upload/download files to local disk or S3-compatible storage
- **Realtime** — Server-Sent Events filtered by table subscriptions and RLS policies
- **Database RPC** — Call PostgreSQL functions via `POST /api/rpc/{function}`
- **Row-Level Security** — JWT claims injected into Postgres session for RLS policy enforcement
- **Email infrastructure** — Log (dev), SMTP, or webhook mailer backends with HTML templates
- **Zero configuration** — Sensible defaults, optional `ayb.toml` for customization
- **Single binary** — No Docker, no containers, no runtime dependencies
- **Embedded PostgreSQL** — Optional built-in Postgres for zero-dependency dev mode

## API

### Collections (auto-generated CRUD)

```
GET    /api/collections/{table}          — list (filter, sort, paginate)
POST   /api/collections/{table}          — create record
GET    /api/collections/{table}/{id}     — read record
PATCH  /api/collections/{table}/{id}     — update record (partial)
DELETE /api/collections/{table}/{id}     — delete record
```

### Query Parameters

| Parameter   | Example | Description |
|-------------|---------|-------------|
| `filter`    | `?filter=status='active' AND age>21` | Filter rows (parameterized, SQL-safe) |
| `sort`      | `?sort=-created,+name` | Sort by fields (- desc, + asc) |
| `page`      | `?page=2&perPage=50` | Pagination (default 20, max 500) |
| `fields`    | `?fields=id,name,email` | Select specific columns |
| `expand`    | `?expand=author,category` | Expand foreign key relationships |
| `skipTotal` | `?skipTotal=true` | Skip COUNT query for faster list responses |

### Authentication

```
POST   /api/auth/register                — create account {email, password}
POST   /api/auth/login                   — authenticate {email, password}
POST   /api/auth/refresh                 — rotate tokens {refreshToken}
POST   /api/auth/logout                  — revoke session {refreshToken}
GET    /api/auth/me                      — get current user (requires auth)
POST   /api/auth/password-reset          — request password reset {email}
POST   /api/auth/password-reset/confirm  — reset password {token, password}
POST   /api/auth/verify                  — verify email {token}
POST   /api/auth/verify/resend           — resend verification (requires auth)
GET    /api/auth/oauth/{provider}        — start OAuth flow (google, github)
GET    /api/auth/oauth/{provider}/callback — OAuth callback
```

### Storage

```
POST   /api/storage/{bucket}             — upload file (multipart/form-data)
GET    /api/storage/{bucket}/{name}      — download file
DELETE /api/storage/{bucket}/{name}      — delete file
GET    /api/storage/{bucket}             — list files in bucket
POST   /api/storage/{bucket}/{name}/sign — get signed URL
```

### Other

```
POST   /api/rpc/{function}              — call a PostgreSQL function
GET    /api/schema                       — full schema as JSON
GET    /api/realtime?tables=t1,t2        — SSE stream (filtered by RLS)
GET    /health                           — health check
```

## Configuration

AYB uses a layered config system: **defaults -> ayb.toml -> environment variables -> CLI flags**.

```toml
# ayb.toml
[server]
host = "0.0.0.0"
port = 8090

[database]
url = "postgresql://user:pass@localhost:5432/mydb"

[admin]
enabled = true
password = "secret"

[auth]
enabled = true
# jwt_secret = "auto-generated if not set"

[email]
backend = "log"       # "log" (dev), "smtp", or "webhook"
# from = "noreply@example.com"

[storage]
enabled = true
backend = "local"     # "local" or "s3"
```

All config values can be overridden with `AYB_` prefixed env vars (e.g. `AYB_DATABASE_URL`, `AYB_AUTH_JWT_SECRET`).

See `ayb config` to print the resolved configuration.

## CLI

```
ayb start      [--database-url] [--port] [--host]   Start the server
ayb stop                                             Stop the server
ayb status                                           Show server status
ayb config     [--config path]                       Print resolved config
ayb migrate    [up|down|status]                      Run database migrations
ayb admin      [create-password]                     Admin utilities
ayb version                                          Print version info
```

## Building from Source

```bash
git clone https://github.com/stuartcrobinson/allyourbase.git
cd allyourbase
make build
./ayb version
```

## Why AllYourBase?

| | PocketBase | Supabase (self-hosted) | AllYourBase |
|---|---|---|---|
| Database | SQLite | PostgreSQL | **PostgreSQL** |
| Deployment | Single binary | 10+ Docker containers | **Single binary** |
| Configuration | One file | Dozens of env vars | **One file** |
| Docker required | No | Yes | **No** |
| RLS support | No | Yes | **Yes** |
| Realtime | Yes | Yes (complex setup) | **Yes (SSE)** |

**No single-binary PostgreSQL BaaS exists as a first-class product.** AllYourBase fills that gap.

## License

[MIT](LICENSE)
