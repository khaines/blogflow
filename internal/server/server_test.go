package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/khaines/blogflow/internal/config"
)

func defaultTestConfig() *config.Config {
	cfg := config.Default()
	cfg.Feed.Enabled = true
	cfg.Sync.Strategy = "webhook"
	return cfg
}

func stubHandler(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, body)
	}
}

func testRouteOptions() RouteOptions {
	return RouteOptions{
		ListHandler:    stubHandler("list"),
		PostHandler:    stubHandler("post"),
		PageHandler:    stubHandler("page"),
		TagHandler:     stubHandler("tags"),
		FeedHandler:    stubHandler("feed"),
		SitemapHandler: stubHandler("sitemap"),
		WebhookHandler: stubHandler("webhook"),
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := defaultTestConfig()
	s := New(cfg, slog.Default())
	s.RegisterRoutes(testRouteOptions())
	return s
}

func TestNew_CreatesServer(t *testing.T) {
	cfg := defaultTestConfig()
	s := New(cfg, nil)

	if s.httpServer == nil {
		t.Fatal("httpServer is nil")
	}
	if s.mux == nil {
		t.Fatal("mux is nil")
	}
	if s.config != cfg {
		t.Fatal("config not stored")
	}
	if s.logger == nil {
		t.Fatal("logger is nil — should fall back to slog.Default()")
	}

	wantAddr := fmt.Sprintf(":%d", cfg.Server.Port)
	if s.httpServer.Addr != wantAddr {
		t.Errorf("Addr = %q, want %q", s.httpServer.Addr, wantAddr)
	}
	if s.httpServer.ReadTimeout != cfg.Server.ReadTimeout {
		t.Errorf("ReadTimeout = %v, want %v", s.httpServer.ReadTimeout, cfg.Server.ReadTimeout)
	}
	if s.httpServer.WriteTimeout != cfg.Server.WriteTimeout {
		t.Errorf("WriteTimeout = %v, want %v", s.httpServer.WriteTimeout, cfg.Server.WriteTimeout)
	}
	if s.httpServer.IdleTimeout != cfg.Server.IdleTimeout {
		t.Errorf("IdleTimeout = %v, want %v", s.httpServer.IdleTimeout, cfg.Server.IdleTimeout)
	}
	if s.httpServer.ReadHeaderTimeout != 5*time.Second {
		t.Errorf("ReadHeaderTimeout = %v, want %v", s.httpServer.ReadHeaderTimeout, 5*time.Second)
	}
}

func TestHealthEndpoint(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

func TestReadyEndpoint(t *testing.T) {
	s := newTestServer(t)

	// Before SetReady(true), should return 503.
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("before SetReady: status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if body := rec.Body.String(); !strings.Contains(body, "not ready") {
		t.Errorf("before SetReady: body = %q, want containing %q", body, "not ready")
	}

	// After SetReady(true), should return 200.
	s.SetReady(true)
	req = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("after SetReady: status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "ready" {
		t.Errorf("after SetReady: body = %q, want %q", body, "ready")
	}
}

// stubContentChecker implements ContentChecker for tests.
type stubContentChecker struct {
	posts int
}

func (s *stubContentChecker) PostCount() int { return s.posts }

func TestReadyEndpoint_WithContent(t *testing.T) {
	s := newTestServer(t)
	s.SetReady(true)
	s.SetContentChecker(&stubContentChecker{posts: 5})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "ready" {
		t.Errorf("body = %q, want %q", body, "ready")
	}
}

func TestReadyEndpoint_NoContent_Graceful(t *testing.T) {
	s := newTestServer(t)
	s.SetReady(true)
	s.SetContentChecker(&stubContentChecker{posts: 0})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "ready (no content)" {
		t.Errorf("body = %q, want %q", body, "ready (no content)")
	}
}

func TestReadyEndpoint_NoContent_Strict(t *testing.T) {
	s := newTestServer(t)
	s.SetReady(true)
	s.SetContentChecker(&stubContentChecker{posts: 0})

	req := httptest.NewRequest(http.MethodGet, "/readyz?strict=true", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if body := rec.Body.String(); !strings.Contains(body, "no content") {
		t.Errorf("body = %q, want containing %q", body, "no content")
	}
}

func TestReadyEndpoint_StrictWithContent(t *testing.T) {
	s := newTestServer(t)
	s.SetReady(true)
	s.SetContentChecker(&stubContentChecker{posts: 3})

	req := httptest.NewRequest(http.MethodGet, "/readyz?strict=true", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "ready" {
		t.Errorf("body = %q, want %q", body, "ready")
	}
}

func TestReadyEndpoint_NoContentChecker(t *testing.T) {
	s := newTestServer(t)
	s.SetReady(true)
	// No content checker set — should behave as before

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "ready" {
		t.Errorf("body = %q, want %q", body, "ready")
	}
}

func TestReadyEndpoint_NoContentChecker_Strict(t *testing.T) {
	s := newTestServer(t)
	s.SetReady(true)
	// No content checker — strict should be ignored (backward compat)

	req := httptest.NewRequest(http.MethodGet, "/readyz?strict=true", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "ready" {
		t.Errorf("body = %q, want %q", body, "ready")
	}
}

func TestReadyEndpoint_NilContentChecker(t *testing.T) {
	s := newTestServer(t)
	s.SetReady(true)
	s.SetContentChecker(nil) // must not panic on subsequent requests

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "ready" {
		t.Errorf("body = %q, want %q", body, "ready")
	}
}

func TestContentReadyEndpoint_WithContent(t *testing.T) {
	s := newTestServer(t)
	s.SetContentChecker(&stubContentChecker{posts: 5})

	req := httptest.NewRequest(http.MethodGet, "/readyz/content", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "content available" {
		t.Errorf("body = %q, want %q", body, "content available")
	}
}

func TestContentReadyEndpoint_NoContent(t *testing.T) {
	s := newTestServer(t)
	s.SetContentChecker(&stubContentChecker{posts: 0})

	req := httptest.NewRequest(http.MethodGet, "/readyz/content", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if body := rec.Body.String(); body != "no content" {
		t.Errorf("body = %q, want %q", body, "no content")
	}
}

func TestContentReadyEndpoint_NoChecker(t *testing.T) {
	s := newTestServer(t)
	// No content checker set

	req := httptest.NewRequest(http.MethodGet, "/readyz/content", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if body := rec.Body.String(); body != "no content" {
		t.Errorf("body = %q, want %q", body, "no content")
	}
}

func TestSecurityHeaders(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	headers := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "SAMEORIGIN",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}
	for key, want := range headers {
		got := rec.Header().Get(key)
		if got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}

	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("Content-Security-Policy header missing")
	}
}

func TestPermissionsPolicyHeader(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	got := rec.Header().Get("Permissions-Policy")
	want := "camera=(), microphone=(), geolocation=(), payment=(), usb=(), browsing-topics=(), interest-cohort=()"
	if got != want {
		t.Errorf("Permissions-Policy = %q, want %q", got, want)
	}
}

func TestHSTSHeader_DefaultOff(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("HSTS should be absent by default, got %q", got)
	}
}

func TestHSTSHeader_EnabledWhenTLSTerminated(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Server.TLSTerminated = true
	cfg.Server.HSTSMaxAge = 63072000

	s := New(cfg, slog.Default())
	s.RegisterRoutes(testRouteOptions())

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	got := rec.Header().Get("Strict-Transport-Security")
	want := "max-age=63072000; includeSubDomains"
	if got != want {
		t.Errorf("Strict-Transport-Security = %q, want %q", got, want)
	}
}

func TestCSPHeader(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "frame-ancestors") {
		t.Errorf("CSP missing frame-ancestors directive: %s", csp)
	}
	if !strings.Contains(csp, "default-src 'none'") {
		t.Errorf("CSP missing default-src 'none': %s", csp)
	}
}

func TestLoggingMiddleware(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg := defaultTestConfig()
	s := New(cfg, logger)
	s.RegisterRoutes(testRouteOptions())

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	logged := buf.String()
	for _, want := range []string{"method", "path", "status", "duration"} {
		if !strings.Contains(logged, want) {
			t.Errorf("log output missing %q: %s", want, logged)
		}
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	cfg := defaultTestConfig()
	s := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	panicOpts := testRouteOptions()
	panicOpts.ListHandler = func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	}
	s.RegisterRoutes(panicOpts)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	// Must not panic — recovery middleware catches it.
	s.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestRecoveryMiddleware_HeadersAlreadySent(t *testing.T) {
	cfg := defaultTestConfig()
	s := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	panicOpts := testRouteOptions()
	panicOpts.ListHandler = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "partial")
		panic("late panic after headers sent")
	}
	s.RegisterRoutes(panicOpts)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	// Must not panic even when headers were already sent.
	s.httpServer.Handler.ServeHTTP(rec, req)

	// Status should be the original 200 since headers were already flushed;
	// the recovery middleware must not attempt http.Error.
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (headers already sent)", rec.Code, http.StatusOK)
	}
}

func TestGracefulShutdown(t *testing.T) {
	cfg := defaultTestConfig()
	s := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.RegisterRoutes(testRouteOptions())

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	// Start server on the already-open listener (no sleep needed).
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Serve(ln)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown error: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Serve returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not stop within timeout")
	}
}

func TestStaticFileServing(t *testing.T) {
	cfg := defaultTestConfig()
	s := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	staticFS := fstest.MapFS{
		"css/style.css": &fstest.MapFile{
			Data: []byte("body { color: red; }"),
		},
	}

	opts := testRouteOptions()
	opts.StaticFS = staticFS
	s.RegisterRoutes(opts)

	req := httptest.NewRequest(http.MethodGet, "/static/css/style.css", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); !strings.Contains(body, "color: red") {
		t.Errorf("body = %q, want CSS content", body)
	}
}

func TestWebhookPathWithSpaces_Panics(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Sync.Strategy = "webhook"
	cfg.Sync.Webhook.Path = "/web hook"

	s := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for webhook path with spaces")
		}
		msg := fmt.Sprintf("%v", r)
		if !strings.Contains(msg, "contains spaces") {
			t.Errorf("panic message = %q, want containing %q", msg, "contains spaces")
		}
	}()

	s.RegisterRoutes(testRouteOptions())
}

