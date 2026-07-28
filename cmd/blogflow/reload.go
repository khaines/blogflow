package main

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"github.com/khaines/blogflow/internal/config"
	"github.com/khaines/blogflow/internal/content"
	"github.com/khaines/blogflow/internal/gitops"
	"github.com/khaines/blogflow/internal/search"
	"github.com/khaines/blogflow/internal/server/handlers"
	"github.com/khaines/blogflow/internal/theme"
)

// publishContentSnapshot builds the search index (when buildSearch is true) for
// a content generation and publishes the content+search snapshot atomically via
// deps. Content and search always share one generation, so a query never mixes
// generations. When buildSearch is false the snapshot's Search is nil and the
// /search route (if it exists) returns 503 until a build is published.
//
// buildSearch reflects the router-build value of search.enabled (fixed for the
// process lifetime), not the possibly-reloaded config value; searchCfg supplies
// the reloadable tunables (caps, limits) for this generation.
func publishContentSnapshot(
	ctx context.Context,
	deps *handlers.Deps,
	idx *content.Index,
	buildSearch bool,
	searchCfg config.SearchConfig,
	logger *slog.Logger,
) {
	gen := deps.NextGeneration()
	if !buildSearch {
		deps.SetSnapshot(gen, idx, nil)
		return
	}

	var oldBytes int64
	if prev := deps.LoadSearch(); prev != nil {
		oldBytes = prev.LogicalBytes()
	}

	start := time.Now()
	si := buildSearchIndexSafe(ctx, idx, searchCfg, gen, logger)
	dur := time.Since(start)

	// A nil index means the build failed (panicked). Publish the new content
	// with Search=nil so the site keeps serving; the registered /search route
	// then returns 503 until a later successful rebuild (design §2.3, §3.2 #15).
	if si == nil {
		deps.SetSnapshot(gen, idx, nil)
		search.RecordRebuildFailure(gen, dur)
		return
	}

	// Publish first so search becomes queryable, then record metrics.
	deps.SetSnapshot(gen, idx, si)
	search.ObserveRebuild(si, dur, si.LogicalBytes()+oldBytes)

	if si.Truncated() {
		logger.Warn("search index truncated",
			"reason", si.TruncationReason(),
			"indexed_docs", si.DocCount(),
			"max_docs", searchCfg.MaxDocs,
			"max_tokens", searchCfg.MaxTokens,
			"max_index_bytes", searchCfg.MaxIndexBytes,
		)
	}
	logger.Info("search index rebuilt",
		"generation", gen,
		"posts", si.DocCount(),
		"tokens", si.TokenCount(),
		"duration", dur,
		"truncated", si.Truncated(),
	)
}

// buildSearchIndexSafe builds the search index and converts a panic into a nil
// result so a search build failure degrades to content-only service instead of
// crashing the reload goroutine — the HTTP recovery middleware does not cover
// this background code path.
func buildSearchIndexSafe(ctx context.Context, idx *content.Index, cfg config.SearchConfig, gen uint64, logger *slog.Logger) (si *search.SearchIndex) {
	defer func() {
		if rec := recover(); rec != nil {
			logger.Error("search index build failed; serving content without search",
				"generation", gen,
				"panic", fmt.Sprint(rec),
			)
			si = nil
		}
	}()
	return search.Build(ctx, idx, cfg, gen)
}

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

		// 4. Rebuild search (if enabled at router build) and publish the
		// content+search snapshot atomically. search.enabled is restart-scoped:
		// route registration never changes at runtime, so we build based on the
		// router-build value while honoring reloaded search tunables.
		var searchCfg config.SearchConfig
		if cfgLoader != nil {
			cfg := cfgLoader.Get()
			searchCfg = cfg.Search
			if cfg.Search.Enabled != deps.SearchEnabled() {
				logger.Warn("search.enabled change ignored until restart",
					"router_enabled", deps.SearchEnabled(),
					"config_enabled", cfg.Search.Enabled,
				)
			}
		}
		publishContentSnapshot(context.Background(), deps, newIdx, deps.SearchEnabled(), searchCfg, logger)
		logger.Info("content reloaded", "posts", len(newIdx.Posts), "pages", len(newIdx.Pages))

		// 5. Cache flush.
		if cache != nil {
			cache.InvalidateAll()
			logger.Info("render cache flushed after content reload")
		}
		return nil
	}
}
