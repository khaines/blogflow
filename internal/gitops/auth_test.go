package gitops

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

	cfg, err := LoadAuthFromEnv(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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

	cfg, err := LoadAuthFromEnv(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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

	cfg, err := LoadAuthFromEnv(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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

	cfg, err := LoadAuthFromEnv(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Method != AuthSSH {
		t.Fatalf("expected AuthSSH (precedence over token), got %d", cfg.Method)
	}
}

func TestLoadAuthFromEnv_SSHKeyMissing(t *testing.T) {
	t.Setenv("BLOGFLOW_GIT_SSH_KEY", "/nonexistent/path/id_ed25519")
	t.Setenv("BLOGFLOW_GIT_TOKEN", "ghp_xxxxxxxxxxxxxxxxxxxx")

	_, err := LoadAuthFromEnv(nil)
	if err == nil {
		t.Fatal("expected error when SSH key is explicitly set but inaccessible")
	}
	if !strings.Contains(err.Error(), "SSH key not accessible") {
		t.Fatalf("unexpected error message: %v", err)
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

func TestValidate_SSHPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission checks not applicable on Windows")
	}

	tmp := t.TempDir()

	// 0644 should be rejected
	unsafeKey := filepath.Join(tmp, "unsafe_key")
	if err := os.WriteFile(unsafeKey, []byte("fake-key"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &AuthConfig{Method: AuthSSH, SSHKeyPath: unsafeKey}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for 0644 permissions")
	}
	if !strings.Contains(err.Error(), "unsafe permissions") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// 0600 should be accepted
	safeKey600 := filepath.Join(tmp, "safe_key_600")
	if err := os.WriteFile(safeKey600, []byte("fake-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg = &AuthConfig{Method: AuthSSH, SSHKeyPath: safeKey600}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected 0600 to be accepted, got: %v", err)
	}

	// 0400 should be accepted
	safeKey400 := filepath.Join(tmp, "safe_key_400")
	if err := os.WriteFile(safeKey400, []byte("fake-key"), 0o400); err != nil {
		t.Fatal(err)
	}
	cfg = &AuthConfig{Method: AuthSSH, SSHKeyPath: safeKey400}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected 0400 to be accepted, got: %v", err)
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

func TestValidate_UnknownMethod(t *testing.T) {
	cfg := &AuthConfig{Method: AuthMethod(99)}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for unknown auth method")
	}
	if !strings.Contains(err.Error(), "unknown auth method") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestAuthConfig_StringRedaction(t *testing.T) {
	cfg := AuthConfig{Method: AuthToken, Token: "ghp_supersecret123"}
	s := cfg.String()
	if strings.Contains(s, "ghp_supersecret123") {
		t.Fatal("String() must not contain the raw token")
	}
	if !strings.Contains(s, "[REDACTED]") {
		t.Fatal("String() must contain [REDACTED] for token auth")
	}

	cfg = AuthConfig{Method: AuthSSH, SSHKeyPath: "/home/user/.ssh/id_ed25519"}
	s = cfg.String()
	if !strings.Contains(s, "/home/user/.ssh/id_ed25519") {
		t.Fatal("String() should include SSH key path")
	}
}

func TestAuthConfig_LogValueRedaction(t *testing.T) {
	// Token method: must redact token, must not leak ssh_key_path
	cfg := AuthConfig{Method: AuthToken, Token: "ghp_supersecret123"}
	lv := cfg.LogValue()
	resolved := lv.Resolve().String()
	if strings.Contains(resolved, "ghp_supersecret123") {
		t.Fatal("LogValue() must not contain the raw token")
	}
	if !strings.Contains(resolved, "[REDACTED]") {
		t.Fatal("LogValue() must contain [REDACTED] for token method")
	}
	if strings.Contains(resolved, "ssh_key_path") {
		t.Fatal("LogValue() for token method must not include ssh_key_path")
	}

	// SSH method: must include ssh_key_path, must not include token field
	cfg = AuthConfig{Method: AuthSSH, SSHKeyPath: "/home/user/.ssh/id_ed25519"}
	lv = cfg.LogValue()
	resolved = lv.Resolve().String()
	if !strings.Contains(resolved, "/home/user/.ssh/id_ed25519") {
		t.Fatal("LogValue() for SSH method must include ssh_key_path")
	}
	if strings.Contains(resolved, "token") {
		t.Fatal("LogValue() for SSH method must not include token field")
	}
	if !strings.Contains(resolved, "ssh") {
		t.Fatal("LogValue() for SSH method must include method=ssh")
	}

	// None method: must only have method=none
	cfg = AuthConfig{Method: AuthNone}
	lv = cfg.LogValue()
	resolved = lv.Resolve().String()
	if !strings.Contains(resolved, "none") {
		t.Fatal("LogValue() for AuthNone must include method=none")
	}
	if strings.Contains(resolved, "token") {
		t.Fatal("LogValue() for AuthNone must not include token field")
	}
	if strings.Contains(resolved, "ssh_key_path") {
		t.Fatal("LogValue() for AuthNone must not include ssh_key_path field")
	}
}

func TestAuthMethod_String(t *testing.T) {
	tests := []struct {
		m    AuthMethod
		want string
	}{
		{AuthNone, "none"},
		{AuthSSH, "ssh"},
		{AuthToken, "token"},
		{AuthMethod(99), "none"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("AuthMethod(%d)", tt.m), func(t *testing.T) {
			if got := tt.m.String(); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// Verify AuthConfig implements slog.LogValuer at compile time.
var _ slog.LogValuer = AuthConfig{}
