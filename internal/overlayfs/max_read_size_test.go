// Large file size enforcement per test-gap-analysis.md item #15
// Tests that files exceeding 64MB threshold are rejected.
package overlayfs

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

func TestLargeFileRejection(t *testing.T) {
	t.Parallel()
	// Verify the default layer respects maxReadSize by creating a large virtual file.
	bigData := make([]byte, maxReadSize+1) //nolint:gosec // test only — bounded by config constant
	fsys := fstest.MapFS{
		"bigfile.txt": &fstest.MapFile{Data: bigData},
	}
	of := NewOverlayFS(fsys).WithLayerNames([]string{"defaults"})
	_, err := fs.ReadFile(of, "bigfile.txt")
	if err == nil {
		t.Errorf("expected error reading file exceeding maxReadSize, got nil")
	} else {
		t.Logf("correctly rejected oversized file: %v", err)
	}
}
