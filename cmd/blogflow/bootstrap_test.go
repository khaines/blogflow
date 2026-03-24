package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/khaines/blogflow/internal/config"
)

// newLocalBareRepo creates a bare repo with one commit on "master" for testing.
func newLocalBareRepo(t *testing.T) string {
	t.Helper()

	srcDir := filepath.Join(t.TempDir(), "src")
	repo, err := git.PlainInit(srcDir, false)
	if err != nil {
		t.Fatalf("init source repo: %v", err)
	}

	postContent := "---\ntitle: Bootstrap Test\ndate: 2024-01-01T00:00:00Z\n---\nBootstrap content\n"
	if err := os.MkdirAll(filepath.Join(srcDir, "posts"), 0o750); err != nil {
		t.Fatalf("mkdir posts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "posts", "test.md"), []byte(postContent), 0o600); err != nil {
		t.Fatalf("write post: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	if _, err := wt.Add("posts/test.md"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := wt.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "test",
			Email: "test@test.com",
			When:  time.Now(),
		},
	}); err != nil {
		t.Fatalf("commit: %v", err)
	}

	bareDir := filepath.Join(t.TempDir(), "bare")
	if _, err := git.PlainClone(bareDir, true, &git.CloneOptions{URL: srcDir}); err != nil {
		t.Fatalf("bare clone: %v", err)
	}

	return bareDir
}

func TestBootstrapContent_ClonesRepo(t *testing.T) {
	t.Parallel()

	bareRepo := newLocalBareRepo(t)
	destDir := filepath.Join(t.TempDir(), "content")

	cfg := config.Default()
	cfg.Sync.Repo = bareRepo
	cfg.Sync.Branch = "master"

	logger := slog.Default()

	puller, branch, dest := bootstrapContent(cfg, destDir, logger)
	if puller == nil {
		t.Fatal("expected non-nil puller after successful bootstrap")
	}
	if branch != "master" {
		t.Errorf("expected branch %q, got %q", "master", branch)
	}
	if dest != destDir {
		t.Errorf("expected dest %q, got %q", destDir, dest)
	}

	// Verify the cloned content exists.
	postPath := filepath.Join(destDir, "posts", "test.md")
	data, err := os.ReadFile(postPath) //nolint:gosec // G304: test reads known path
	if err != nil {
		t.Fatalf("expected cloned post file at %s: %v", postPath, err)
	}
	if len(data) == 0 {
		t.Fatal("cloned post file is empty")
	}
}

func TestBootstrapContent_CloneFailureIsNonFatal(t *testing.T) {
	t.Parallel()

	destDir := filepath.Join(t.TempDir(), "content")

	cfg := config.Default()
	cfg.Sync.Repo = "https://invalid.example.com/nonexistent/repo.git"
	cfg.Sync.Branch = "main"

	logger := slog.Default()

	// Should not panic — clone failure is logged but non-fatal.
	puller, _, _ := bootstrapContent(cfg, destDir, logger)
	// Puller may be non-nil (auth succeeded) even if clone failed, or nil if auth failed.
	_ = puller

	// Dest dir may or may not exist; the important thing is no panic/crash.
	if _, err := os.Stat(filepath.Join(destDir, ".git")); err == nil {
		t.Error("did not expect a .git dir after a failed clone")
	}
}

func TestBootstrapContent_DefaultBranch(t *testing.T) {
	t.Parallel()

	bareRepo := newLocalBareRepo(t)
	destDir := filepath.Join(t.TempDir(), "content")

	cfg := config.Default()
	cfg.Sync.Repo = bareRepo
	cfg.Sync.Branch = "" // should default to "main"

	logger := slog.Default()

	// "main" branch doesn't exist in our test repo (it uses "master"),
	// so this should fail gracefully.
	_, branch, _ := bootstrapContent(cfg, destDir, logger)
	if branch != "main" {
		t.Errorf("expected default branch %q, got %q", "main", branch)
	}

	// Non-fatal: server would continue.
}

func TestBootstrapContent_DefaultContentPath(t *testing.T) {
	// NOT parallel: os.Chdir is process-global.

	bareRepo := newLocalBareRepo(t)

	// Use a temp dir as the working directory to avoid polluting the real cwd.
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	cfg := config.Default()
	cfg.Sync.Repo = bareRepo
	cfg.Sync.Branch = "master"

	logger := slog.Default()

	// Empty contentPath should default to "content".
	_, _, dest := bootstrapContent(cfg, "", logger)
	if dest != "content" {
		t.Errorf("expected default dest %q, got %q", "content", dest)
	}

	postPath := filepath.Join(tmpDir, "content", "posts", "test.md")
	if _, err := os.Stat(postPath); err != nil {
		t.Fatalf("expected cloned post at default content path %s: %v", postPath, err)
	}
}

func TestBootstrapContent_PullExisting(t *testing.T) {
	t.Parallel()

	bareRepo := newLocalBareRepo(t)
	destDir := filepath.Join(t.TempDir(), "content")

	cfg := config.Default()
	cfg.Sync.Repo = bareRepo
	cfg.Sync.Branch = "master"

	logger := slog.Default()

	// First clone.
	bootstrapContent(cfg, destDir, logger) //nolint:dogsled

	// Second call should pull (not re-clone) and succeed.
	bootstrapContent(cfg, destDir, logger) //nolint:dogsled

	postPath := filepath.Join(destDir, "posts", "test.md")
	if _, err := os.Stat(postPath); err != nil {
		t.Fatalf("expected post file after pull: %v", err)
	}
}
