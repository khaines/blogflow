package config

import (
	"bytes"
	"io"
	"io/fs"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
)

// trackReader wraps an io.Reader and tracks total bytes consumed.
type trackReader struct {
	r     io.Reader
	count atomic.Int64
}

func (r *trackReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	r.count.Add(int64(n))
	return n, err
}

func (r *trackReader) Close() error               { return nil }
func (r *trackReader) Stat() (fs.FileInfo, error) { return nil, nil }

// trackFS is an fs.FS that serves a "site.yaml" whose content the loader tracks.
type trackFS struct {
	data  []byte       // the total content of site.yaml (may be larger than limiter)
	track *trackReader // set on Open()
}

func (t *trackFS) Open(name string) (fs.File, error) {
	if name == "site.yaml" {
		t.track = &trackReader{r: bytes.NewReader(t.data)}
		return t.track, nil
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

// TestLoad_BoundedRead_rejectOversized verifies that oversized config files
// are rejected *without* fully reading the file. io.LimitReader caps the bytes
// consumed from the underlying reader to maxConfigFileSize+1.
func TestLoad_BoundedRead_rejectOversized(t *testing.T) {
	t.Parallel()

	// Serve a 2 MB file. With bounded read, only ~1 MB should be consumed.
	big := bytes.Repeat([]byte("x"), 2*1024*1024)
	cfs := &trackFS{data: big}
	loader := NewLoader(cfs)
	_, err := loader.Load()
	if err == nil {
		t.Fatal("expected error for oversized config, got nil")
	}
	if !strings.Contains(err.Error(), "1 MB") {
		t.Fatalf("expected size-limit error mentioning '1 MB', got: %v", err)
	}

	// io.LimitReader caps bytes at maxConfigFileSize+1, so the underlying
	// reader should only be called for that many bytes.
	read := cfs.track.count.Load()
	if read > maxConfigFileSize+1 {
		t.Errorf("bounded read leaked: reader consumed %d bytes (limit ~%d)", read, maxConfigFileSize)
	}
}

// TestLoad_BoundedRead_boundary verifies that exactly maxConfigFileSize bytes
// succeeds while maxConfigFileSize+1 bytes is rejected.
func TestLoad_BoundedRead_boundary(t *testing.T) {
	t.Parallel()

	// Build a valid config that fits within maxConfigFileSize.
	yaml := []byte("site:\n  title: \"Boundary Test\"\n  base_url: \"http://localhost:8080\"\n")

	// Exactly maxConfigFileSize: pad with spaces to fill (spaces are valid YAML).
	exact := bytes.Repeat([]byte(" "), maxConfigFileSize)
	copy(exact, yaml)
	cfs1 := &trackFS{data: exact}
	loader1 := NewLoader(cfs1)
	_, err := loader1.Load()
	if err != nil {
		t.Fatalf("expected valid config at exactly %d bytes, got: %v", maxConfigFileSize, err)
	}

	// maxConfigFileSize+1: should fail with the size-limit error.
	over := bytes.Repeat([]byte(" "), maxConfigFileSize+1)
	copy(over, yaml)
	cfs2 := &trackFS{data: over}
	loader2 := NewLoader(cfs2)
	_, err = loader2.Load()
	if err == nil {
		t.Fatal("expected size-limit error at maxConfigFileSize+1 bytes")
	}
}

// TestLoad_FileSizeLimit_existing verifies the existing behavior still
// rejects files over 1 MB when using fstest.MapFS (for regression).
func TestLoad_FileSizeLimit_existing(t *testing.T) {
	t.Parallel()

	bigData := make([]byte, 2*1024*1024)
	for i := range bigData {
		bigData[i] = ' '
	}
	fsys := fstest.MapFS{
		"site.yaml": &fstest.MapFile{Data: bigData},
	}
	loader := NewLoader(fsys)
	_, err := loader.Load()
	if err == nil {
		t.Fatal("expected error for oversized config, got nil")
	}
	if !strings.Contains(err.Error(), "1 MB") {
		t.Fatalf("expected size-limit error mentioning '1 MB', got: %v", err)
	}
}
