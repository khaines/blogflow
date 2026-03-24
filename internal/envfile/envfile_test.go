package envfile

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadEnvOrFile_FileExists(t *testing.T) {
	tmp := t.TempDir()
	secretFile := filepath.Join(tmp, "secret.txt")
	if err := os.WriteFile(secretFile, []byte("  file-secret-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TEST_SECRET_FILE", secretFile)
	t.Setenv("TEST_SECRET", "")

	val, ok, err := ReadEnvOrFile("TEST_SECRET", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if val != "file-secret-value" {
		t.Fatalf("got %q, want %q", val, "file-secret-value")
	}
}

func TestReadEnvOrFile_FileMissing_FallsBackToEnv(t *testing.T) {
	t.Setenv("TEST_SECRET_FILE", "")
	t.Setenv("TEST_SECRET", "env-value")

	val, ok, err := ReadEnvOrFile("TEST_SECRET", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if val != "env-value" {
		t.Fatalf("got %q, want %q", val, "env-value")
	}
}

func TestReadEnvOrFile_BothSet_FileWins_Warning(t *testing.T) {
	tmp := t.TempDir()
	secretFile := filepath.Join(tmp, "secret.txt")
	if err := os.WriteFile(secretFile, []byte("from-file"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TEST_SECRET_FILE", secretFile)
	t.Setenv("TEST_SECRET", "from-env")

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	val, ok, err := ReadEnvOrFile("TEST_SECRET", logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if val != "from-file" {
		t.Fatalf("got %q, want %q (file should take precedence)", val, "from-file")
	}
	if !strings.Contains(buf.String(), "both env var and _FILE variant set") {
		t.Fatalf("expected warning log, got: %s", buf.String())
	}
}

func TestReadEnvOrFile_NeitherSet(t *testing.T) {
	t.Setenv("TEST_SECRET_FILE", "")
	t.Setenv("TEST_SECRET", "")

	val, ok, err := ReadEnvOrFile("TEST_SECRET", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false when neither is set")
	}
	if val != "" {
		t.Fatalf("expected empty string, got %q", val)
	}
}

func TestReadEnvOrFile_FileNotReadable(t *testing.T) {
	t.Setenv("TEST_SECRET_FILE", "/nonexistent/path/secret.txt")
	t.Setenv("TEST_SECRET", "")

	_, _, err := ReadEnvOrFile("TEST_SECRET", nil)
	if err == nil {
		t.Fatal("expected error when _FILE points to nonexistent file")
	}
	if !strings.Contains(err.Error(), "TEST_SECRET_FILE") {
		t.Fatalf("error should mention the env var, got: %v", err)
	}
}

func TestReadEnvOrFile_EmptyFile(t *testing.T) {
	tmp := t.TempDir()
	secretFile := filepath.Join(tmp, "empty.txt")
	if err := os.WriteFile(secretFile, []byte("   \n  "), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TEST_SECRET_FILE", secretFile)
	t.Setenv("TEST_SECRET", "")

	val, ok, err := ReadEnvOrFile("TEST_SECRET", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true (file exists)")
	}
	if val != "" {
		t.Fatalf("expected empty string after trimming, got %q", val)
	}
}
