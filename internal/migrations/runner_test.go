//go:build integration

package migrations_test

import (
	"context"
	"os"
	"testing"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/testutil"
)

var sharedPG *testutil.PGContainer

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	sharedPG = pg
	code := m.Run()
	cleanup()
	os.Exit(code)
}

// resetDB drops and recreates the public schema for test isolation.
func resetDB(t *testing.T, ctx context.Context) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	if err != nil {
		t.Fatalf("resetting schema: %v", err)
	}
}

func TestBootstrap(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())

	// Bootstrap should create _ayb_migrations table.
	err := runner.Bootstrap(ctx)
	testutil.NoError(t, err)

	// Verify table exists.
	var exists bool
	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = '_ayb_migrations')").
		Scan(&exists)
	testutil.NoError(t, err)
	testutil.True(t, exists, "_ayb_migrations table should exist")
}

func TestBootstrapIdempotent(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())

	// Run bootstrap twice — should not error.
	err := runner.Bootstrap(ctx)
	testutil.NoError(t, err)
	err = runner.Bootstrap(ctx)
	testutil.NoError(t, err)
}

func TestRunMigrations(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	err := runner.Bootstrap(ctx)
	testutil.NoError(t, err)

	// Run migrations.
	applied, err := runner.Run(ctx)
	testutil.NoError(t, err)
	testutil.True(t, applied >= 1, "should apply at least 1 migration")

	// Verify _ayb_meta table was created by 001_ayb_meta.sql.
	var exists bool
	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = '_ayb_meta')").
		Scan(&exists)
	testutil.NoError(t, err)
	testutil.True(t, exists, "_ayb_meta table should exist")
}

func TestRunMigrationsIdempotent(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	err := runner.Bootstrap(ctx)
	testutil.NoError(t, err)

	// First run applies migrations.
	applied1, err := runner.Run(ctx)
	testutil.NoError(t, err)

	// Second run should apply zero.
	applied2, err := runner.Run(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, applied2, 0)

	// First run should have applied at least one.
	testutil.True(t, applied1 >= 1, "first run should apply migrations")
}

func TestGetApplied(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	err := runner.Bootstrap(ctx)
	testutil.NoError(t, err)

	// Before running, no applied migrations.
	applied, err := runner.GetApplied(ctx)
	testutil.NoError(t, err)
	testutil.SliceLen(t, applied, 0)

	// Run migrations.
	_, err = runner.Run(ctx)
	testutil.NoError(t, err)

	// After running, should have applied migrations.
	applied, err = runner.GetApplied(ctx)
	testutil.NoError(t, err)
	testutil.True(t, len(applied) >= 1, "should have applied migrations")
	testutil.Equal(t, applied[0].Name, "001_ayb_meta.sql")
	testutil.False(t, applied[0].AppliedAt.IsZero(), "applied_at should be set")
}
