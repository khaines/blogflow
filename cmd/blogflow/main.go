// Package main is the entry point for the blogflow blog engine.
package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	blogflow "github.com/khaines/blogflow"
	"github.com/khaines/blogflow/internal/config"
	"github.com/khaines/blogflow/internal/content"
	"github.com/khaines/blogflow/internal/gitops"
	"github.com/khaines/blogflow/internal/overlayfs"
	"github.com/khaines/blogflow/internal/server"
	"github.com/khaines/blogflow/internal/server/handlers"
	"github.com/khaines/blogflow/internal/theme"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func versionString() string {
	return fmt.Sprintf("blogflow version %s (commit: %s, built: %s)", version, commit, date)
}

func main() {
	// Subcommands that must run before flag.Parse
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "healthcheck":
			os.Exit(runHealthcheck(os.Args[2:]))
		case "version":
			fmt.Println(versionString())
			os.Exit(0)
		}
	}

	// CLI flags
	contentPath := flag.String("content", "", "Path to content directory")
	themePath := flag.String("theme", "", "Path to custom theme directory")
	configPath := flag.String("config", "", "Path to config directory")
	port := flag.Int("port", 0, "HTTP port (overrides config)")
	dev := flag.Bool("dev", false, "Development mode (verbose logging)")
	showVersion := flag.Bool("version", false, "Print version and exit")
	contentRepo := flag.String("content-repo", "", "Git repository URL for content bootstrap")
	contentBranch := flag.String("content-branch", "", "Branch to clone for content bootstrap (default: main)")
	flag.Parse()

	if *showVersion {
		fmt.Println(versionString())
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

	cfgLoader := config.NewLoader(configFS, config.WithLogger(logger))
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
	if *contentRepo != "" || *contentBranch != "" {
		cfgCopy := *cfg
		if *contentRepo != "" {
			cfgCopy.Sync.Repo = *contentRepo
		}
		if *contentBranch != "" {
			cfgCopy.Sync.Branch = *contentBranch
		}
		cfg = &cfgCopy
	}
	logger.Info("configuration loaded", "port", cfg.Server.Port, "theme", cfg.Theme.Name)

	// 2b. Content bootstrap: clone repo before scanning
	if cfg.Sync.Repo != "" {
		bootstrapContent(cfg, *contentPath, logger)
	}

	// 3. Initialize content pipeline
	renderer := content.NewRenderer()
	scanner := content.NewScanner(renderer, cfg.Content.PostsDir, cfg.Content.PagesDir, cfg.Content.SummaryLength, logger)

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

	// 5. Initialize render cache (if enabled)
	var renderCache *content.Cache
	if cfg.Cache.Enabled {
		renderCache = content.NewCache(cfg.Cache)
		logger.Info("render cache initialized", "max_entries", cfg.Cache.MaxEntries, "ttl", cfg.Cache.TTL)
	}

	// 6. Create and configure HTTP server
	srv := server.New(cfg, logger)

	// 7. Build handler dependencies
	deps := handlers.NewDeps(cfg, idx, themeEngine)

	// 8. Content reloader for sync strategies
	reloader := newContentReloader(scanner, contentOverlay, deps, renderCache, logger)

	// 9. Initialize sync strategy
	syncStrategy, err := gitops.NewStrategy(&cfg.Sync, reloader, logger)
	if err != nil {
		logger.Error("failed to create sync strategy", "error", err)
		os.Exit(1)
	}

	// Wire up content directory for file-watching sync
	if ws, ok := syncStrategy.(*gitops.WatchStrategy); ok && *contentPath != "" {
		ws.SetDirs(*contentPath)
	}

	// 10. Register routes
	staticFS, fsErr := fs.Sub(contentOverlay, "static")
	if fsErr != nil {
		logger.Warn("static directory not available — /static/ routes will 404", "error", fsErr)
		staticFS = nil
	}

	feedHandler := handlers.NewFeedHandler(cfg, idx)
	sitemapHandler := handlers.NewSitemapHandler(cfg, idx)

	routeOpts := server.RouteOptions{
		ListHandler:    handlers.ListHandler(deps),
		PostHandler:    handlers.PostHandler(deps),
		PageHandler:    handlers.PageHandler(deps),
		TagHandler:     handlers.TagHandler(deps),
		FeedHandler:    feedHandler.ServeHTTP,
		SitemapHandler: sitemapHandler.ServeHTTP,
		StaticFS:       staticFS,
	}
	if ws, ok := syncStrategy.(*gitops.WebhookStrategy); ok {
		routeOpts.WebhookHandler = ws.Handler()
	}
	srv.RegisterRoutes(routeOpts)
	srv.SetContentChecker(deps)

	// 11. Start sync strategy (non-fatal: a blog with embedded defaults has nothing to watch)
	syncCtx, syncCancel := context.WithCancel(context.Background())
	if err := syncStrategy.Start(syncCtx); err != nil {
		logger.Warn("sync strategy disabled — content will not auto-reload",
			"strategy", syncStrategy.Name(), "reason", err)
	} else {
		logger.Info("sync strategy started", "strategy", syncStrategy.Name())
	}

	// 12. Graceful shutdown on SIGINT/SIGTERM
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		logger.Info("shutdown signal received", "signal", sig)

		srv.SetReady(false)

		syncCancel()
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		if stopErr := syncStrategy.Stop(stopCtx); stopErr != nil {
			logger.Error("sync strategy stop error", "error", stopErr)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			logger.Error("shutdown error", "error", err)
		}
	}()

	// 13. Start server and mark ready after bind confirmation
	logger.Info("server starting", "addr", fmt.Sprintf(":%d", cfg.Server.Port))

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	// Wait for listener to bind or an immediate failure
	select {
	case err := <-errCh:
		logger.Error("server failed to start", "error", err)
		os.Exit(1)
	case <-srv.Ready():
		srv.SetReady(true)
		logger.Info("server ready")
	case <-time.After(5 * time.Second):
		logger.Error("server did not become ready within timeout")
		os.Exit(1)
	}

	// Wait for server to finish (shutdown or error)
	if err := <-errCh; err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
	logger.Info("server stopped")
}