func TestLoggingMiddleware_RequestURI(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg := defaultTestConfig()
	s := New(cfg, logger)
	s.RegisterRoutes(testRouteOptions())

	req := httptest.NewRequest(http.MethodGet, "/healthz?foo=bar", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	logged := buf.String()
	if !strings.Contains(logged, "/healthz?foo=bar") {
		t.Errorf("log should contain full RequestURI, got: %s", logged)
	}
}

func TestHealthEndpoint_CacheControl(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want %q", got, "no-store")
	}
}

func TestReadyEndpoint_CacheControl(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want %q", got, "no-store")
	}
}

func TestReadyChannel_ClosedAfterStart(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Server.Port = 0 // let OS pick a free port
	s := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.RegisterRoutes(testRouteOptions())

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Start()
	}()

	select {
	case <-s.Ready():
		// success — channel closed once listener is bound
	case err := <-errCh:
		t.Fatalf("Start returned before Ready: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Ready channel was not closed within timeout")
	}

	// Clean shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown error: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Start returned unexpected error: %v", err)
	}
}

func TestReadyChannel_ClosedAfterServe(t *testing.T) {
	cfg := defaultTestConfig()
	s := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.RegisterRoutes(testRouteOptions())

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Serve(ln)
	}()

	select {
	case <-s.Ready():
		// success — channel closed when Serve is called with pre-bound listener
	case err := <-errCh:
		t.Fatalf("Serve returned before Ready: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Ready channel was not closed within timeout")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown error: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Serve returned unexpected error: %v", err)
	}
}

