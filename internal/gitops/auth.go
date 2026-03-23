// Package gitops provides content synchronization and git authentication for BlogFlow.
package gitops

import (
	"fmt"
	"log/slog"
	"os"
)

// AuthMethod represents a git authentication configuration.
type AuthMethod int

const (
	AuthNone AuthMethod = iota
	AuthSSH
	AuthToken
)

// String returns a human-readable label for the AuthMethod.
func (m AuthMethod) String() string {
	switch m {
	case AuthSSH:
		return "ssh"
	case AuthToken:
		return "token"
	default:
		return "none"
	}
}

// AuthConfig holds git authentication settings derived from environment variables.
type AuthConfig struct {
	Method     AuthMethod
	SSHKeyPath string // from BLOGFLOW_GIT_SSH_KEY
	Token      string // from BLOGFLOW_GIT_TOKEN
}

// String returns a human-readable representation with secrets redacted.
func (a AuthConfig) String() string {
	switch a.Method {
	case AuthSSH:
		return fmt.Sprintf("AuthConfig{Method:ssh, SSHKeyPath:%s}", a.SSHKeyPath)
	case AuthToken:
		return fmt.Sprintf("AuthConfig{Method:token, Token:[REDACTED]}")
	default:
		return "AuthConfig{Method:none}"
	}
}

// LogValue implements slog.LogValuer to redact secrets in structured logs.
func (a AuthConfig) LogValue() slog.Value {
	switch a.Method {
	case AuthSSH:
		return slog.GroupValue(
			slog.String("method", "ssh"),
			slog.String("ssh_key_path", a.SSHKeyPath),
		)
	case AuthToken:
		return slog.GroupValue(
			slog.String("method", "token"),
			slog.String("token", "[REDACTED]"),
		)
	default:
		return slog.GroupValue(
			slog.String("method", "none"),
		)
	}
}

// LoadAuthFromEnv reads git authentication configuration from environment variables.
// Returns an error if credentials are explicitly set but unusable.
// Returns AuthNone if no credentials are configured.
func LoadAuthFromEnv(logger *slog.Logger) (*AuthConfig, error) {
	if logger == nil {
		logger = slog.Default()
	}

	// Check SSH key first (preferred)
	if keyPath := os.Getenv("BLOGFLOW_GIT_SSH_KEY"); keyPath != "" {
		cfg := &AuthConfig{Method: AuthSSH, SSHKeyPath: keyPath}
		if err := cfg.Validate(); err != nil {
			return nil, err
		}
		logger.Info("git auth: SSH key configured", "path", keyPath)
		return cfg, nil
	}

	// Check token
	if token := os.Getenv("BLOGFLOW_GIT_TOKEN"); token != "" {
		cfg := &AuthConfig{Method: AuthToken, Token: token}
		if err := cfg.Validate(); err != nil {
			return nil, err
		}
		logger.Info("git auth: token configured")
		return cfg, nil
	}

	logger.Debug("git auth: no credentials configured (public repos only)")
	cfg := &AuthConfig{Method: AuthNone}
	return cfg, nil
}

// Validate checks the auth configuration is usable.
func (a *AuthConfig) Validate() error {
	switch a.Method {
	case AuthSSH:
		if a.SSHKeyPath == "" {
			return fmt.Errorf("gitops: SSH auth configured but key path is empty")
		}
		info, err := os.Stat(a.SSHKeyPath)
		if err != nil {
			return fmt.Errorf("gitops: SSH key not accessible: %w", err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("gitops: SSH key %s has unsafe permissions %o; must be 0600 or 0400", a.SSHKeyPath, info.Mode().Perm())
		}
	case AuthToken:
		if a.Token == "" {
			return fmt.Errorf("gitops: token auth configured but token is empty")
		}
	case AuthNone:
		// valid
	default:
		return fmt.Errorf("gitops: unknown auth method: %d", a.Method)
	}
	return nil
}
