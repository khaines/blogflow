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

	"github.com/kenhaines/blogflow/internal/config"
)

func defaultTestConfig() *config.Config {
	cfg := config.Default()
	cfg.Feed.Enabled = true
	cfg.Sync.Strategy = "webhook"
	return cfg
}

func stubHandler(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
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
		fmt.Fprint(w, "partial")
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
