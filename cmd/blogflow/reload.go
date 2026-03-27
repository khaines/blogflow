package main

import (
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/khaines/blogflow/internal/config"
	"github.com/khaines/blogflow/internal/content"
	"github.com/khaines/blogflow/internal/gitops"
	"github.com/khaines/blogflow/internal/server/handlers"
	"github.com/khaines/blogflow/internal/theme"
)

// newContentReloader builds a ContentReloader that reloads config, theme
// templates, and content (in that order), then flushes the render cache.
// Config and theme reload failures are logged but do not prevent content
// from being served — partial reload is better than no reload.
func newContentReloader(
	scanner *content.Scanner,
	fsys fs.FS,
	deps *handlers.Deps,
	cache *content.Cache,
	cfgLoader *config.Loader,
	themeEngine *theme.Engine,
	logger *slog.Logger,
) gitops.ContentReloader {
	return func() error {
		// 1. Config reload — may affect content scanning behaviour.
		if cfgLoader != nil {
			if _, err := cfgLoader.Reload(); err != nil {
				logger.Error("config reload failed", "error", err)
			} else {
				logger.Info("config reloaded")
			}
		}

		// 2. Theme reload — re-parse templates from the overlay FS.
		if themeEngine != nil {
			if err := themeEngine.Reload(); err != nil {
				logger.Error("theme reload failed", "error", err)
			} else {
				logger.Info("theme reloaded")
			}
		}

		// 3. Content rescan.
		newIdx, err := scanner.Scan(fsys)
		if err != nil {
			return fmt.Errorf("content reload: %w", err)
		}
		deps.SetIndex(newIdx)
		logger.Info("content reloaded", "posts", len(newIdx.Posts), "pages", len(newIdx.Pages))

		// 4. Cache flush.
		if cache != nil {
			cache.InvalidateAll()
			logger.Info("render cache flushed after content reload")
		}
		return nil
	}
}
