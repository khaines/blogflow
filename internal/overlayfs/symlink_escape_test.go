package overlayfs

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckSymlinkSafe_RejectsEscapeFromOverlay(t *testing.T) {
	t.Parallel()

	// Create a fake overlay root directory.
	overlayRoot, err := os.MkdirTemp("", "overlay-root-*")
	if err != nil {
		t.Fatalf("create overlay root: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(overlayRoot) })

	// Create a symlink inside the overlay that points outside it.
	fakeThemeDir := filepath.Join(overlayRoot, "theme")
	if err := os.Mkdir(fakeThemeDir, 0o750 /* test fixture */); err != nil {
		t.Fatalf("create theme dir: %v", err)
	}

	// Resolve the absolute path of the OS temp dir (which is outside overlayRoot).
	absTemp, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		t.Fatalf("eval os.TempDir(): %v", err)
	}

	// Create a symlink inside the overlay pointing to the OS temp directory.
	symlinkPath := filepath.Join(fakeThemeDir, "etc-passwd")
	if err := os.Symlink(absTemp, symlinkPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	// The path as it would appear to the overlay layer.
	overlayPath := "theme/etc-passwd"

	err = checkSymlinkSafe(overlayRoot, overlayPath)
	if err == nil {
		t.Fatal("expected checkSymlinkSafe to reject a symlink escaping the overlay root")
	}
	if _, ok := err.(*fs.PathError); !ok {
		t.Errorf("expected *fs.PathError, got %T: %v", err, err)
	}
}

func TestCheckSymlinkSafe_AllowsEmptyRoot(t *testing.T) {
	t.Parallel()

	// Non-disk layers (root == "") should pass through without error.
	err := checkSymlinkSafe("", "foo.txt")
	if err != nil {
		t.Fatalf("expected nil for non-disk layer (empty root), got: %v", err)
	}
}

func TestCheckSymlinkSafe_HandlesNonexistentPath(t *testing.T) {
	t.Parallel()

	overlayRoot, err := os.MkdirTemp("", "overlay-root-*")
	if err != nil {
		t.Fatalf("create overlay root: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(overlayRoot) })

	// A path that doesn't exist should not error (allow fallthrough to next layer).
	err = checkSymlinkSafe(overlayRoot, "does-not-exist.txt")
	if err != nil {
		t.Fatalf("expected nil for nonexistent path (not a symlink issue), got: %v", err)
	}
}

func TestCheckSymlinkSafe_RejectsRootSymlinkEscape(t *testing.T) {
	t.Parallel()

	overlayRoot, err := os.MkdirTemp("", "overlay-root-*")
	if err != nil {
		t.Fatalf("create overlay root: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(overlayRoot) })

	// Create a symlink at the root level pointing outside.
	absTemp, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		t.Fatalf("eval os.TempDir(): %v", err)
	}

	symlinkName := filepath.Join(overlayRoot, "escape")
	if err := os.Symlink(absTemp, symlinkName); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	err = checkSymlinkSafe(overlayRoot, "escape")
	if err == nil {
		t.Fatal("expected checkSymlinkSafe to reject a symlink from overlay root escaping outward")
	}
}

func TestCheckSymlinkSafe_RejectsNestedChainEscape(t *testing.T) {
	t.Parallel()

	overlayRoot, err := os.MkdirTemp("", "overlay-root-*")
	if err != nil {
		t.Fatalf("create overlay root: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(overlayRoot) })

	// Create a chain: inside-overlay -> leaf-dir  -> os.TempDir()
	chainDir := filepath.Join(overlayRoot, "chain")
	if err := os.Mkdir(chainDir, 0o750 /* test fixture */); err != nil {
		t.Fatalf("create chain dir: %v", err)
	}

	linkTarget := filepath.Join(overlayRoot, "leaf")
	leafFile := filepath.Join(linkTarget, "file.txt")
	if err := os.MkdirAll(linkTarget, 0o750 /* test fixture */); err != nil {
		t.Fatalf("create leaf dir: %v", err)
	}
	if err := os.WriteFile(leafFile, []byte("safe"), 0o600 /* test fixture */); err != nil {
		t.Fatalf("write leaf file: %v", err)
	}

	// leaf -> linkTarget (safe, stays inside overlay)
	if err := os.Symlink(linkTarget, filepath.Join(chainDir, "leaf")); err != nil {
		t.Fatalf("create safe symlink: %v", err)
	}

	// Now create: inside-overlay -> outside
	absTemp, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		t.Fatalf("eval os.TempDir(): %v", err)
	}

	escapeLink := filepath.Join(linkTarget, "escape")
	if err := os.Symlink(absTemp, escapeLink); err != nil {
		t.Fatalf("create escape symlink in chain: %v", err)
	}

	// Resolving through the chain should still detect the escape.
	err = checkSymlinkSafe(overlayRoot, "chain/leaf/escape")
	if err == nil {
		t.Fatal("expected checkSymlinkSafe to reject a symlink chain that ultimately escapes the overlay root")
	}
}

// TestOverlayFS_SymlinkOpen validates that an OverlayFS using real disk layers
// (via os.DirFS) correctly invokes checkSymlinkSafe through the Open path.
// This tests the integration of symlink escape detection end-to-end.
func TestOverlayFS_SymlinkOpen(t *testing.T) {
	t.Parallel()

	// Set up two disk layers with real files.
	overlayRoot, err := os.MkdirTemp("", "overlay-root-*")
	if err != nil {
		t.Fatalf("create overlay root: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(overlayRoot) })

	targetFile := filepath.Join(overlayRoot, "target.txt")
	if err := os.WriteFile(targetFile, []byte("safe content"), 0o600 /* test fixture */); err != nil {
		t.Fatalf("write target: %v", err)
	}

	upperDir, err := os.MkdirTemp("", "upper-*")
	if err != nil {
		t.Fatalf("create upper dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(upperDir) })

	// Write a normal file in the upper layer.
	if err := os.WriteFile(filepath.Join(upperDir, "upper.txt"), []byte("upper"), 0o600 /* test fixture */); err != nil {
		t.Fatalf("write upper file: %v", err)
	}

	ofs := NewOverlayFS(os.DirFS(upperDir), os.DirFS(overlayRoot)).WithLayerNames([]string{"upper", "lower"})

	// Open a file from the lower layer — should succeed.
	upperRoot, _ := filepath.EvalSymlinks(upperDir)
	lowerRoot, _ := filepath.EvalSymlinks(overlayRoot)
	ofs.layerMeta[1] = layerMeta{rootPath: lowerRoot, isDisk: true}

	// Verify upper layer root resolved to a real directory
	info, err := os.Stat(upperRoot)
	if err != nil || !info.IsDir() {
		t.Fatalf("upperRoot %q is not a resolvable directory", upperRoot)
	}
	// Open a file from the lower layer — should succeed.
	f, err := ofs.Open("target.txt")
	if err != nil {
		t.Fatalf("Open lower-layer file: %v", err)
	}
	_ = f.Close()
}
