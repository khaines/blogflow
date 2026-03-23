package gitops

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"golang.org/x/crypto/ssh"
)

// newBareRepoWithCommit creates a local bare repo with one commit on "main".
// Returns the bare repo path. Uses t.TempDir() so cleanup is automatic.
func newBareRepoWithCommit(t *testing.T) string {
	t.Helper()

	// Create a non-bare repo, commit a file, then create a bare clone.
	srcDir := filepath.Join(t.TempDir(), "src")
	repo, err := git.PlainInit(srcDir, false)
	if err != nil {
		t.Fatalf("init source repo: %v", err)
	}

	// Write a file and commit it.
	if err := os.WriteFile(filepath.Join(srcDir, "README.md"), []byte("# test\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err = wt.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "test",
			Email: "test@test.com",
			When:  time.Now(),
		},
	}); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Clone into a bare repo that we can use as a remote.
	bareDir := filepath.Join(t.TempDir(), "bare")
	if _, err := git.PlainClone(bareDir, true, &git.CloneOptions{URL: srcDir}); err != nil {
		t.Fatalf("bare clone: %v", err)
	}

	return bareDir
}

// addCommitToBareRepo pushes a new commit to a bare repo by cloning it,
// committing a file, and pushing back.
func addCommitToBareRepo(t *testing.T, bareDir, filename, content string) {
	t.Helper()

	tmpDir := filepath.Join(t.TempDir(), "push-src")
	repo, err := git.PlainClone(tmpDir, false, &git.CloneOptions{URL: bareDir})
	if err != nil {
		t.Fatalf("clone for push: %v", err)
	}

	if err := os.WriteFile(filepath.Join(tmpDir, filename), []byte(content), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	if _, err := wt.Add(filename); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := wt.Commit("add "+filename, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "test",
			Email: "test@test.com",
			When:  time.Now(),
		},
	}); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if err := repo.Push(&git.PushOptions{}); err != nil {
		t.Fatalf("push: %v", err)
	}
}

// generateTempSSHKey writes a PEM-encoded Ed25519 private key to a temp file
// and returns its path.
func generateTempSSHKey(t *testing.T) string {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}

	pemBytes, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}

	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(pemBytes), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	return keyPath
}

