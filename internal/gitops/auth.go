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

// AuthConfig holds git authentication settings derived from environment variables.
type AuthConfig struct {
	Method     AuthMethod
	SSHKeyPath string // from BLOGFLOW_GIT_SSH_KEY
	Token      string // from BLOGFLOW_GIT_TOKEN
}

// LoadAuthFromEnv reads git authentication configuration from environment variables.
// Returns AuthNone if no credentials are configured.
func LoadAuthFromEnv(logger *slog.Logger) *AuthConfig {
	if logger == nil {
		logger = slog.Default()
	}

	// Check SSH key first (preferred)
	if keyPath := os.Getenv("BLOGFLOW_GIT_SSH_KEY"); keyPath != "" {
		if _, err := os.Stat(keyPath); err != nil {
			logger.Warn("BLOGFLOW_GIT_SSH_KEY path not accessible", "path", keyPath, "error", err)
		} else {
			logger.Info("git auth: SSH key configured", "path", keyPath)
			return &AuthConfig{Method: AuthSSH, SSHKeyPath: keyPath}
		}
	}

	// Check token
	if token := os.Getenv("BLOGFLOW_GIT_TOKEN"); token != "" {
		if len(token) < 10 {
			logger.Warn("BLOGFLOW_GIT_TOKEN appears too short — verify it's valid")
		}
		logger.Info("git auth: token configured")
		return &AuthConfig{Method: AuthToken, Token: token}
	}

	logger.Debug("git auth: no credentials configured (public repos only)")
	return &AuthConfig{Method: AuthNone}
}

// Validate checks the auth configuration is usable.
func (a *AuthConfig) Validate() error {
	switch a.Method {
	case AuthSSH:
		if a.SSHKeyPath == "" {
			return fmt.Errorf("gitops: SSH auth configured but key path is empty")
		}
		if _, err := os.Stat(a.SSHKeyPath); err != nil {
			return fmt.Errorf("gitops: SSH key not accessible: %w", err)
		}
	case AuthToken:
		if a.Token == "" {
			return fmt.Errorf("gitops: token auth configured but token is empty")
		}
	case AuthNone:
		// valid
	}
	return nil
}
