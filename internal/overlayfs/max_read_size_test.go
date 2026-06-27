// Large file size enforcement per test-gap-analysis.md item #15
// Tests that files exceeding 64MB threshold are rejected.
package overlayfs

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLargeFileRejection(t *testing.T) {
	t.Parallel()
	bigData := make([]byte, maxReadSize+1) //nolint:gosec // test only — bounded by known constant
	fsys := fstest.MapFS{
		"bigfile.txt": &fstest.MapFile{Data: bigData},
	}
	of := NewOverlayFS(fsys).WithLayerNames([]string{"defaults"})
	_, err := fs.ReadFile(of, "bigfile.txt")
	if err == nil {
		t.Fatal("expected error reading file exceeding maxReadSize, got nil")
	}
	wantErr := "exceeds maximum read size"
	if !strings.Contains(err.Error(), wantErr) {
		t.Errorf("error should contain %q; got: %q", wantErr, err.Error())
	}
}

func TestLargeFileAtBoundary(t *testing.T) {
	t.Parallel()
	okData := make([]byte, maxReadSize) //nolint:gosec // test only
	fsys := fstest.MapFS{
		"okfile.txt": &fstest.MapFile{Data: okData},
	}
	of := NewOverlayFS(fsys).WithLayerNames([]string{"defaults"})
	data, err := fs.ReadFile(of, "okfile.txt")
	if err != nil {
		t.Fatalf("expected file at maxReadSize to succeed, got error: %v", err)
	}
	if len(data) != maxReadSize {
		t.Errorf("read %d bytes, want %d", len(data), maxReadSize)
	}
}
