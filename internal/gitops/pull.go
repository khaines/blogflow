// Package gitops provides content synchronization and git operations for BlogFlow.
package gitops

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// Puller handles git clone and pull operations for content/theme repos.
type Puller struct {
	auth       transport.AuthMethod
	logger     *slog.Logger
	SparseDirs []string // if non-empty, only these directories are checked out
	depth      int      // git clone/fetch depth; 1 = shallowest
}

// PullerOption configures optional Puller behaviour.
type PullerOption func(*Puller)

// WithCloneDepth sets the shallow clone/fetch depth.
// Values < 1 are silently clamped to 1.
func WithCloneDepth(depth int) PullerOption {
	return func(p *Puller) {
		if depth < 1 {
			depth = 1
		}
		p.depth = depth
	}
}

// NewPuller creates a git puller with the given auth configuration.
func NewPuller(authCfg *AuthConfig, logger *slog.Logger, opts ...PullerOption) (*Puller, error) {
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

	p := &Puller{auth: auth, logger: logger, depth: 1}
	for _, opt := range opts {
		opt(p)
	}
	return p, nil
}

// CloneOrPull clones a repo to destPath if it doesn't exist, or pulls if it does.
// Returns true if content changed, false if already up-to-date.
//
// repoURL must match the originally-cloned remote URL. Because pulls operate on
// the existing remote configuration, a changed URL only takes effect when the
// fallback re-clone path is triggered (e.g. shallow-clone corruption). To
// intentionally switch URLs, delete destPath first and let CloneOrPull re-clone.
func (p *Puller) CloneOrPull(ctx context.Context, repoURL, branch, destPath string) (changed bool, err error) {
	tracer := otel.Tracer("github.com/khaines/blogflow/gitops")
	ctx, span := tracer.Start(ctx, "gitops.CloneOrPull")
	defer span.End()

	span.SetAttributes(
		attribute.String("gitops.repo_url", SanitizeURL(repoURL)),
		attribute.String("gitops.branch", branch),
	)

	if len(p.SparseDirs) > 0 {
		cleaned, valErr := validateSparseDirs(p.SparseDirs)
		if valErr != nil {
			span.SetStatus(codes.Error, valErr.Error())
			span.RecordError(valErr)
			return false, fmt.Errorf("gitops: %w", valErr)
		}
		p.SparseDirs = cleaned
	}

	if _, err := os.Stat(filepath.Join(destPath, ".git")); err == nil {
		span.SetAttributes(attribute.String("gitops.operation", "pull"))
		changed, pullErr := p.pull(ctx, repoURL, branch, destPath)
		if pullErr != nil {
			span.SetStatus(codes.Error, pullErr.Error())
			span.RecordError(pullErr)
		}
		return changed, pullErr
	}

	span.SetAttributes(attribute.String("gitops.operation", "clone"))
	cloneErr := p.clone(ctx, repoURL, branch, destPath)
	if cloneErr != nil {
		span.SetStatus(codes.Error, cloneErr.Error())
		span.RecordError(cloneErr)
	}
	return true, cloneErr
}

