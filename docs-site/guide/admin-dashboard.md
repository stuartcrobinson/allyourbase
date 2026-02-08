# Admin Dashboard

AYB includes a built-in admin dashboard for browsing tables, managing records, and inspecting your database schema.

## Access

The dashboard is available at `http://localhost:8090/admin` by default.

## Configuration

```toml
[admin]
enabled = true
path = "/admin"
password = "your-admin-password"
```

When `password` is set, the dashboard requires authentication. Without a password, the dashboard is open (suitable for local development only).

## Features

### Table browser

- Sidebar listing all tables in your database
- Paginated data table with sorting
- Click any row to view full record details

### Record management

- **Create** new records with a form auto-generated from the table schema
- **Edit** existing records inline
- **Delete** records with confirmation

### Schema viewer

- View columns, data types, and constraints for each table
- See primary keys, foreign key relationships, and indexes

## Security

For production deployments, always set an admin password:

```bash
AYB_ADMIN_PASSWORD=your-secure-password ayb start
```

Or generate a password hash:

```bash
ayb admin create-password
```

::: warning
Never expose the admin dashboard without a password on a public network.
:::
