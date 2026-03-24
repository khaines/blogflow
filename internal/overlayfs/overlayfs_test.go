package overlayfs

import (
	"errors"
	"io/fs"
	"sync"
	"testing"
	"testing/fstest"
)

func newTestOverlay(layers ...fs.FS) *OverlayFS {
	names := make([]string, len(layers))
	for i := range layers {
		names[i] = layerName(i)
	}
	return NewOverlayFS(layers...).WithLayerNames(names)
}

func layerName(i int) string {
	names := []string{"theme", "content", "config", "defaults"}
	if i < len(names) {
		return names[i]
	}
	return "unknown"
}

// §3.1 #1: Open file from highest-priority layer
func TestOpen_TopLayer(t *testing.T) {
	theme := fstest.MapFS{"templates/post.html": {Data: []byte("theme-post")}}
	defaults := fstest.MapFS{"templates/post.html": {Data: []byte("default-post")}}

	ofs := newTestOverlay(theme, defaults)
	data, err := fs.ReadFile(ofs, "templates/post.html")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "theme-post" {
		t.Errorf("got %q, want %q", data, "theme-post")
	}
}

// §3.1 #2: Open file from defaults (fallback)
func TestOpen_Fallback(t *testing.T) {
	theme := fstest.MapFS{}
	defaults := fstest.MapFS{"templates/base.html": {Data: []byte("default-base")}}

	ofs := newTestOverlay(theme, defaults)
	data, err := fs.ReadFile(ofs, "templates/base.html")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "default-base" {
		t.Errorf("got %q, want %q", data, "default-base")
	}
}

// §3.1 #3: Open file from middle layer
func TestOpen_MiddleLayer(t *testing.T) {
	theme := fstest.MapFS{}
	content := fstest.MapFS{}
	config := fstest.MapFS{"site.yaml": {Data: []byte("config-site")}}
	defaults := fstest.MapFS{"site.yaml": {Data: []byte("default-site")}}

	ofs := newTestOverlay(theme, content, config, defaults)
	data, err := fs.ReadFile(ofs, "site.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "config-site" {
		t.Errorf("got %q, want %q", data, "config-site")
	}
}

// §3.1 #5: ReadDir merges entries (union)
func TestReadDir_Union(t *testing.T) {
	theme := fstest.MapFS{
		"templates/a.html": {Data: []byte("a")},
		"templates/b.html": {Data: []byte("theme-b")},
	}
	defaults := fstest.MapFS{
		"templates/b.html": {Data: []byte("default-b")},
		"templates/c.html": {Data: []byte("c")},
	}

	ofs := newTestOverlay(theme, defaults)
	entries, err := fs.ReadDir(ofs, "templates")
	if err != nil {
		t.Fatal(err)
	}

	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}

	// Union: a.html (theme), b.html (theme wins), c.html (defaults)
	want := []string{"a.html", "b.html", "c.html"}
	if len(names) != len(want) {
		t.Fatalf("got %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("entry[%d] = %q, want %q", i, names[i], want[i])
		}
	}
}

// §3.1 #6: ReadDir returns sorted entries
func TestReadDir_Sorted(t *testing.T) {
	layer := fstest.MapFS{
		"dir/zebra.html":  {Data: []byte("z")},
		"dir/alpha.html":  {Data: []byte("a")},
		"dir/middle.html": {Data: []byte("m")},
	}

	ofs := newTestOverlay(layer)
	entries, err := fs.ReadDir(ofs, "dir")
	if err != nil {
		t.Fatal(err)
	}

	for i := 1; i < len(entries); i++ {
		if entries[i].Name() < entries[i-1].Name() {
			t.Errorf("not sorted: %q after %q", entries[i].Name(), entries[i-1].Name())
		}
	}
}

// §3.1 #9: NewFromPaths with empty paths → only defaults
func TestNewOverlayFS_EmptyPaths(t *testing.T) {
	defaults := fstest.MapFS{"test.txt": {Data: []byte("hello")}}
	ofs := newTestOverlay(defaults)

	if ofs.LayerCount() != 1 {
		t.Errorf("LayerCount() = %d, want 1", ofs.LayerCount())
	}
	data, err := fs.ReadFile(ofs, "test.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Errorf("got %q, want %q", data, "hello")
	}
}

// §3.1 #10: resolveInfo identifies layer
func TestResolveInfo(t *testing.T) {
	theme := fstest.MapFS{}
	content := fstest.MapFS{"posts/hello.md": {Data: []byte("# Hello")}}
	defaults := fstest.MapFS{}

	ofs := newTestOverlay(theme, content, defaults)
	res, err := ofs.resolveInfo("posts/hello.md")
	if err != nil {
		t.Fatal(err)
	}
	if res.LayerIndex != 1 {
		t.Errorf("LayerIndex = %d, want 1", res.LayerIndex)
	}
	if res.LayerName != "content" {
		t.Errorf("LayerName = %q, want %q", res.LayerName, "content")
	}
}

