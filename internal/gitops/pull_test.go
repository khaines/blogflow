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
	if err := os.WriteFile(filepath.Join(srcDir, "README.md"), []byte("# test\n"), 0o644); err != nil {
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
	data, err := os.ReadFile(filepath.Join(destDir, "README.md"))
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
