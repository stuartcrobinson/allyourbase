package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestSetVersion(t *testing.T) {
	SetVersion("1.2.3", "abc123", "2026-01-01")
	if buildVersion != "1.2.3" {
		t.Fatalf("expected 1.2.3, got %q", buildVersion)
	}
	if buildCommit != "abc123" {
		t.Fatalf("expected abc123, got %q", buildCommit)
	}
	if buildDate != "2026-01-01" {
		t.Fatalf("expected 2026-01-01, got %q", buildDate)
	}
	SetVersion("dev", "none", "unknown")
}

// captureStdout captures stdout output from the given function.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	buf := make([]byte, 64*1024)
	n, _ := r.Read(buf)
	r.Close()
	return string(buf[:n])
}

func TestVersionCommand(t *testing.T) {
	SetVersion("0.1.0", "deadbeef", "2026-02-07")
	defer SetVersion("dev", "none", "unknown")

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"version"})
		_ = rootCmd.Execute()
	})

	if !strings.Contains(output, "0.1.0") {
		t.Fatalf("expected version in output, got %q", output)
	}
	if !strings.Contains(output, "deadbeef") {
		t.Fatalf("expected commit in output, got %q", output)
	}
}

func TestConfigCommandProducesValidTOML(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(origDir)

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"config"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Verify it's valid TOML.
	var parsed map[string]any
	if err := toml.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("config output is not valid TOML: %v\noutput:\n%s", err, output)
	}
	if _, ok := parsed["server"]; !ok {
		t.Fatal("expected 'server' section in config output")
	}
	if _, ok := parsed["database"]; !ok {
		t.Fatal("expected 'database' section in config output")
	}
}

func TestRootCommandRegistersSubcommands(t *testing.T) {
	expected := []string{"start", "stop", "status", "config", "version", "migrate", "admin"}

	commands := make(map[string]bool)
	for _, cmd := range rootCmd.Commands() {
		commands[cmd.Use] = true
	}

	for _, name := range expected {
		found := false
		for use := range commands {
			if strings.HasPrefix(use, name) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q to be registered", name)
		}
	}
}

func TestMigrateCreateGeneratesFile(t *testing.T) {
	tmpDir := t.TempDir()
	migrDir := filepath.Join(tmpDir, "migrations")

	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(origDir)

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"migrate", "create", "add_posts", "--migrations-dir", migrDir})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "Created migration") {
		t.Fatalf("expected 'Created migration' in output, got %q", output)
	}

	entries, err := os.ReadDir(migrDir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 migration file, got %d", len(entries))
	}
	if !strings.HasSuffix(entries[0].Name(), "_add_posts.sql") {
		t.Fatalf("expected filename ending in _add_posts.sql, got %q", entries[0].Name())
	}
}

func TestHelpDoesNotError(t *testing.T) {
	rootCmd.SetArgs([]string{"--help"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
