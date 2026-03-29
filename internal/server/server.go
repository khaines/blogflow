// Package server provides BlogFlow's HTTP server with middleware,
// routing, health checks, and graceful shutdown.
package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/khaines/blogflow/internal/config"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// ContentChecker reports how many posts are available. Implementations must
// be safe for concurrent use (the handler calls PostCount on every request).
type ContentChecker interface {
	PostCount() int
}

// Server is the BlogFlow HTTP server.
type Server struct {
	httpServer     *http.Server
	metricsServer  *http.Server // nil when MetricsPort == 0
	mux            *http.ServeMux
	config         *config.Config
	logger         *slog.Logger
	ready          atomic.Bool
	readyCh        chan struct{}
	readyOnce      sync.Once
	ipResolver     *ClientIPResolver
	contentChecker atomic.Pointer[ContentChecker]
}

// New creates a new BlogFlow server.
func New(cfg *config.Config, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	mux := http.NewServeMux()

	ipResolver, err := NewClientIPResolver(cfg.Server.TrustedProxyCIDRs)
	if err != nil {
		logger.Error("invalid trusted proxy CIDR", "error", err)
		panic(fmt.Sprintf("server: invalid trusted proxy CIDR: %v", err))
	}

	s := &Server{
		mux:        mux,
		config:     cfg,
		logger:     logger,
		readyCh:    make(chan struct{}),
		ipResolver: ipResolver,
	}

	s.httpServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           s.middleware(mux),
		ReadTimeout:       cfg.Server.ReadTimeout,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
	}

	if cfg.Server.MetricsPort > 0 {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("GET /metrics", MetricsHandler())
		metricsMux.HandleFunc("GET /healthz", s.healthHandler)
		s.metricsServer = &http.Server{
			Addr:              fmt.Sprintf(":%d", cfg.Server.MetricsPort),
			Handler:           s.recoveryMiddleware(metricsMux),
			ReadTimeout:       cfg.Server.ReadTimeout,
			ReadHeaderTimeout: 5 * time.Second,
			WriteTimeout:      cfg.Server.WriteTimeout,
			IdleTimeout:       cfg.Server.IdleTimeout,
		}
	}

	return s
}

// RouteOptions holds the handler functions injected into the server.
type RouteOptions struct {
	HomeHandler      http.HandlerFunc
	ListHandler      http.HandlerFunc
	PostsListHandler http.HandlerFunc
	PostHandler      http.HandlerFunc
	PageHandler      http.HandlerFunc
	TagHandler       http.HandlerFunc
	FeedHandler      http.HandlerFunc
	SitemapHandler   http.HandlerFunc
	WebhookHandler   http.HandlerFunc
	StaticFS         fs.FS
}

// RegisterRoutes sets up all HTTP routes. Call this after content and theme are loaded.
// contentHandler, pageHandler, etc. are injected as http.HandlerFunc.
func (s *Server) RegisterRoutes(opts RouteOptions) {
	// Nil-handler guards for required routes.
	if opts.HomeHandler == nil {
		panic("server: RegisterRoutes requires HomeHandler")
	}
	if opts.ListHandler == nil {
		panic("server: RegisterRoutes requires ListHandler")
	}
	if opts.PostsListHandler == nil {
		panic("server: RegisterRoutes requires PostsListHandler")
	}
	if opts.PostHandler == nil {
		panic("server: RegisterRoutes requires PostHandler")
	}
	if opts.PageHandler == nil {
		panic("server: RegisterRoutes requires PageHandler")
	}
	if opts.TagHandler == nil {
		panic("server: RegisterRoutes requires TagHandler")
	}
	if opts.SitemapHandler == nil {
		panic("server: RegisterRoutes requires SitemapHandler")
	}

	// Content routes
	s.mux.HandleFunc("GET /{$}", opts.HomeHandler)
	s.mux.HandleFunc("GET /posts", opts.PostsListHandler)
	s.mux.HandleFunc("GET /page/{page}", opts.ListHandler)
	s.mux.HandleFunc("GET /posts/{slug}", opts.PostHandler)
	s.mux.HandleFunc("GET /pages/{slug}", opts.PageHandler)
	s.mux.HandleFunc("GET /tags/{tag}", opts.TagHandler)

	// Feed
	if s.config.Feed.Enabled {
		if opts.FeedHandler == nil {
			panic("server: RegisterRoutes requires FeedHandler when feed is enabled")
		}
		s.mux.HandleFunc("GET /feed.xml", opts.FeedHandler)
	}

	// Sitemap
	s.mux.HandleFunc("GET /sitemap.xml", opts.SitemapHandler)

	// Health checks
	s.mux.HandleFunc("GET /healthz", s.healthHandler)
	s.mux.HandleFunc("GET /readyz", s.readyHandler)
	s.mux.HandleFunc("GET /readyz/content", s.contentReadyHandler)

	// Prometheus metrics: on main mux only when no separate metrics port is configured
	if s.config.Server.MetricsPort == 0 {
		s.mux.Handle("GET /metrics", MetricsHandler())
	}

	// Static assets
	if opts.StaticFS != nil {
		s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(opts.StaticFS)))
	}

	// Webhook (if configured)
	if s.config.Sync.Strategy == "webhook" {
		if opts.WebhookHandler == nil {
			panic("server: RegisterRoutes requires WebhookHandler when sync strategy is webhook")
		}
		webhookPath := s.config.Sync.Webhook.Path
		if strings.ContainsAny(webhookPath, " \t") {
			panic(fmt.Sprintf("server: webhook path %q contains spaces", webhookPath))
		}
		s.mux.HandleFunc("POST "+webhookPath, opts.WebhookHandler)
	}
}

