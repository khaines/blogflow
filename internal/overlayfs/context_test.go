package overlayfs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"strings"
	"testing"
	"testing/fstest"
)

func newTestContextOverlay(logger *slog.Logger, layers ...fs.FS) *ContextOverlayFS {
	names := make([]string, len(layers))
	for i := range layers {
		names[i] = layerName(i)
	}
	return NewContextOverlayFS(NewOverlayFS(layers, names), logger)
}

// --- Context cancellation aborts resolution ---

func TestContextOpen_Cancelled(t *testing.T) {
	layer := fstest.MapFS{"file.txt": {Data: []byte("data")}}
	cfs := newTestContextOverlay(nil, layer)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := cfs.Open(ctx, "file.txt")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestContextReadFile_Cancelled(t *testing.T) {
	layer := fstest.MapFS{"file.txt": {Data: []byte("data")}}
	cfs := newTestContextOverlay(nil, layer)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := cfs.ReadFile(ctx, "file.txt")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestContextReadDir_Cancelled(t *testing.T) {
	layer := fstest.MapFS{"dir/file.txt": {Data: []byte("data")}}
	cfs := newTestContextOverlay(nil, layer)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := cfs.ReadDir(ctx, "dir")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestContextStat_Cancelled(t *testing.T) {
	layer := fstest.MapFS{"file.txt": {Data: []byte("data")}}
	cfs := newTestContextOverlay(nil, layer)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := cfs.Stat(ctx, "file.txt")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestContextOpenFile_Cancelled(t *testing.T) {
	layer := fstest.MapFS{"file.txt": {Data: []byte("data")}}
	cfs := newTestContextOverlay(nil, layer)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := cfs.OpenFile(ctx, "file.txt")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// --- Security logging on path traversal ---

func TestContextOpen_PathTraversalLogged(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	cfs := newTestContextOverlay(logger, fstest.MapFS{})

	ctx := context.WithValue(context.Background(), RequestIDKey, "req-123")
	ctx = context.WithValue(ctx, RemoteAddrKey, "192.168.1.100")

	_, err := cfs.Open(ctx, "../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}

	logged := buf.String()
	if !strings.Contains(logged, "path traversal attempt") {
		t.Error("expected 'path traversal attempt' in log")
	}
	if !strings.Contains(logged, "req-123") {
		t.Error("expected request_id in log")
	}
	if !strings.Contains(logged, "192.168.1.100") {
		t.Error("expected remote_addr in log")
	}
}

func TestContextReadFile_PathTraversalLogged(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	cfs := newTestContextOverlay(logger, fstest.MapFS{})

	ctx := context.WithValue(context.Background(), RequestIDKey, "req-456")
	_, err := cfs.ReadFile(ctx, "/etc/shadow")
	if err == nil {
		t.Fatal("expected error for absolute path")
	}
	if !strings.Contains(buf.String(), "path traversal attempt") {
		t.Error("expected security log")
	}
}

func TestContextStat_PathTraversalLogged(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	cfs := newTestContextOverlay(logger, fstest.MapFS{})

	_, err := cfs.Stat(context.Background(), "../etc/passwd")
	if err == nil {
		t.Fatal("expected error for traversal path")
	}
	if !strings.Contains(buf.String(), "path traversal attempt") {
		t.Error("expected security log")
	}
}

// --- All FS methods work through the wrapper ---

func TestContextOpen_Works(t *testing.T) {
	theme := fstest.MapFS{"templates/post.html": {Data: []byte("theme-post")}}
	defaults := fstest.MapFS{"templates/post.html": {Data: []byte("default-post")}}
	cfs := newTestContextOverlay(nil, theme, defaults)

	f, err := cfs.Open(context.Background(), "templates/post.html")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "theme-post" {
		t.Errorf("got %q, want %q", data, "theme-post")
	}
}

func TestContextReadFile_Works(t *testing.T) {
	layer := fstest.MapFS{"file.txt": {Data: []byte("hello")}}
	cfs := newTestContextOverlay(nil, layer)

	data, err := cfs.ReadFile(context.Background(), "file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Errorf("got %q, want %q", data, "hello")
	}
}

func TestContextReadFile_Fallback(t *testing.T) {
	theme := fstest.MapFS{}
	defaults := fstest.MapFS{"base.html": {Data: []byte("default-base")}}
	cfs := newTestContextOverlay(nil, theme, defaults)

	data, err := cfs.ReadFile(context.Background(), "base.html")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "default-base" {
		t.Errorf("got %q, want %q", data, "default-base")
	}
}

func TestContextReadDir_Works(t *testing.T) {
	theme := fstest.MapFS{
		"templates/a.html": {Data: []byte("a")},
	}
	defaults := fstest.MapFS{
		"templates/b.html": {Data: []byte("b")},
	}
	cfs := newTestContextOverlay(nil, theme, defaults)

	entries, err := cfs.ReadDir(context.Background(), "templates")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].Name() != "a.html" || entries[1].Name() != "b.html" {
		t.Errorf("got [%s, %s], want [a.html, b.html]", entries[0].Name(), entries[1].Name())
	}
}

func TestContextStat_Works(t *testing.T) {
	layer := fstest.MapFS{"file.txt": {Data: []byte("hello world")}}
	cfs := newTestContextOverlay(nil, layer)

	info, err := cfs.Stat(context.Background(), "file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if info.Name() != "file.txt" {
		t.Errorf("Name = %q, want %q", info.Name(), "file.txt")
	}
	if info.Size() != 11 {
		t.Errorf("Size = %d, want 11", info.Size())
	}
}

func TestContextOpenFile_Works(t *testing.T) {
	layer := fstest.MapFS{"file.txt": {Data: []byte("hello world")}}
	cfs := newTestContextOverlay(nil, layer)

	f, info, err := cfs.OpenFile(context.Background(), "file.txt")
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

func TestContextOpen_NotFound(t *testing.T) {
	cfs := newTestContextOverlay(nil, fstest.MapFS{})

	_, err := cfs.Open(context.Background(), "nope.txt")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected ErrNotExist, got %v", err)
	}
}

// --- Nil logger is safe ---

func TestContextOpen_NilLogger(t *testing.T) {
	layer := fstest.MapFS{"file.txt": {Data: []byte("data")}}
	cfs := NewContextOverlayFS(NewOverlayFS([]fs.FS{layer}, []string{"test"}), nil)

	f, err := cfs.Open(context.Background(), "file.txt")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
}

func TestContextOpen_NilLoggerPathTraversal(t *testing.T) {
	cfs := NewContextOverlayFS(NewOverlayFS([]fs.FS{fstest.MapFS{}}, []string{"test"}), nil)

	_, err := cfs.Open(context.Background(), "../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

// --- Delegation methods ---

func TestContextInvalidateLayer(t *testing.T) {
	theme := fstest.MapFS{}
	defaults := fstest.MapFS{"file.txt": {Data: []byte("default")}}
	cfs := newTestContextOverlay(nil, theme, defaults)

	// Warm the negative cache
	_, _ = cfs.ReadFile(context.Background(), "file.txt")

	// Replace and invalidate
	newTheme := fstest.MapFS{"file.txt": {Data: []byte("theme")}}
	_ = cfs.ReplaceLayer(0, newTheme)

	data, _ := cfs.ReadFile(context.Background(), "file.txt")
	if string(data) != "theme" {
		t.Errorf("got %q, want %q", data, "theme")
	}
}

func TestContextInvalidateAll(t *testing.T) {
	cfs := newTestContextOverlay(nil, fstest.MapFS{"file.txt": {Data: []byte("v1")}})

	_, _ = cfs.ReadFile(context.Background(), "file.txt")
	cfs.InvalidateAll() // should not panic
}

// --- Logging with context values ---

func TestContextOpen_LogsOperation(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	layer := fstest.MapFS{"file.txt": {Data: []byte("data")}}
	cfs := newTestContextOverlay(logger, layer)

	ctx := context.WithValue(context.Background(), RequestIDKey, "req-789")
	_, err := cfs.Open(ctx, "file.txt")
	if err != nil {
		t.Fatal(err)
	}

	logged := buf.String()
	if !strings.Contains(logged, "overlayfs operation") {
		t.Error("expected operation log")
	}
	if !strings.Contains(logged, "req-789") {
		t.Error("expected request_id in log")
	}
}

// --- Context key types ---

func TestContextKeys_Distinct(t *testing.T) {
	ctx := context.WithValue(context.Background(), RequestIDKey, "req-abc")
	ctx = context.WithValue(ctx, RemoteAddrKey, "10.0.0.1")

	if got := contextString(ctx, RequestIDKey); got != "req-abc" {
		t.Errorf("RequestIDKey = %q, want %q", got, "req-abc")
	}
	if got := contextString(ctx, RemoteAddrKey); got != "10.0.0.1" {
		t.Errorf("RemoteAddrKey = %q, want %q", got, "10.0.0.1")
	}
}

func TestContextString_Missing(t *testing.T) {
	if got := contextString(context.Background(), RequestIDKey); got != "" {
		t.Errorf("missing key should return empty string, got %q", got)
	}
}

// --- Constructor panics on nil inner ---

func TestNewContextOverlayFS_NilInnerPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for nil inner OverlayFS")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "nil OverlayFS") {
			t.Errorf("unexpected panic value: %v", r)
		}
	}()
	NewContextOverlayFS(nil, nil)
}

// --- Inner accessor ---

func TestContextInner_ReturnsSameOverlayFS(t *testing.T) {
	inner := NewOverlayFS([]fs.FS{fstest.MapFS{}}, []string{"test"})
	cfs := NewContextOverlayFS(inner, nil)

	if got := cfs.Inner(); got != inner {
		t.Errorf("Inner() returned different pointer")
	}
}

// --- Permission error propagates (does not fall through) ---

func TestContextOpen_PermissionError(t *testing.T) {
	permFS := &errFS{err: fs.ErrPermission}
	defaults := fstest.MapFS{"file.txt": {Data: []byte("default")}}
	cfs := newTestContextOverlay(nil, permFS, defaults)

	_, err := cfs.Open(context.Background(), "file.txt")
	if err == nil {
		t.Fatal("expected permission error")
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("expected fs.ErrPermission, got %v", err)
	}
}
