package config

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const debounceDuration = 500 * time.Millisecond

// Reload re-reads site.yaml through the configured FS, re-applies
// environment variable overrides, re-validates, and atomically stores
// the new config. On success it fires all registered OnChange callbacks.
// On validation (or parse) failure the previous config is preserved and
// the error is returned.
func (l *Loader) Reload() (*Config, error) {
	tracer := otel.Tracer("github.com/khaines/blogflow/config")
	_, span := tracer.Start(context.Background(), "config.Reload")
	defer span.End()
	span.SetAttributes(attribute.String("config.path", "site.yaml"))

	l.reloadMu.Lock()
	defer l.reloadMu.Unlock()

	cfg, err := l.Load()
	if err != nil {
		span.SetAttributes(attribute.Bool("config.success", false))
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
		return nil, err
	}
	span.SetAttributes(attribute.Bool("config.success", true))
	l.fireCallbacks(cfg)
	return cfg, nil
}

// OnChange registers a callback that is invoked after every successful
// Reload (including Watch-triggered reloads). The callback receives the
// newly loaded config. Safe for concurrent use.
func (l *Loader) OnChange(fn func(*Config)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.callbacks = append(l.callbacks, fn)
}

// Watch starts an fsnotify watcher on the config directory and reloads
// site.yaml whenever it is written or recreated. File-change events are
// debounced by 500 ms so rapid successive writes result in a single
// Reload + OnChange cycle. Watch blocks until ctx is cancelled.
//
// Requires WithWatchDir to have been passed to NewLoader.
func (l *Loader) Watch(ctx context.Context) error {
	if l.configDir == "" {
		return fmt.Errorf("config: Watch requires a watch directory (use WithWatchDir option)")
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("config: creating file watcher: %w", err)
	}
	defer func() { _ = watcher.Close() }()

	// Watch the directory (not the file directly) so we pick up
	// editor atomic-save renames and file recreations.
	if err := watcher.Add(l.configDir); err != nil {
		return fmt.Errorf("config: watching %s: %w", l.configDir, err)
	}

	configFile := filepath.Join(l.configDir, "site.yaml")
	var debounceTimer *time.Timer
	var debounceCh <-chan time.Time

	for {
		select {
		case <-ctx.Done():
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return ctx.Err()

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if filepath.Clean(event.Name) != filepath.Clean(configFile) {
				continue
			}
			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) {
				continue
			}
			// Reset debounce timer on every qualifying event.
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.NewTimer(debounceDuration)
			debounceCh = debounceTimer.C

		case <-debounceCh:
			debounceCh = nil
			// Best-effort reload: validation failures preserve old config.
			_, _ = l.Reload()

		case _, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			// TODO: surface via structured logging once slog is wired in.
		}
	}
}

func (l *Loader) fireCallbacks(cfg *Config) {
	l.mu.RLock()
	cbs := make([]func(*Config), len(l.callbacks))
	copy(cbs, l.callbacks)
	l.mu.RUnlock()

	for _, cb := range cbs {
		cb(cfg)
	}
}