func TestReadyChannel_DoubleCloseProtection(t *testing.T) {
	cfg := defaultTestConfig()
	s := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.RegisterRoutes(testRouteOptions())

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Serve(ln)
	}()

	select {
	case <-s.Ready():
	case <-time.After(5 * time.Second):
		t.Fatal("Ready channel was not closed within timeout")
	}

	// Calling Serve's close path again via shutdown+re-serve should not panic
	// thanks to sync.Once protection. Verify by directly invoking the ready
	// signal a second time through a second Serve on a new listener.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown error: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Serve returned unexpected error: %v", err)
	}

	// A second Serve on the same Server must not panic from double-close.
	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen (2nd): %v", err)
	}
	errCh2 := make(chan error, 1)
	go func() {
		errCh2 <- s.Serve(ln2)
	}()

	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	if err := s.Shutdown(ctx2); err != nil {
		t.Fatalf("Shutdown (2nd) error: %v", err)
	}
	if err := <-errCh2; err != nil {
		t.Fatalf("Serve (2nd) returned unexpected error: %v", err)
	}
}

func TestReadyChannel_NotClosedBeforeStart(t *testing.T) {
	cfg := defaultTestConfig()
	s := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	select {
	case <-s.Ready():
		t.Fatal("Ready channel should not be closed before Start/Serve")
	default:
		// expected — channel is still open
	}
}