// §3.1 #11: ReadDir root directory
func TestReadDir_Root(t *testing.T) {
	theme := fstest.MapFS{"a.txt": {Data: []byte("a")}}
	defaults := fstest.MapFS{"b.txt": {Data: []byte("b")}}

	ofs := newTestOverlay(theme, defaults)
	entries, err := fs.ReadDir(ofs, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("got %d entries, want 2", len(entries))
	}
}

// §3.2 #1: File not found in any layer
func TestOpen_NotFound(t *testing.T) {
	ofs := newTestOverlay(fstest.MapFS{})
	_, err := ofs.Open("nonexistent.txt")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !isNotExist(err) {
		t.Errorf("expected ErrNotExist, got %v", err)
	}
}

// §3.2 #2: Path traversal with ..
func TestOpen_PathTraversal(t *testing.T) {
	ofs := newTestOverlay(fstest.MapFS{})
	_, err := ofs.Open("../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

// §3.2 #3: Absolute path
func TestOpen_AbsolutePath(t *testing.T) {
	ofs := newTestOverlay(fstest.MapFS{})
	_, err := ofs.Open("/etc/passwd")
	if err == nil {
		t.Fatal("expected error for absolute path")
	}
}

// §3.2 #5: Path with backslash
func TestOpen_Backslash(t *testing.T) {
	ofs := newTestOverlay(fstest.MapFS{})
	_, err := ofs.Open("templates\\post.html")
	if err == nil {
		t.Fatal("expected error for backslash path")
	}
}

// §3.2 #6: Empty path
func TestOpen_EmptyPath(t *testing.T) {
	ofs := newTestOverlay(fstest.MapFS{})
	_, err := ofs.Open("")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

// §3.2 #7: Dot path (root)
func TestOpen_DotPath(t *testing.T) {
	layer := fstest.MapFS{"file.txt": {Data: []byte("data")}}
	ofs := newTestOverlay(layer)
	f, err := ofs.Open(".")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
}

// §3.2 #9: ReadDir when some layers lack directory
func TestReadDir_PartialLayers(t *testing.T) {
	theme := fstest.MapFS{
		"partials/header.html": {Data: []byte("theme-header")},
	}
	// content has no partials dir
	content := fstest.MapFS{}
	defaults := fstest.MapFS{
		"partials/header.html": {Data: []byte("default-header")},
		"partials/footer.html": {Data: []byte("default-footer")},
	}

	ofs := newTestOverlay(theme, content, defaults)
	entries, err := fs.ReadDir(ofs, "partials")
	if err != nil {
		t.Fatal(err)
	}

	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}

	// Theme contributes header (overrides default), defaults contribute footer
	if len(names) != 2 {
		t.Fatalf("got %v, want 2 entries", names)
	}
}

// §3.2 #10: Nil layer in constructor
func TestNewOverlayFS_NilLayers(t *testing.T) {
	defaults := fstest.MapFS{"test.txt": {Data: []byte("hello")}}
	ofs := NewOverlayFS(nil, defaults, nil).WithLayerNames([]string{"nil1", "defaults", "nil2"})

	if ofs.LayerCount() != 1 {
		t.Errorf("LayerCount() = %d, want 1 (nil layers should be skipped)", ofs.LayerCount())
	}
}

// §3.2 #11: Zero layers
func TestNewOverlayFS_ZeroLayers(t *testing.T) {
	ofs := NewOverlayFS()
	_, err := ofs.Open("anything.txt")
	if err == nil {
		t.Fatal("expected error with zero layers")
	}
}

// §3.2 #12: Concurrent reads
func TestOpen_Concurrent(t *testing.T) {
	layer := fstest.MapFS{"file.txt": {Data: []byte("data")}}
	ofs := newTestOverlay(layer)

	done := make(chan struct{})
	for i := 0; i < 100; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			f, err := ofs.Open("file.txt")
			if err != nil {
				t.Error(err)
				return
			}
			_ = f.Close()
		}()
	}
	for i := 0; i < 100; i++ {
		<-done
	}
}

// §3.2 #13: Concurrent read + invalidation
func TestOpen_ConcurrentWithInvalidation(t *testing.T) {
	layer := fstest.MapFS{"file.txt": {Data: []byte("data")}}
	ofs := newTestOverlay(layer)

	done := make(chan struct{})
	// Readers
	for i := 0; i < 50; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 100; j++ {
				f, err := ofs.Open("file.txt")
				if err == nil {
					_ = f.Close()
				}
			}
		}()
	}
	// Invalidators
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 100; j++ {
				ofs.InvalidateAll()
			}
		}()
	}
	for i := 0; i < 60; i++ {
		<-done
	}
}

