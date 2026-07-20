// Large file size enforcement per test-gap-analysis.md item #15
// Tests that files exceeding 64MB threshold are rejected.
package overlayfs

import (
	"errors"
	"io"
	"io/fs"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"
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

func TestReadFileBoundsPayloadWhenStatReportsSmallSize(t *testing.T) {
	fsys := &smallStatHugePayloadFS{
		name: "growing.txt",
		size: maxReadSize + 1,
	}
	of := NewOverlayFS(fsys).WithLayerNames([]string{"mutable"})

	_, err := fs.ReadFile(of, "growing.txt")
	if err == nil {
		t.Fatal("expected size-limit error, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds maximum read size") {
		t.Fatalf("ReadFile error = %v, want size-limit rejection", err)
	}
	if fsys.readFileCalled.Load() {
		t.Fatal("ReadFileFS.ReadFile was called; want bounded Open/readAll payload read")
	}
}

type smallStatHugePayloadFS struct {
	name           string
	size           int64
	readFileCalled atomic.Bool
}

func (f *smallStatHugePayloadFS) Open(name string) (fs.File, error) {
	if name != f.name {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &streamingFile{name: name, remaining: f.size}, nil
}

func (f *smallStatHugePayloadFS) ReadFile(string) ([]byte, error) {
	f.readFileCalled.Store(true)
	return nil, errors.New("unbounded ReadFileFS.ReadFile should not be called")
}

func (f *smallStatHugePayloadFS) Stat(name string) (fs.FileInfo, error) {
	if name != f.name {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}
	return streamingFileInfo{name: name, size: 10}, nil
}

type streamingFile struct {
	name      string
	remaining int64
}

func (f *streamingFile) Stat() (fs.FileInfo, error) {
	return streamingFileInfo{name: f.name, size: 10}, nil
}

func (f *streamingFile) Read(p []byte) (int, error) {
	if f.remaining <= 0 {
		return 0, io.EOF
	}
	n := min(int64(len(p)), f.remaining)
	for i := int64(0); i < n; i++ {
		p[i] = 'x'
	}
	f.remaining -= n
	return int(n), nil
}

func (f *streamingFile) Close() error {
	return nil
}

type streamingFileInfo struct {
	name string
	size int64
}

func (i streamingFileInfo) Name() string {
	return i.name
}

func (i streamingFileInfo) Size() int64 {
	return i.size
}

func (i streamingFileInfo) Mode() fs.FileMode {
	return 0
}

func (i streamingFileInfo) ModTime() time.Time {
	return time.Time{}
}

func (i streamingFileInfo) IsDir() bool {
	return false
}

func (i streamingFileInfo) Sys() any {
	return nil
}
