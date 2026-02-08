# Testing Guide — AllYourBase (AYB)

Reference document for testing conventions and patterns. All tests should be fast by default.

## Quick Commands

```bash
# Unit tests only (no DB, sub-second)
make test

# Integration tests — shared container, ~11s
make test-integration

# Both
make test-all

# Single package
go test ./internal/config/...

# Single test
go test ./internal/schema/ -run TestPgTypeToJSON

# Verbose
go test -v ./internal/config/...
```

## Project Layout

```
internal/
  config/
    config.go
    config_test.go          # unit tests (no DB)
  schema/
    schema.go
    schema_test.go          # unit tests (no DB)
    typemap.go
    typemap_test.go         # unit tests (no DB)
    introspect.go
    introspect_test.go      # unit tests for pure functions + integration for DB queries
    cache_test.go           # integration (needs DB)
    watcher_test.go         # integration (needs DB)
  postgres/
    pool_test.go            # integration (needs DB)
  migrations/
    runner_test.go          # integration (needs DB)
  server/
    server_test.go          # unit tests (httptest, no real server)
    middleware_test.go       # unit tests (httptest)
  testutil/
    testutil.go             # assertion helpers
    pgcontainer.go          # TEST_DATABASE_URL helper (build tag: integration)
```

## Two Test Tiers

### Unit Tests (default, no build tag)

- Run with plain `go test ./...`
- **Zero external dependencies** — no DB, no Docker, no network
- Sub-second execution for the entire suite
- Test pure functions, validation logic, HTTP handlers (via `httptest`)
- Place `_test.go` next to the code it tests

### Integration Tests (`//go:build integration`)

- Run with `make test-integration` — starts one shared Postgres container, each package gets a temp database (~11s)
- Or set `TEST_DATABASE_URL` manually to point at any running Postgres
- Test real DB queries, migrations, schema introspection, LISTEN/NOTIFY
- All integration test files start with `//go:build integration`

## Conventions

### Table-Driven Tests

Use table-driven tests with `t.Run()` as the default pattern:

```go
func TestValidate(t *testing.T) {
    tests := []struct {
        name    string
        cfg     Config
        wantErr string // empty means no error expected
    }{
        {
            name: "valid config",
            cfg:  *Default(),
        },
        {
            name:    "port too high",
            cfg:     Config{Server: ServerConfig{Port: 99999}},
            wantErr: "server.port must be between",
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := tt.cfg.Validate()
            if tt.wantErr == "" {
                testutil.NoError(t, err)
            } else {
                testutil.ErrorContains(t, err, tt.wantErr)
            }
        })
    }
}
```

### Assertion Helpers

Use `internal/testutil` instead of testify. Keeps deps minimal and tests fast:

```go
import "github.com/allyourbase/ayb/internal/testutil"

testutil.Equal(t, got, want)
testutil.NoError(t, err)
testutil.ErrorContains(t, err, "substring")
testutil.True(t, condition, "message")
testutil.Nil(t, val)
testutil.NotNil(t, val)
```

All helpers call `t.Helper()` so failures report the caller's line number.

### Parallel Tests

- **DO** use `t.Parallel()` for integration tests with I/O waits
- **DO NOT** use `t.Parallel()` for fast unit tests — goroutine overhead makes them slower
- When using parallel + table-driven, capture the loop variable:
  ```go
  for _, tt := range tests {
      tt := tt // capture
      t.Run(tt.name, func(t *testing.T) {
          t.Parallel()
          // ...
      })
  }
  ```

### Test Packages

- Use `package foo_test` (external/black-box) when testing only exported API
- Use `package foo` (same package) when you need to test unexported functions
- Prefer external test packages when possible

### No Mocking Frameworks

- Use interfaces + hand-written fakes
- Keep interfaces small (1-3 methods) and define at consumer site
- For `*slog.Logger`, use `testutil.DiscardLogger()` (centralized helper, not a local copy)

### Temp Files

Use `t.TempDir()` — auto-cleaned, no manual teardown:

```go
func TestGenerateDefault(t *testing.T) {
    path := filepath.Join(t.TempDir(), "ayb.toml")
    err := GenerateDefault(path)
    testutil.NoError(t, err)
}
```

### Integration Test Helper

All integration test packages use a **shared Postgres via `TestMain`**. `TEST_DATABASE_URL` must be set (use `make test-integration`). Each package creates a temporary database for isolation.

```go
//go:build integration

var sharedPG *testutil.PGContainer

func TestMain(m *testing.M) {
    ctx := context.Background()
    pg, cleanup := testutil.StartPostgresForTestMain(ctx)
    sharedPG = pg
    code := m.Run()
    cleanup()
    os.Exit(code)
}

func resetDB(t *testing.T, ctx context.Context) {
    t.Helper()
    _, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
    if err != nil {
        t.Fatalf("resetting schema: %v", err)
    }
}
```

`make test-integration` handles the shared container automatically. For ad-hoc use, point `TEST_DATABASE_URL` at any running Postgres.

## Speed Guidelines

1. **No I/O in unit tests.** Use `httptest`, `bytes.Buffer`, in-memory fakes.
2. **Leverage test caching.** Don't use `-count=1` unless debugging flaky tests.
3. **Use `make test-integration`.** One shared container, temp database per package. ~11s total.
4. **Keep unit tests under 100ms total per package.** If they're slower, something is wrong.
5. **Gate slow tests.** Anything over 1s belongs behind `//go:build integration`.
6. **Integration suite target: <15s.** Currently ~11s with shared container.

## CI Pipeline

```yaml
# Fast gate (runs on every push, blocks merge)
- go test ./...           # unit tests, <2s
- go vet ./...
- golangci-lint run

# Thorough (runs in parallel with fast gate)
- go test -race ./...                       # race detector
- make test-integration                     # integration tests, ~12s
```

## Race Detector

- Always run `-race` in CI (separate job so it doesn't block fast feedback)
- Never run `-race` during local development by default (2-20x slowdown)
- Fix race conditions immediately — they cause real bugs

## Adding New Tests

When adding a new feature (e.g., Section 5 REST API):

1. Write tests alongside the feature, not after
2. Unit-test pure logic (filter parsing, query building) with no DB
3. Integration-test the full HTTP → DB → response path
4. Follow existing patterns in nearby test files