// OpenFile returns consistent handle + info
func TestOpenFile(t *testing.T) {
	layer := fstest.MapFS{"file.txt": {Data: []byte("hello world")}}
	ofs := newTestOverlay(layer)

	f, info, err := ofs.OpenFile("file.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	if info.Name() != "file.txt" {
		t.Errorf("Name = %q, want %q", info.Name(), "file.txt")
	}
	if info.Size() != 11 {
		t.Errorf("Size = %d, want 11", info.Size())
	}
}

// ReplaceLayer works correctly
func TestReplaceLayer(t *testing.T) {
	old := fstest.MapFS{"file.txt": {Data: []byte("old")}}
	ofs := newTestOverlay(old)

	data, _ := fs.ReadFile(ofs, "file.txt")
	if string(data) != "old" {
		t.Fatalf("before replace: got %q", data)
	}

	newLayer := fstest.MapFS{"file.txt": {Data: []byte("new")}}
	if err := ofs.ReplaceLayer(0, newLayer); err != nil {
		t.Fatal(err)
	}

	data, _ = fs.ReadFile(ofs, "file.txt")
	if string(data) != "new" {
		t.Errorf("after replace: got %q, want %q", data, "new")
	}
}

// ReplaceLayer out of range
func TestReplaceLayer_OutOfRange(t *testing.T) {
	ofs := newTestOverlay(fstest.MapFS{})
	err := ofs.ReplaceLayer(5, fstest.MapFS{})
	if err == nil {
		t.Fatal("expected error for out-of-range index")
	}
}

// InvalidateLayer clears relevant cache entries
func TestInvalidateLayer(t *testing.T) {
	theme := fstest.MapFS{}
	defaults := fstest.MapFS{"file.txt": {Data: []byte("default")}}
	ofs := newTestOverlay(theme, defaults)

	// First read caches that theme layer misses
	_, _ = fs.ReadFile(ofs, "file.txt")

	// Now add file to theme layer via replace
	newTheme := fstest.MapFS{"file.txt": {Data: []byte("theme")}}
	_ = ofs.ReplaceLayer(0, newTheme)

	// Should now read from theme layer
	data, _ := fs.ReadFile(ofs, "file.txt")
	if string(data) != "theme" {
		t.Errorf("after invalidation: got %q, want %q", data, "theme")
	}
}

// fs.WalkDir works with overlay FS
func TestWalkDir(t *testing.T) {
	layer := fstest.MapFS{
		"posts/a.md":   {Data: []byte("a")},
		"posts/b.md":   {Data: []byte("b")},
		"static/c.css": {Data: []byte("c")},
	}
	ofs := newTestOverlay(layer)

	var paths []string
	err := fs.WalkDir(ofs, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) < 4 { // ., posts, posts/a.md, posts/b.md, static, static/c.css
		t.Errorf("WalkDir found %d paths, expected at least 4: %v", len(paths), paths)
	}
}

// §3.2 #4: Null byte in path
func TestOpen_NullByte(t *testing.T) {
	ofs := newTestOverlay(fstest.MapFS{})
	_, err := ofs.Open("templates/post\x00.html")
	if err == nil {
		t.Fatal("expected error for null byte path")
	}
}

// §3.2 #8: ReadDir on nonexistent directory
func TestReadDir_AllMiss(t *testing.T) {
	ofs := newTestOverlay(fstest.MapFS{}, fstest.MapFS{})
	_, err := fs.ReadDir(ofs, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
}

// §3.2 #14: Permission error does not fall through
func TestOpen_PermissionError(t *testing.T) {
	permFS := &errFS{err: fs.ErrPermission}
	defaults := fstest.MapFS{"file.txt": {Data: []byte("default")}}
	ofs := newTestOverlay(permFS, defaults)

	_, err := ofs.Open("file.txt")
	if err == nil {
		t.Fatal("expected permission error, not fallthrough to defaults")
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("expected fs.ErrPermission, got %v", err)
	}
}

// errFS is a test helper that returns a fixed error for all operations.
type errFS struct {
	err error
}

func (e *errFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	return nil, e.err
}

// Concurrent ReplaceLayer + Open (detects A1 race)
func TestReplaceLayer_ConcurrentWithOpen(t *testing.T) {
	old := fstest.MapFS{"file.txt": {Data: []byte("v1")}}
	ofs := newTestOverlay(old)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				f, err := ofs.Open("file.txt")
				if err == nil {
					_ = f.Close()
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 100; j++ {
			newLayer := fstest.MapFS{"file.txt": {Data: []byte("v2")}}
			_ = ofs.ReplaceLayer(0, newLayer)
		}
	}()
	wg.Wait()
}
