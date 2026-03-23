// Package main is the entry point for the blogflow blog engine.
package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	blogflow "github.com/kenhaines/blogflow"
	"github.com/kenhaines/blogflow/internal/config"
	"github.com/kenhaines/blogflow/internal/content"
	"github.com/kenhaines/blogflow/internal/overlayfs"
	"github.com/kenhaines/blogflow/internal/server"
	"github.com/kenhaines/blogflow/internal/theme"
)

var version = "dev"

func main() {
	// CLI flags
	contentPath := flag.String("content", "", "Path to content directory")
	themePath := flag.String("theme", "", "Path to custom theme directory")
	configPath := flag.String("config", "", "Path to config directory")
	port := flag.Int("port", 0, "HTTP port (overrides config)")
	dev := flag.Bool("dev", false, "Development mode (verbose logging)")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("blogflow %s\n", version)
		os.Exit(0)
	}

	// Logger
	logLevel := slog.LevelInfo
	if *dev {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)

	logger.Info("blogflow starting", "version", version)

	// 1. Build overlay FS for content (4-layer: theme → content → config → defaults)
	contentOverlay, err := overlayfs.NewFromPaths(*themePath, *contentPath, *configPath, blogflow.Defaults)
	if err != nil {
		logger.Error("failed to create overlay filesystem", "error", err)
		os.Exit(1)
	}
	logger.Info("overlay filesystem initialized", "layers", contentOverlay.LayerCount())

	// 2. Load configuration (2-layer: config → defaults only)
	var configFS fs.FS
	if *configPath != "" {
		configOverlay, cfgErr := overlayfs.NewFromPaths("", "", *configPath, blogflow.Defaults)
		if cfgErr != nil {
			logger.Error("failed to create config overlay", "error", cfgErr)
			os.Exit(1)
		}
		configFS = configOverlay
	} else {
		configFS = blogflow.Defaults
	}

	cfgLoader := config.NewLoader(configFS)
	if _, err := cfgLoader.Load(); err != nil {
		logger.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}
	cfg := cfgLoader.Get()

	// CLI flag overrides
	if *port > 0 {
		// Copy to avoid mutating the atomic-pointer-backed config
		cfgCopy := *cfg
		cfgCopy.Server.Port = *port
		cfg = &cfgCopy
	}
	logger.Info("configuration loaded", "port", cfg.Server.Port, "theme", cfg.Theme.Name)

	// 3. Initialize content pipeline
	renderer := content.NewRenderer()
	scanner := content.NewScanner(renderer, cfg.Content.PostsDir, cfg.Content.PagesDir, cfg.Content.SummaryLength)

	idx, err := scanner.Scan(contentOverlay)
	if err != nil {
		logger.Error("failed to scan content", "error", err)
		os.Exit(1)
	}
	logger.Info("content scanned", "posts", len(idx.Posts), "pages", len(idx.Pages))

	// 4. Initialize theme engine
	themeEngine, err := theme.NewEngine(contentOverlay)
	if err != nil {
		logger.Error("failed to initialize theme engine", "error", err)
		os.Exit(1)
	}
	logger.Info("theme engine initialized")

	// Suppress unused warnings — real handlers PR will consume these.
	_ = idx
	_ = themeEngine

	// 5. Create and configure HTTP server
	srv := server.New(cfg, logger)

	// 6. Register routes (placeholder handlers until content-handlers PR)
	staticFS, fsErr := fs.Sub(contentOverlay, "static")
	if fsErr != nil {
		logger.Warn("static directory not available — /static/ routes will 404", "error", fsErr)
		staticFS = nil
	}

	srv.RegisterRoutes(server.RouteOptions{
		ListHandler:    placeholderHandler("list", logger),
		PostHandler:    placeholderHandler("post", logger),
		PageHandler:    placeholderHandler("page", logger),
		TagHandler:     placeholderHandler("tag", logger),
		FeedHandler:    placeholderHandler("feed", logger),
		SitemapHandler: placeholderHandler("sitemap", logger),
		StaticFS:       staticFS,
	})

	// 7. Graceful shutdown on SIGINT/SIGTERM
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		logger.Info("shutdown signal received", "signal", sig)

		srv.SetReady(false)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			logger.Error("shutdown error", "error", err)
		}
	}()

	// 8. Start server and mark ready after bind confirmation
	logger.Info("server starting", "addr", fmt.Sprintf(":%d", cfg.Server.Port))

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	// Brief wait to detect immediate bind failures
	select {
	case err := <-errCh:
		logger.Error("server failed to start", "error", err)
		os.Exit(1)
	case <-time.After(100 * time.Millisecond):
		srv.SetReady(true)
		logger.Info("server ready")
	}

	// Wait for server to finish (shutdown or error)
	if err := <-errCh; err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
	logger.Info("server stopped")
}

func placeholderHandler(name string, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprintf(w, "blogflow: %s handler (placeholder)\n", name)
	}
}