func TestNewPuller_AuthNone(t *testing.T) {
	p, err := NewPuller(&AuthConfig{Method: AuthNone}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.auth != nil {
		t.Fatal("expected nil auth for AuthNone")
	}
}

func TestNewPuller_NilAuthConfig(t *testing.T) {
	p, err := NewPuller(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.auth != nil {
		t.Fatal("expected nil auth when authCfg is nil (defaults to AuthNone)")
	}
}

func TestNewPuller_AuthToken(t *testing.T) {
	p, err := NewPuller(&AuthConfig{Method: AuthToken, Token: "ghp_test123"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.auth == nil {
		t.Fatal("expected non-nil auth for AuthToken")
	}
}

func TestNewPuller_AuthSSH(t *testing.T) {
	keyPath := generateTempSSHKey(t)

	p, err := NewPuller(&AuthConfig{Method: AuthSSH, SSHKeyPath: keyPath}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.auth == nil {
		t.Fatal("expected non-nil auth for AuthSSH")
	}
}

func TestNewPuller_AuthSSH_BadPath(t *testing.T) {
	_, err := NewPuller(&AuthConfig{Method: AuthSSH, SSHKeyPath: "/nonexistent/key"}, nil)
	if err == nil {
		t.Fatal("expected error for missing SSH key")
	}
}

func TestCloneOrPull_CloneNew(t *testing.T) {
	bareRepo := newBareRepoWithCommit(t)

	p, err := NewPuller(&AuthConfig{Method: AuthNone}, nil)
	if err != nil {
		t.Fatalf("new puller: %v", err)
	}

	destDir := filepath.Join(t.TempDir(), "clone-dest")

	changed, err := p.CloneOrPull(context.Background(), bareRepo, "master", destDir)
	if err != nil {
		t.Fatalf("CloneOrPull (clone): %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true on fresh clone")
	}

	// Verify the file was cloned.
	data, err := os.ReadFile(filepath.Join(destDir, "README.md")) //nolint:gosec // G304: test reads known path
	if err != nil {
		t.Fatalf("read cloned file: %v", err)
	}
	if string(data) != "# test\n" {
		t.Fatalf("unexpected content: %q", data)
	}
}

func TestCloneOrPull_PullExisting(t *testing.T) {
	bareRepo := newBareRepoWithCommit(t)

	p, err := NewPuller(&AuthConfig{Method: AuthNone}, nil)
	if err != nil {
		t.Fatalf("new puller: %v", err)
	}

	destDir := filepath.Join(t.TempDir(), "pull-dest")

	// First clone.
	if _, err := p.CloneOrPull(context.Background(), bareRepo, "master", destDir); err != nil {
		t.Fatalf("initial clone: %v", err)
	}

	// Pull again — should be up-to-date.
	changed, err := p.CloneOrPull(context.Background(), bareRepo, "master", destDir)
	if err != nil {
		t.Fatalf("CloneOrPull (pull): %v", err)
	}
	if changed {
		t.Fatal("expected changed=false when already up-to-date")
	}
}

func TestCloneOrPull_ContextCancel(t *testing.T) {
	bareRepo := newBareRepoWithCommit(t)

	p, err := NewPuller(&AuthConfig{Method: AuthNone}, nil)
	if err != nil {
		t.Fatalf("new puller: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	destDir := filepath.Join(t.TempDir(), "cancel-dest")

	_, err = p.CloneOrPull(ctx, bareRepo, "master", destDir)
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
}

func TestCloneOrPull_PullWithNewCommit(t *testing.T) {
	bareRepo := newBareRepoWithCommit(t)

	p, err := NewPuller(&AuthConfig{Method: AuthNone}, nil)
	if err != nil {
		t.Fatalf("new puller: %v", err)
	}

	destDir := filepath.Join(t.TempDir(), "pull-new-commit")

	// Initial clone.
	if _, err := p.CloneOrPull(context.Background(), bareRepo, "master", destDir); err != nil {
		t.Fatalf("initial clone: %v", err)
	}

	// Push a second commit to the bare repo.
	addCommitToBareRepo(t, bareRepo, "second.txt", "new content\n")

	// Pull should detect the change.
	changed, err := p.CloneOrPull(context.Background(), bareRepo, "master", destDir)
	if err != nil {
		t.Fatalf("pull after new commit: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true after new commit was pushed")
	}

	// Verify the new file is present.
	data, err := os.ReadFile(filepath.Join(destDir, "second.txt")) //nolint:gosec // G304: test reads known path
	if err != nil {
		t.Fatalf("read new file: %v", err)
	}
	if string(data) != "new content\n" {
		t.Fatalf("unexpected content: %q", data)
	}
}

func TestCloneOrPull_PullFallbackReclone(t *testing.T) {
	bareRepo := newBareRepoWithCommit(t)

	p, err := NewPuller(&AuthConfig{Method: AuthNone}, nil)
	if err != nil {
		t.Fatalf("new puller: %v", err)
	}

	destDir := filepath.Join(t.TempDir(), "fallback-dest")

	// Initial clone.
	if _, err := p.CloneOrPull(context.Background(), bareRepo, "master", destDir); err != nil {
		t.Fatalf("initial clone: %v", err)
	}

	// Corrupt the repo to force a pull failure.
	if err := os.RemoveAll(filepath.Join(destDir, ".git", "objects")); err != nil {
		t.Fatalf("corrupt repo: %v", err)
	}

	// Push a new commit to the bare repo.
	addCommitToBareRepo(t, bareRepo, "new.txt", "content")

	// CloneOrPull should detect pull failure and fall back to re-clone.
	changed, err := p.CloneOrPull(context.Background(), bareRepo, "master", destDir)
	if err != nil {
		t.Fatalf("fallback re-clone failed: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true after re-clone")
	}

	// Verify the new file is present after re-clone.
	data, err := os.ReadFile(filepath.Join(destDir, "new.txt")) //nolint:gosec // G304: test reads known path
	if err != nil {
		t.Fatalf("read file after re-clone: %v", err)
	}
	if string(data) != "content" {
		t.Fatalf("unexpected content: %q", data)
	}
}

func TestSanitizeURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no creds", "https://github.com/org/repo.git", "https://github.com/org/repo.git"},
		{"with token", "https://token@github.com/org/repo.git", "https://github.com/org/repo.git"},
		{"with user+pass", "https://user:pass@github.com/org/repo.git", "https://github.com/org/repo.git"},
		{"local path", "/tmp/bare-repo", "/tmp/bare-repo"},
		{"ssh url", "git@github.com:org/repo.git", "git@github.com:org/repo.git"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeURL(tt.in)
			if got != tt.want {
				t.Errorf("sanitizeURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