// validateSparseDirs sanitizes and validates sparse directory entries.
// It rejects absolute paths and path-traversal components, and normalizes
// each entry (trailing slashes, redundant separators).
func validateSparseDirs(dirs []string) ([]string, error) {
	cleaned := make([]string, 0, len(dirs))
	for _, d := range dirs {
		d = path.Clean(d)
		if d == "." || d == "" {
			continue
		}
		if path.IsAbs(d) {
			return nil, fmt.Errorf("sparse dir must be relative, got %q", d)
		}
		if d == ".." || strings.HasPrefix(d, "../") || strings.Contains(d, "/../") {
			return nil, fmt.Errorf("sparse dir must not contain path traversal, got %q", d)
		}
		cleaned = append(cleaned, d)
	}
	return cleaned, nil
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

func (p *Puller) clone(ctx context.Context, repoURL, branch, destPath string) (retErr error) {
	tracer := otel.Tracer("github.com/khaines/blogflow/gitops")
	ctx, span := tracer.Start(ctx, "gitops.clone")
	defer func() {
		if retErr != nil {
			span.SetStatus(codes.Error, retErr.Error())
			span.RecordError(retErr)
		}
		span.End()
	}()
	span.SetAttributes(attribute.String("gitops.repo_url", SanitizeURL(repoURL)))

	p.logger.Info("cloning repository", "url", SanitizeURL(repoURL), "branch", branch, "dest", destPath)

	sparse := len(p.SparseDirs) > 0

	opts := &git.CloneOptions{
		URL:           repoURL,
		Auth:          p.auth,
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		SingleBranch:  true,
		Depth:         p.depth,
		Tags:          git.NoTags, // content repos don't need tags
		NoCheckout:    sparse,
	}

	repo, err := git.PlainCloneContext(ctx, destPath, false, opts)
	if err != nil {
		return fmt.Errorf("gitops: clone %s: %w", SanitizeURL(repoURL), err)
	}

	if sparse {
		wt, wtErr := repo.Worktree()
		if wtErr != nil {
			return fmt.Errorf("gitops: worktree after clone %s: %w", SanitizeURL(repoURL), wtErr)
		}
		if coErr := wt.Checkout(&git.CheckoutOptions{
			Branch:                    plumbing.NewBranchReferenceName(branch),
			SparseCheckoutDirectories: p.SparseDirs,
		}); coErr != nil {
			return fmt.Errorf("gitops: sparse checkout %s: %w", SanitizeURL(repoURL), coErr)
		}
		p.logger.Info("sparse checkout applied", "dirs", p.SparseDirs)
	}

	p.logger.Info("clone complete", "url", SanitizeURL(repoURL))
	return nil
}

func (p *Puller) pull(ctx context.Context, repoURL, branch, destPath string) (_ bool, retErr error) {
	tracer := otel.Tracer("github.com/khaines/blogflow/gitops")
	ctx, span := tracer.Start(ctx, "gitops.pull")
	defer func() {
		if retErr != nil {
			span.SetStatus(codes.Error, retErr.Error())
			span.RecordError(retErr)
		}
		span.End()
	}()

	p.logger.Debug("pulling repository", "dest", destPath, "branch", branch)

	repo, err := git.PlainOpen(destPath)
	if err != nil {
		return false, fmt.Errorf("gitops: open repo %s: %w", destPath, err)
	}

	// Record HEAD before fetch so we can detect actual content changes.
	headBefore, err := repo.Head()
	if err != nil {
		return false, fmt.Errorf("gitops: head %s: %w", destPath, err)
	}

	// Use FetchContext + hard reset instead of PullContext so we can set
	// Tags: NoTags — PullOptions does not expose a Tags field.
	fetchErr := repo.FetchContext(ctx, &git.FetchOptions{
		Auth:  p.auth,
		Depth: p.depth,
		Tags:  git.NoTags,
		Force: true,
	})

	switch {
	case fetchErr == nil:
		// New objects fetched — fast-forward worktree below.
	case errors.Is(fetchErr, git.NoErrAlreadyUpToDate):
		p.logger.Debug("already up to date", "dest", destPath)
		return false, nil
	default:
		// Shallow clone + fetch is a known go-git limitation.
		// Fall back to delete and re-clone.
		p.logger.Warn("pull failed, falling back to re-clone",
			"dest", destPath, "error", fetchErr)
		if removeErr := os.RemoveAll(destPath); removeErr != nil {
			return false, fmt.Errorf("gitops: failed to clear for re-clone %s: %w", destPath, removeErr)
		}
		if cloneErr := p.clone(ctx, repoURL, branch, destPath); cloneErr != nil {
			return false, cloneErr
		}
		return true, nil
	}

	// Resolve the remote tracking ref and hard-reset the worktree.
	remoteRef, err := repo.Reference(
		plumbing.NewRemoteReferenceName("origin", branch), true)
	if err != nil {
		return false, fmt.Errorf("gitops: resolve remote ref %s: %w", destPath, err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return false, fmt.Errorf("gitops: worktree %s: %w", destPath, err)
	}

	if err := wt.Reset(&git.ResetOptions{
		Commit: remoteRef.Hash(),
		Mode:   git.HardReset,
	}); err != nil {
		return false, fmt.Errorf("gitops: reset %s: %w", destPath, err)
	}

	headAfter, err := repo.Head()
	if err != nil {
		return false, fmt.Errorf("gitops: head after pull %s: %w", destPath, err)
	}

	changed := headBefore.Hash() != headAfter.Hash()
	span.SetAttributes(attribute.Bool("gitops.changed", changed))

	// Re-apply sparse checkout after pull — PullContext checks out all files,
	// so we remove entries outside the sparse set.
	if changed && len(p.SparseDirs) > 0 {
		if cleanErr := p.cleanNonSparsePaths(destPath); cleanErr != nil {
			return false, fmt.Errorf("gitops: sparse cleanup after pull %s: %w", destPath, cleanErr)
		}
		p.logger.Info("sparse checkout re-applied after pull", "dirs", p.SparseDirs)
	}

	if changed {
		p.logger.Info("pull complete — content updated", "dest", destPath)
	} else {
		p.logger.Debug("already up to date", "dest", destPath)
	}
	return changed, nil
}

// cleanNonSparsePaths removes top-level files and directories from destPath
// that are not in the configured SparseDirs set. The .git directory is always
// preserved. Nested sparse dirs (e.g. "posts/drafts") are handled by
// extracting the top-level component for the allow-list.
func (p *Puller) cleanNonSparsePaths(destPath string) error {
	allowed := make(map[string]bool, len(p.SparseDirs))
	for _, d := range p.SparseDirs {
		// Extract top-level component so "posts/drafts" preserves "posts".
		top := strings.SplitN(d, "/", 2)[0]
		allowed[top] = true
	}

	entries, err := os.ReadDir(destPath)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", destPath, err)
	}

	for _, e := range entries {
		name := e.Name()
		if name == ".git" || allowed[name] {
			continue
		}
		if err := os.RemoveAll(filepath.Join(destPath, name)); err != nil {
			return fmt.Errorf("remove %s: %w", name, err)
		}
	}
	return nil
}