// Ready returns a channel that is closed once the server's network listener
// is bound and the server is ready to accept connections.
func (s *Server) Ready() <-chan struct{} {
	return s.readyCh
}

// Start begins serving HTTP. Blocks until the server stops.
func (s *Server) Start() error {
	addr := s.httpServer.Addr
	if addr == "" {
		addr = ":http"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("server: %w", err)
	}
	s.logger.Info("server listening", "addr", ln.Addr().String())
	s.readyOnce.Do(func() { close(s.readyCh) })
	if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server: %w", err)
	}
	return nil
}

// Serve begins serving HTTP on the given listener. Blocks until the server stops.
// The ready channel is closed immediately since the listener is already bound.
func (s *Server) Serve(ln net.Listener) error {
	s.logger.Info("server starting", "addr", ln.Addr().String())
	s.readyOnce.Do(func() { close(s.readyCh) })
	if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server: %w", err)
	}
	return nil
}

// Shutdown gracefully stops the server with the given context deadline.
// If a separate metrics server is running, both servers are shut down
// concurrently so that one cannot consume the other's timeout budget.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("server shutting down")

	var metricsErr error
	var wg sync.WaitGroup
	if s.metricsServer != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.metricsServer.Shutdown(ctx); err != nil {
				metricsErr = fmt.Errorf("metrics server: %w", err)
			}
		}()
	}

	var mainErr error
	if err := s.httpServer.Shutdown(ctx); err != nil {
		mainErr = fmt.Errorf("main server: %w", err)
	}
	wg.Wait()
	return errors.Join(mainErr, metricsErr)
}

// StartMetrics starts the metrics server on its dedicated port.
// Returns nil immediately if no separate metrics port is configured.
// Blocks until the metrics server stops.
func (s *Server) StartMetrics() error {
	if s.metricsServer == nil {
		return nil
	}
	addr := s.metricsServer.Addr
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("metrics server: %w", err)
	}
	s.logger.Info("metrics server listening", "addr", ln.Addr().String())
	if err := s.metricsServer.Serve(ln); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("metrics server: %w", err)
	}
	return nil
}

// MetricsServer returns the dedicated metrics *http.Server, or nil
// if metrics are served on the main port.
func (s *Server) MetricsServer() *http.Server {
	return s.metricsServer
}

// middleware chains standard middleware: request-ID, logging, security headers, recovery, metrics, otelhttp.
func (s *Server) middleware(next http.Handler) http.Handler {
	// Order: request-ID (outermost) → otelhttp → logging → recovery → security headers → metrics → handler
	return s.requestIDMiddleware(
		otelhttp.NewMiddleware("blogflow")(
			s.loggingMiddleware(
				s.recoveryMiddleware(
					s.securityHeadersMiddleware(
						MetricsMiddleware(next),
					),
				),
			),
		),
	)
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(wrapped, r)
		s.logger.Info("request",
			"method", r.Method,
			"path", r.URL.RequestURI(),
			"status", wrapped.statusCode,
			"duration", time.Since(start),
			"remote", r.RemoteAddr,
			"client_ip", s.ipResolver.ClientIP(r),
			"request_id", RequestIDFromContext(r.Context()),
		)
	})
}

func (s *Server) securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy",
			"default-src 'none'; script-src 'self'; object-src 'none'; "+
				"connect-src 'none'; style-src 'self'; img-src 'self' https: data:; "+
				"font-src 'self' https:; base-uri 'self'; form-action 'self'; "+
				"frame-ancestors 'self'")
		w.Header().Set("Permissions-Policy",
			"camera=(), microphone=(), geolocation=(), payment=(), usb=(), browsing-topics=(), interest-cohort=()")
		if s.config.Server.TLSTerminated {
			w.Header().Set("Strict-Transport-Security",
				fmt.Sprintf("max-age=%d; includeSubDomains", s.config.Server.HSTSMaxAge))
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				panicStr := fmt.Sprintf("%v", rec)
				if len(panicStr) > 256 {
					panicStr = panicStr[:256] + "...[truncated]"
				}
				s.logger.Error("panic recovered",
					"panic_type", fmt.Sprintf("%T", rec),
					"panic", panicStr,
					"method", r.Method,
					"path", r.URL.Path,
					"stack", string(debug.Stack()),
				)
				if rw, ok := w.(*responseWriter); ok && rw.headerWritten {
					// Headers already sent — can't send 500; close connection
					return
				}
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "ok")
}

// SetReady marks the server as ready (or not) for traffic.
func (s *Server) SetReady(v bool) { s.ready.Store(v) }

// SetContentChecker installs a ContentChecker used by /readyz and
// /readyz/content to report content availability. Safe for concurrent use.
// Passing nil clears the checker (readyz falls back to basic ready/not-ready).
func (s *Server) SetContentChecker(cc ContentChecker) {
	if cc == nil {
		s.contentChecker.Store(nil)
		return
	}
	s.contentChecker.Store(&cc)
}

func (s *Server) readyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Cache-Control", "no-store")
	if !s.ready.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprint(w, "not ready")
		return
	}

	strict := r.URL.Query().Get("strict") == "true"
	if cc := s.contentChecker.Load(); cc != nil && (*cc).PostCount() == 0 {
		if strict {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprint(w, "not ready (no content)")
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ready (no content)")
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "ready")
}

func (s *Server) contentReadyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Cache-Control", "no-store")
	if cc := s.contentChecker.Load(); cc != nil && (*cc).PostCount() > 0 {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "content available")
		return
	}
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = fmt.Fprint(w, "no content")
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode    int
	headerWritten bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.headerWritten {
		rw.statusCode = code
		rw.headerWritten = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.headerWritten {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}

// Unwrap returns the underlying ResponseWriter for middleware that need it.
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}
