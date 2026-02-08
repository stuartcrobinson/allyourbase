# Comparison

How AllYourBase compares to PocketBase and Supabase (self-hosted).

## Feature matrix

| Feature | PocketBase | Supabase (self-hosted) | AllYourBase |
|---------|-----------|----------------------|-------------|
| **Database** | SQLite | PostgreSQL | PostgreSQL |
| **Deployment** | Single binary | 10+ Docker containers | Single binary |
| **Configuration** | One file | Dozens of env vars | One file (`ayb.toml`) |
| **Docker required** | No | Yes | No |
| **Auto-generated API** | Yes | Yes | Yes |
| **Filtering** | Custom syntax | PostgREST | SQL-like, parameterized |
| **Sorting** | Yes | Yes | Yes |
| **Pagination** | Yes | Yes | Yes |
| **FK expansion** | Yes | Select embedding | Yes (`?expand=`) |
| **Row-Level Security** | No | Yes | Yes |
| **Auth** | Yes | Yes | Yes |
| **OAuth** | Many providers | Many providers | Google, GitHub |
| **File storage** | Yes | Yes | Yes (local + S3) |
| **Realtime** | SSE | WebSocket (complex) | SSE |
| **Admin dashboard** | Yes | Yes (complex) | Yes |
| **Database RPC** | No | Yes (PostgREST) | Yes |
| **Horizontal scaling** | No (SQLite) | Yes | Yes (PostgreSQL) |
| **Bulk operations** | No | Yes | Coming soon |

## When to use AllYourBase

**Choose AllYourBase if you:**
- Want PostgreSQL without the operational complexity of Supabase
- Need a single binary you can deploy anywhere
- Want RLS support that PocketBase doesn't offer
- Need horizontal scaling that SQLite can't provide
- Want an opinionated, batteries-included setup with minimal configuration

## When to use PocketBase

**Choose PocketBase if you:**
- Want the simplest possible setup and SQLite is sufficient
- Don't need Row-Level Security
- Need many OAuth providers out of the box
- Want a mature, battle-tested product

## When to use Supabase

**Choose Supabase if you:**
- Want a managed cloud service (Supabase Cloud)
- Need the full ecosystem (edge functions, branching, SQL editor, API explorer)
- Need advanced PostgREST features (select embedding, computed columns)
- Want a large community and extensive documentation

## Key advantages of AllYourBase

### PostgreSQL without complexity

Supabase self-hosted requires 10+ containers (PostgREST, GoTrue, Realtime, Storage, Kong, etc.). AYB gives you the same PostgreSQL foundation in a single binary.

### Single binary + embedded PostgreSQL

For development and prototyping, `ayb start` downloads and manages its own PostgreSQL — no database setup needed. For production, point it at any PostgreSQL instance.

### SQL-safe filtering

AYB's filter syntax is familiar and parameterized:

```
?filter=status='active' AND age>21
```

No custom DSL to learn. Values are parameterized to prevent SQL injection.
