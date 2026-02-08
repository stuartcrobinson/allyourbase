package pgmanager

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestPIDFileRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.pid")

	err := writePID(path, 12345)
	testutil.NoError(t, err)

	pid, err := readPID(path)
	testutil.NoError(t, err)
	testutil.Equal(t, pid, 12345)
}

func TestPIDFileReadMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.pid")
	pid, err := readPID(path)
	testutil.NoError(t, err)
	testutil.Equal(t, pid, 0)
}

func TestPIDFileRemoveMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.pid")
	err := removePID(path)
	testutil.NoError(t, err)
}

func TestPIDFileRemove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.pid")
	err := writePID(path, 99)
	testutil.NoError(t, err)

	err = removePID(path)
	testutil.NoError(t, err)

	// Should be gone.
	_, err = os.Stat(path)
	testutil.True(t, os.IsNotExist(err), "file should be removed")
}

func TestCleanupOrphanNoFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.pid")
	// Should not panic.
	cleanupOrphan(path, testutil.DiscardLogger())
}

func TestCleanupOrphanDeadProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stale.pid")
	// Write a PID that almost certainly doesn't exist.
	err := writePID(path, 2147483647)
	testutil.NoError(t, err)

	cleanupOrphan(path, testutil.DiscardLogger())

	// Stale PID file should be cleaned up.
	_, err = os.Stat(path)
	testutil.True(t, os.IsNotExist(err), "stale PID file should be removed")
}

func TestLogWriter(t *testing.T) {
	lw := newLogWriter(testutil.DiscardLogger())
	n, err := lw.Write([]byte("test output\n"))
	testutil.NoError(t, err)
	testutil.Equal(t, n, 12) // "test output\n" = 12 bytes
}

func TestLogWriterEmptyLine(t *testing.T) {
	lw := newLogWriter(testutil.DiscardLogger())
	n, err := lw.Write([]byte("\n"))
	testutil.NoError(t, err)
	testutil.Equal(t, n, 1)
}

func TestConnURLFormat(t *testing.T) {
	m := &Manager{
		connURL: "postgresql://ayb:ayb@127.0.0.1:15432/ayb?sslmode=disable",
	}
	testutil.Equal(t, m.ConnURL(), "postgresql://ayb:ayb@127.0.0.1:15432/ayb?sslmode=disable")
}

func TestNewDoesNotStart(t *testing.T) {
	m := New(Config{Logger: testutil.DiscardLogger()})
	testutil.False(t, m.IsRunning(), "should not be running after New()")
	testutil.Equal(t, m.ConnURL(), "")
}

func TestAybHome(t *testing.T) {
	home, err := aybHome()
	testutil.NoError(t, err)
	testutil.True(t, home != "", "home should not be empty")

	info, err := os.Stat(home)
	testutil.NoError(t, err)
	testutil.True(t, info.IsDir(), "should be a directory")
}

func TestReadPostmasterPID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "postmaster.pid")
	// Postgres postmaster.pid has the PID on the first line.
	err := os.WriteFile(path, []byte("42\n/some/data/dir\n5432\n"), 0o644)
	testutil.NoError(t, err)

	pid, err := readPostmasterPID(path)
	testutil.NoError(t, err)
	testutil.Equal(t, pid, 42)
}

func TestStopWhenNotRunning(t *testing.T) {
	m := New(Config{Logger: testutil.DiscardLogger()})
	err := m.Stop()
	testutil.NoError(t, err)
}
