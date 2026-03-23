package main

import (
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/khaines/blogflow/internal/content"
	"github.com/khaines/blogflow/internal/gitops"
	"github.com/khaines/blogflow/internal/server/handlers"
)

// newContentReloader builds a ContentReloader that re-scans content from fsys
// and flushes the render cache (if non-nil). The deps.Index pointer is updated
// in place so existing HTTP handlers see fresh content on subsequent requests.
func newContentReloader(
	scanner *content.Scanner,
	fsys fs.FS,
	deps *handlers.Deps,
	cache *content.Cache,
	logger *slog.Logger,
) gitops.ContentReloader {
	return func() error {
		newIdx, err := scanner.Scan(fsys)
		if err != nil {
			return fmt.Errorf("content reload: %w", err)
		}
		deps.Index = newIdx
		logger.Info("content reloaded", "posts", len(newIdx.Posts), "pages", len(newIdx.Pages))

		if cache != nil {
			cache.InvalidateAll()
			slog.Info("render cache flushed after content reload")
		}
		return nil
	}
}
