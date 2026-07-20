package theme

import (
	"io"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/khaines/blogflow/internal/overlayfs"
)

const testMaxTemplateReadSize = 64 * 1024 * 1024

func TestNewEngineRejectsOversizedTemplateBeforeParse(t *testing.T) {
	t.Parallel()

	oversized := oversizedTemplateFS{
		path: "templates/oversized.html",
		size: testMaxTemplateReadSize + 1,
	}
	defaults := fstest.MapFS{
		"templates/base.html": &fstest.MapFile{Data: []byte(`<!DOCTYPE html>{{block "content" .}}{{end}}`)},
	}
	fsys := overlayfs.NewOverlayFS(oversized, defaults).WithLayerNames([]string{"theme", "defaults"})

	_, err := NewEngine(fsys)
	if err == nil {
		t.Fatal("NewEngine returned nil error for oversized template")
	}

	msg := err.Error()
	if !strings.Contains(msg, "exceeds maximum read size") {
		t.Fatalf("NewEngine error = %v, want size-limit rejection", err)
	}
	if strings.Contains(msg, "parsing template") {
		t.Fatalf("NewEngine error = %v, want rejection before parsing", err)
	}
}

type oversizedTemplateFS struct {
	path string
	size int64
}

func (f oversizedTemplateFS) Open(name string) (fs.File, error) {
	if name != f.path {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &syntheticTemplateFile{
		name:      name,
		remaining: f.size,
	}, nil
}

func (f oversizedTemplateFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name != "templates" {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	return []fs.DirEntry{
		syntheticDirEntry{
			name: strings.TrimPrefix(f.path, "templates/"),
			size: f.size,
		},
	}, nil
}

type syntheticTemplateFile struct {
	name      string
	remaining int64
}

func (f *syntheticTemplateFile) Stat() (fs.FileInfo, error) {
	return syntheticFileInfo{name: f.name, size: f.remaining}, nil
}

func (f *syntheticTemplateFile) Read(p []byte) (int, error) {
	if f.remaining <= 0 {
		return 0, io.EOF
	}
	n := min(int64(len(p)), f.remaining)
	for i := int64(0); i < n; i++ {
		p[i] = '{'
	}
	f.remaining -= n
	return int(n), nil
}

func (f *syntheticTemplateFile) Close() error {
	return nil
}

type syntheticDirEntry struct {
	name string
	size int64
}

func (e syntheticDirEntry) Name() string {
	return e.name
}

func (e syntheticDirEntry) IsDir() bool {
	return false
}

func (e syntheticDirEntry) Type() fs.FileMode {
	return 0
}

func (e syntheticDirEntry) Info() (fs.FileInfo, error) {
	return syntheticFileInfo(e), nil
}

type syntheticFileInfo struct {
	name string
	size int64
}

func (i syntheticFileInfo) Name() string {
	return i.name
}

func (i syntheticFileInfo) Size() int64 {
	return i.size
}

func (i syntheticFileInfo) Mode() fs.FileMode {
	return 0
}

func (i syntheticFileInfo) ModTime() time.Time {
	return time.Time{}
}

func (i syntheticFileInfo) IsDir() bool {
	return false
}

func (i syntheticFileInfo) Sys() any {
	return nil
}
