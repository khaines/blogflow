package gitops

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAuthFromEnv_SSHKey(t *testing.T) {
	tmp := t.TempDir()
	keyFile := filepath.Join(tmp, "id_ed25519")
	if err := os.WriteFile(keyFile, []byte("fake-key"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("BLOGFLOW_GIT_SSH_KEY", keyFile)
	t.Setenv("BLOGFLOW_GIT_TOKEN", "")

	cfg := LoadAuthFromEnv(nil)
	if cfg.Method != AuthSSH {
		t.Fatalf("expected AuthSSH, got %d", cfg.Method)
	}
	if cfg.SSHKeyPath != keyFile {
		t.Fatalf("expected SSHKeyPath %q, got %q", keyFile, cfg.SSHKeyPath)
	}
}

func TestLoadAuthFromEnv_Token(t *testing.T) {
	t.Setenv("BLOGFLOW_GIT_SSH_KEY", "")
	t.Setenv("BLOGFLOW_GIT_TOKEN", "ghp_xxxxxxxxxxxxxxxxxxxx")

	cfg := LoadAuthFromEnv(nil)
	if cfg.Method != AuthToken {
		t.Fatalf("expected AuthToken, got %d", cfg.Method)
	}
	if cfg.Token != "ghp_xxxxxxxxxxxxxxxxxxxx" {
		t.Fatalf("unexpected token value")
	}
}

func TestLoadAuthFromEnv_None(t *testing.T) {
	t.Setenv("BLOGFLOW_GIT_SSH_KEY", "")
	t.Setenv("BLOGFLOW_GIT_TOKEN", "")

	cfg := LoadAuthFromEnv(nil)
	if cfg.Method != AuthNone {
		t.Fatalf("expected AuthNone, got %d", cfg.Method)
	}
}

func TestLoadAuthFromEnv_SSHKeyPrecedence(t *testing.T) {
	tmp := t.TempDir()
	keyFile := filepath.Join(tmp, "id_ed25519")
	if err := os.WriteFile(keyFile, []byte("fake-key"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("BLOGFLOW_GIT_SSH_KEY", keyFile)
	t.Setenv("BLOGFLOW_GIT_TOKEN", "ghp_xxxxxxxxxxxxxxxxxxxx")

	cfg := LoadAuthFromEnv(nil)
	if cfg.Method != AuthSSH {
		t.Fatalf("expected AuthSSH (precedence over token), got %d", cfg.Method)
	}
}

func TestLoadAuthFromEnv_SSHKeyMissing(t *testing.T) {
	t.Setenv("BLOGFLOW_GIT_SSH_KEY", "/nonexistent/path/id_ed25519")
	t.Setenv("BLOGFLOW_GIT_TOKEN", "ghp_xxxxxxxxxxxxxxxxxxxx")

	cfg := LoadAuthFromEnv(nil)
	if cfg.Method != AuthToken {
		t.Fatalf("expected AuthToken (SSH key missing falls through), got %d", cfg.Method)
	}
}

func TestValidate_SSH(t *testing.T) {
	tmp := t.TempDir()
	keyFile := filepath.Join(tmp, "id_ed25519")
	if err := os.WriteFile(keyFile, []byte("fake-key"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Valid SSH config
	cfg := &AuthConfig{Method: AuthSSH, SSHKeyPath: keyFile}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Empty key path
	cfg = &AuthConfig{Method: AuthSSH, SSHKeyPath: ""}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for empty SSH key path")
	}

	// Non-existent key
	cfg = &AuthConfig{Method: AuthSSH, SSHKeyPath: "/nonexistent/key"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for non-existent SSH key")
	}
}

func TestValidate_Token(t *testing.T) {
	// Valid token
	cfg := &AuthConfig{Method: AuthToken, Token: "ghp_xxxxxxxxxxxxxxxxxxxx"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Empty token
	cfg = &AuthConfig{Method: AuthToken, Token: ""}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestValidate_None(t *testing.T) {
	cfg := &AuthConfig{Method: AuthNone}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error for AuthNone, got %v", err)
	}
}
