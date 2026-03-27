package overlayfs

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// contextKey is an unexported type for context keys defined in this package,
// preventing collisions with keys defined in other packages.
type contextKey int

const (
	// RequestIDKey is the context key for the HTTP request ID (string).
	// HTTP middleware populates this for log correlation and tracing.
	RequestIDKey contextKey = iota

	// RemoteAddrKey is the context key for the client's remote address (string).
	// Used in security WARN logs for path traversal attribution.
	RemoteAddrKey
)

// resolutionKey is a distinct unexported type for the Resolution context key.
type resolutionContextKey struct{}

// tracer is the package-level OpenTelemetry tracer for ContextOverlayFS.
// Obtained from the global provider on each call to support test swapping.
const tracerName = "github.com/khaines/blogflow/internal/overlayfs"

// ContextOverlayFS wraps OverlayFS with context.Context support for
// cancellation, tracing, and security log correlation. This is the
// public API surface — consumers should use this type, not OverlayFS directly.
//
// It does NOT implement io/fs.FS because its methods require context.Context.
// For stdlib consumers that need fs.FS (e.g., html/template.ParseFS), use
// the inner OverlayFS directly.
//
// ContextOverlayFS is NOT safe for concurrent use. It is designed to be
// created per-request and accessed from a single goroutine.
//
// Security: Resolution MUST NOT appear in HTTP responses. It is
// intended for server-side observability only.
type ContextOverlayFS struct {
	inner  *OverlayFS
	logger *slog.Logger
}

// NewContextOverlayFS creates a context-aware overlay wrapping the given OverlayFS.
// A nil logger is safe — logging will be silently skipped.
// Panics if inner is nil (programming error).
func NewContextOverlayFS(inner *OverlayFS, logger *slog.Logger) *ContextOverlayFS {
	if inner == nil {
		panic("overlayfs: NewContextOverlayFS called with nil OverlayFS")
	}
	return &ContextOverlayFS{inner: inner, logger: logger}
}

// Inner returns the underlying OverlayFS for callers that need a plain
// fs.FS (e.g., html/template.ParseFS).
func (c *ContextOverlayFS) Inner() *OverlayFS {
	return c.inner
}

// Open opens a file with context support. Checks ctx.Err() before
// delegating to the inner OverlayFS. Logs WARN on path traversal attempts
// with request context values.
func (c *ContextOverlayFS) Open(ctx context.Context, name string) (fs.File, error) {
	if err := c.checkPath(ctx, "open", name); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("overlayfs: open %q: %w", name, err)
	}

	ctx, span := otel.Tracer(tracerName).Start(ctx, "overlayfs.open", trace.WithAttributes(
		attribute.String("path", name),
	))
	defer span.End()

	start := time.Now()
	f, err := c.inner.Open(name)
	c.recordResolution(ctx, span, name)
	c.finishSpan(span, err)
	c.logOperation(ctx, "open", name, start, err)
	return f, err
}

// ReadFile reads a file with context support.
func (c *ContextOverlayFS) ReadFile(ctx context.Context, name string) ([]byte, error) {
	if err := c.checkPath(ctx, "readfile", name); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("overlayfs: readfile %q: %w", name, err)
	}

	ctx, span := otel.Tracer(tracerName).Start(ctx, "overlayfs.readfile", trace.WithAttributes(
		attribute.String("path", name),
	))
	defer span.End()

	start := time.Now()
	data, err := c.inner.ReadFile(name)
	c.recordResolution(ctx, span, name)
	c.finishSpan(span, err)
	c.logOperation(ctx, "readfile", name, start, err)
	return data, err
}

// ReadDir lists a directory with context support.
func (c *ContextOverlayFS) ReadDir(ctx context.Context, name string) ([]fs.DirEntry, error) {
	if err := c.checkPath(ctx, "readdir", name); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("overlayfs: readdir %q: %w", name, err)
	}

	ctx, span := otel.Tracer(tracerName).Start(ctx, "overlayfs.readdir", trace.WithAttributes(
		attribute.String("path", name),
	))
	defer span.End()

	start := time.Now()
	entries, err := c.inner.ReadDir(name)
	c.finishSpan(span, err)
	c.logOperation(ctx, "readdir", name, start, err)
	return entries, err
}

// Stat returns file info with context support.
func (c *ContextOverlayFS) Stat(ctx context.Context, name string) (fs.FileInfo, error) {
	if err := c.checkPath(ctx, "stat", name); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("overlayfs: stat %q: %w", name, err)
	}

	ctx, span := otel.Tracer(tracerName).Start(ctx, "overlayfs.stat", trace.WithAttributes(
		attribute.String("path", name),
	))
	defer span.End()

	start := time.Now()
	info, err := c.inner.Stat(name)
	c.recordResolution(ctx, span, name)
	c.finishSpan(span, err)
	c.logOperation(ctx, "stat", name, start, err)
	return info, err
}

