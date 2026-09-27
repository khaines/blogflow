package overlayfs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newGitContentLayer builds a content directory that looks like a git clone
// with a .git/config holding a secret, plus a symlink under static/ that
// points into .git.
func newGitContentLayer(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{".git", "static", "posts"} {
		if err := os.Mkdir(filepath.Join(root, d), 0o750 /* test fixture */); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "config"), []byte("url = https://u:SECRET@host/r.git\n"), 0o600 /* test fixture */); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "static", "ok.css"), []byte("body{}"), 0o600 /* test fixture */); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../.git/config", filepath.Join(root, "static", "leak.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../.git", filepath.Join(root, "static", "gitdir")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../.git/config", filepath.Join(root, "posts", "leak.md")); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestOverlayFS_RefusesGitDirectoryViaSymlink(t *testing.T) {
	t.Parallel()

	ofs, err := NewFromPaths("", newGitContentLayer(t), "", nil)
	if err != nil {
		t.Fatalf("NewFromPaths: %v", err)
	}

	for _, name := range []string{"static/leak.txt", "static/gitdir/config", "posts/leak.md", ".git/config"} {
		if f, err := ofs.Open(name); err == nil {
			_ = f.Close()
			t.Errorf("Open(%q) succeeded; want refusal", name)
		} else if !errors.Is(err, fs.ErrPermission) {
			t.Errorf("Open(%q) error = %v; want fs.ErrPermission", name, err)
		}
		if _, err := fs.ReadFile(ofs, name); !errors.Is(err, fs.ErrPermission) {
			t.Errorf("ReadFile(%q) error = %v; want fs.ErrPermission", name, err)
		}
		if _, err := fs.Stat(ofs, name); !errors.Is(err, fs.ErrPermission) {
			t.Errorf("Stat(%q) error = %v; want fs.ErrPermission", name, err)
		}
	}

	// Paths naming .git are refused by name, before any layer lookup, so
	// the answer does not depend on case sensitivity or on existence.
	for _, name := range []string{".GIT/config", ".git/missing", "posts/.Git/HEAD"} {
		if _, err := ofs.Open(name); !errors.Is(err, fs.ErrPermission) {
			t.Errorf("Open(%q) error = %v; want fs.ErrPermission", name, err)
		}
		if _, err := fs.ReadFile(ofs, name); !errors.Is(err, fs.ErrPermission) {
			t.Errorf("ReadFile(%q) error = %v; want fs.ErrPermission", name, err)
		}
		if _, err := fs.Stat(ofs, name); !errors.Is(err, fs.ErrPermission) {
			t.Errorf("Stat(%q) error = %v; want fs.ErrPermission", name, err)
		}
		if _, err := ofs.Resolve(name); !errors.Is(err, fs.ErrPermission) {
			t.Errorf("Resolve(%q) error = %v; want fs.ErrPermission", name, err)
		}
	}

	for _, dir := range []string{".git", "static/gitdir"} {
		if entries, err := fs.ReadDir(ofs, dir); err == nil {
			t.Errorf("ReadDir(%q) listed %d entries; want refusal", dir, len(entries))
		} else if !errors.Is(err, fs.ErrPermission) {
			t.Errorf("ReadDir(%q) error = %v; want fs.ErrPermission", dir, err)
		}
	}

	// The root listing does not advertise .git.
	root, err := fs.ReadDir(ofs, ".")
	if err != nil {
		t.Fatalf("ReadDir(.): %v", err)
	}
	for _, e := range root {
		if e.Name() == ".git" {
			t.Error("ReadDir(.) lists .git")
		}
	}

	// A directory handle from Open (what http.FileServerFS lists) hides
	// .git too.
	f, err := ofs.Open(".")
	if err != nil {
		t.Fatalf("Open(.): %v", err)
	}
	defer func() { _ = f.Close() }()
	d, ok := f.(fs.ReadDirFile)
	if !ok {
		t.Fatalf("Open(.) returned %T, want fs.ReadDirFile", f)
	}
	for {
		batch, err := d.ReadDir(1)
		for _, e := range batch {
			if strings.EqualFold(e.Name(), ".git") {
				t.Error("directory handle from Open lists .git")
			}
		}
		if err != nil {
			break
		}
	}

	// Regular files are still served.
	if _, err := fs.ReadFile(ofs, "static/ok.css"); err != nil {
		t.Fatalf("ReadFile(static/ok.css): %v", err)
	}
}

func TestHasGitComponent(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]bool{
		".git":            true,
		".git/config":     true,
		"a/.GIT/HEAD":     true,
		"static/.gitkeep": false,
		"posts/git.md":    false,
		"a/b/c":           false,
	} {
		if got := hasGitComponent(name); got != want {
			t.Errorf("hasGitComponent(%q) = %v, want %v", name, got, want)
		}
	}
}
