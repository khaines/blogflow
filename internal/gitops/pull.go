// Package gitops provides content synchronization and git operations for BlogFlow.
package gitops

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

// Puller handles git clone and pull operations for content/theme repos.
type Puller struct {
	auth   transport.AuthMethod
	logger *slog.Logger
}

// NewPuller creates a git puller with the given auth configuration.
func NewPuller(authCfg *AuthConfig, logger *slog.Logger) (*Puller, error) {
	if authCfg == nil {
		authCfg = &AuthConfig{Method: AuthNone}
	}

	if logger == nil {
		logger = slog.Default()
	}

	var auth transport.AuthMethod
	switch authCfg.Method {
	case AuthSSH:
		keys, err := ssh.NewPublicKeysFromFile("git", authCfg.SSHKeyPath, "")
		if err != nil {
			return nil, fmt.Errorf("gitops: loading SSH key: %w", err)
		}
		auth = keys
	case AuthToken:
		auth = &http.BasicAuth{
			Username: "x-access-token", // GitHub token auth convention
			Password: authCfg.Token,
		}
	case AuthNone:
		auth = nil // public repos
	}

	return &Puller{auth: auth, logger: logger}, nil
}

// CloneOrPull clones a repo to destPath if it doesn't exist, or pulls if it does.
// Returns true if content changed, false if already up-to-date.
//
// repoURL must match the originally-cloned remote URL. Because pulls operate on
// the existing remote configuration, a changed URL only takes effect when the
// fallback re-clone path is triggered (e.g. shallow-clone corruption). To
// intentionally switch URLs, delete destPath first and let CloneOrPull re-clone.
func (p *Puller) CloneOrPull(ctx context.Context, repoURL, branch, destPath string) (changed bool, err error) {
	if _, err := os.Stat(filepath.Join(destPath, ".git")); err == nil {
		return p.pull(ctx, repoURL, branch, destPath)
	}
	return true, p.clone(ctx, repoURL, branch, destPath)
}

// SanitizeURL strips embedded credentials from a URL for safe logging.
func SanitizeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	u.User = nil
	return u.String()
}

func (p *Puller) clone(ctx context.Context, repoURL, branch, destPath string) error {
	p.logger.Info("cloning repository", "url", SanitizeURL(repoURL), "branch", branch, "dest", destPath)

	opts := &git.CloneOptions{
		URL:           repoURL,
		Auth:          p.auth,
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		SingleBranch:  true,
		Depth:         1, // shallow clone for efficiency
	}

	_, err := git.PlainCloneContext(ctx, destPath, false, opts)
	if err != nil {
		return fmt.Errorf("gitops: clone %s: %w", SanitizeURL(repoURL), err)
	}

	p.logger.Info("clone complete", "url", SanitizeURL(repoURL))
	return nil
}

func (p *Puller) pull(ctx context.Context, repoURL, branch, destPath string) (bool, error) {
	p.logger.Debug("pulling repository", "dest", destPath, "branch", branch)

	repo, err := git.PlainOpen(destPath)
	if err != nil {
		return false, fmt.Errorf("gitops: open repo %s: %w", destPath, err)
	}

	// Record HEAD before pull so we can detect actual content changes.
	// Force-pull may not return NoErrAlreadyUpToDate even when nothing changed.
	headBefore, err := repo.Head()
	if err != nil {
		return false, fmt.Errorf("gitops: head %s: %w", destPath, err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return false, fmt.Errorf("gitops: worktree %s: %w", destPath, err)
	}

	err = wt.PullContext(ctx, &git.PullOptions{
		Auth:          p.auth,
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		SingleBranch:  true,
		Force:         true,
	})

	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		// Shallow clone + pull is a known go-git limitation.
		// Fall back to delete and re-clone.
		p.logger.Warn("pull failed, falling back to re-clone",
			"dest", destPath, "error", err)
		if removeErr := os.RemoveAll(destPath); removeErr != nil {
			return false, fmt.Errorf("gitops: failed to clear for re-clone %s: %w", destPath, removeErr)
		}
		if cloneErr := p.clone(ctx, repoURL, branch, destPath); cloneErr != nil {
			return false, cloneErr
		}
		return true, nil
	}

	headAfter, err := repo.Head()
	if err != nil {
		return false, fmt.Errorf("gitops: head after pull %s: %w", destPath, err)
	}

	changed := headBefore.Hash() != headAfter.Hash()
	if changed {
		p.logger.Info("pull complete — content updated", "dest", destPath)
	} else {
		p.logger.Debug("already up to date", "dest", destPath)
	}
	return changed, nil
}