// OpenFile opens a file and returns both handle and FileInfo atomically
// from a single layer resolution, with context support.
func (c *ContextOverlayFS) OpenFile(ctx context.Context, name string) (fs.File, fs.FileInfo, error) {
	if err := c.checkPath(ctx, "openfile", name); err != nil {
		return nil, nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("overlayfs: openfile %q: %w", name, err)
	}

	ctx, span := otel.Tracer(tracerName).Start(ctx, "overlayfs.openfile", trace.WithAttributes(
		attribute.String("path", name),
	))
	defer span.End()

	start := time.Now()
	f, info, err := c.inner.OpenFile(name)
	c.recordResolution(ctx, span, name)
	c.finishSpan(span, err)
	c.logOperation(ctx, "openfile", name, start, err)
	return f, info, err
}

// InvalidateLayer delegates to the inner OverlayFS.
func (c *ContextOverlayFS) InvalidateLayer(layerIndex int) {
	c.inner.InvalidateLayer(layerIndex)
}

// InvalidateAll delegates to the inner OverlayFS.
func (c *ContextOverlayFS) InvalidateAll() {
	c.inner.InvalidateAll()
}

// ReplaceLayer delegates to the inner OverlayFS.
func (c *ContextOverlayFS) ReplaceLayer(layerIndex int, newFS fs.FS) error {
	return c.inner.ReplaceLayer(layerIndex, newFS)
}

// checkPath validates the path and logs a WARN on traversal attempts.
func (c *ContextOverlayFS) checkPath(ctx context.Context, op, name string) error {
	if fs.ValidPath(name) {
		return nil
	}

	if c.logger != nil {
		c.logger.WarnContext(ctx, "path traversal attempt",
			slog.String("op", op),
			slog.String("path", name),
			slog.String("request_id", contextString(ctx, RequestIDKey)),
			slog.String("remote_addr", contextString(ctx, RemoteAddrKey)),
		)
	}

	return &fs.PathError{Op: op, Path: name, Err: fs.ErrInvalid}
}

// logOperation logs span-like info for each FS operation via slog.
func (c *ContextOverlayFS) logOperation(ctx context.Context, op, name string, start time.Time, err error) {
	if c.logger == nil || !c.logger.Enabled(ctx, slog.LevelDebug) {
		return
	}
	duration := time.Since(start)

	attrs := []slog.Attr{
		slog.String("op", op),
		slog.String("path", name),
		slog.Duration("duration", duration),
		slog.String("request_id", contextString(ctx, RequestIDKey)),
	}

	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
		c.logger.LogAttrs(ctx, slog.LevelDebug, "overlayfs operation failed", attrs...)
	} else {
		c.logger.LogAttrs(ctx, slog.LevelDebug, "overlayfs operation", attrs...)
	}
}

// recordResolution adds Resolution attributes to the span if the path
// can be resolved. ReadDir does not resolve to a single layer, so this
// is not called for readdir operations.
func (c *ContextOverlayFS) recordResolution(_ context.Context, span trace.Span, name string) {
	res, err := c.inner.Resolve(name)
	if err != nil {
		return
	}
	span.SetAttributes(
		attribute.String("layer.name", res.LayerName),
		attribute.Int("layer.index", res.LayerIndex),
	)
}

// finishSpan sets span status to Error when err is non-nil.
func (c *ContextOverlayFS) finishSpan(span trace.Span, err error) {
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
	}
}

// contextString extracts a string value from the context, returning "" if absent.
func contextString(ctx context.Context, key contextKey) string {
	v, _ := ctx.Value(key).(string)
	return v
}

// ResolutionFromContext extracts a Resolution stored by ContextWithResolution.
// Returns the Resolution and true if present, or the zero value and false
// if no resolution has been stored.
func ResolutionFromContext(ctx context.Context) (Resolution, bool) {
	r, ok := ctx.Value(resolutionContextKey{}).(Resolution)
	return r, ok
}

// ContextWithResolution returns a new context carrying the given Resolution.
// This is intended for middleware or handlers that call OverlayFS.Resolve
// and want to propagate the result downstream for observability.
//
// Security: Resolution MUST NOT appear in HTTP responses. It is
// intended for server-side observability only.
func ContextWithResolution(ctx context.Context, r Resolution) context.Context {
	return context.WithValue(ctx, resolutionContextKey{}, r)
}
