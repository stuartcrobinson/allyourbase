# Getting Started

Get AYB running and make your first API call in under 5 minutes.

## Install

### curl (macOS / Linux)

```bash
curl -fsSL https://allyourbase.io/install.sh | sh
```

### Homebrew

```bash
brew install allyourbase/tap/ayb
```

### Go

```bash
go install github.com/allyourbase/ayb/cmd/ayb@latest
```

### Binary download

Download the latest release from [GitHub Releases](https://github.com/stuartcrobinson/allyourbase/releases) for your OS and architecture.

### Docker

```bash
docker run -p 8090:8090 ghcr.io/stuartcrobinson/allyourbase
```

## Start the server

### Embedded PostgreSQL (zero config)

```bash
ayb start
```

AYB will download and manage its own PostgreSQL instance automatically. No database setup needed.

### External PostgreSQL

```bash
ayb start --database-url postgresql://user:pass@localhost:5432/mydb
```

The API is now live at `http://localhost:8090/api` and the admin dashboard at `http://localhost:8090/admin`.

## Create a table

Create a `posts` table in your PostgreSQL database:

```sql
CREATE TABLE posts (
  id SERIAL PRIMARY KEY,
  title TEXT NOT NULL,
  body TEXT,
  published BOOLEAN DEFAULT false,
  created_at TIMESTAMPTZ DEFAULT now()
);
```

You can run this via `psql`, the admin dashboard SQL editor, or any PostgreSQL client.

## Make your first API call

### List records

```bash
curl http://localhost:8090/api/collections/posts
```

### Create a record

```bash
curl -X POST http://localhost:8090/api/collections/posts \
  -H "Content-Type: application/json" \
  -d '{"title": "Hello World", "body": "My first post", "published": true}'
```

### Filter and sort

```bash
curl "http://localhost:8090/api/collections/posts?filter=published=true&sort=-created_at"
```

### Get a single record

```bash
curl http://localhost:8090/api/collections/posts/1
```

## Use the JavaScript SDK

```bash
npm install @allyourbase/js
```

```ts
import { AYBClient } from "@allyourbase/js";

const ayb = new AYBClient("http://localhost:8090");

// Create
await ayb.records.create("posts", { title: "Hello", published: true });

// List
const { items } = await ayb.records.list("posts", {
  filter: "published=true",
  sort: "-created_at",
});
console.log(items);
```

## Next steps

- [Configuration](/guide/configuration) — Customize AYB with `ayb.toml`
- [REST API Reference](/guide/api-reference) — Full endpoint documentation
- [Authentication](/guide/authentication) — Add user auth and RLS
- [Quickstart: Todo App](/guide/quickstart) — Build a full app in 5 minutes
