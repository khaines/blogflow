// Package server provides BlogFlow's HTTP server with middleware,
// routing, health checks, and graceful shutdown.
package server

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"

	"github.com/khaines/blogflow/internal/config"
)

// Server is the BlogFlow HTTP server.
type Server struct {
	httpServer *http.Server
	mux        *http.ServeMux
	config     *config.Config
	logger     *slog.Logger
	ready      atomic.Bool
}

// New creates a new BlogFlow server.
func New(cfg *config.Config, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	mux := http.NewServeMux()
	s := &Server{
		mux:    mux,
		config: cfg,
		logger: logger,
	}

	s.httpServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           s.middleware(mux),
		ReadTimeout:       cfg.Server.ReadTimeout,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
	}

	return s
}

// RouteOptions holds the handler functions injected into the server.
type RouteOptions struct {
	ListHandler    http.HandlerFunc
	PostHandler    http.HandlerFunc
	PageHandler    http.HandlerFunc
	TagHandler     http.HandlerFunc
	FeedHandler    http.HandlerFunc
	SitemapHandler http.HandlerFunc
	WebhookHandler http.HandlerFunc
	StaticFS       fs.FS
}

// RegisterRoutes sets up all HTTP routes. Call this after content and theme are loaded.
// contentHandler, pageHandler, etc. are injected as http.HandlerFunc.
func (s *Server) RegisterRoutes(opts RouteOptions) {
	// Nil-handler guards for required routes.
	if opts.ListHandler == nil {
		panic("server: RegisterRoutes requires ListHandler")
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
	s.mux.HandleFunc("GET /{$}", opts.ListHandler)
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

// Start begins serving HTTP. Blocks until the server stops.
func (s *Server) Start() error {
	s.logger.Info("server starting", "addr", s.httpServer.Addr)
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server: %w", err)
	}
	return nil
}

// Serve begins serving HTTP on the given listener. Blocks until the server stops.
func (s *Server) Serve(ln net.Listener) error {
	s.logger.Info("server starting", "addr", ln.Addr().String())
	if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server: %w", err)
	}
	return nil
}

// Shutdown gracefully stops the server with the given context deadline.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("server shutting down")
	return s.httpServer.Shutdown(ctx)
}

// middleware chains standard middleware: logging, security headers, recovery.
func (s *Server) middleware(next http.Handler) http.Handler {
	// Order: logging (outermost) → recovery → security headers → handler
	return s.loggingMiddleware(
		s.recoveryMiddleware(
			s.securityHeadersMiddleware(next),
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
		)
	})
}

func (s *Server) securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy",
			"default-src 'none'; script-src 'none'; object-src 'none'; "+
				"connect-src 'none'; style-src 'self'; img-src 'self' https: data:; "+
				"font-src 'self' https:; base-uri 'self'; form-action 'self'; "+
				"frame-ancestors 'self'")
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

func (s *Server) readyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Cache-Control", "no-store")
	if !s.ready.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprint(w, "not ready")
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "ready")
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
