package gitops_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/kenhaines/blogflow/internal/config"
	"github.com/kenhaines/blogflow/internal/gitops"
)

var noop gitops.ContentReloader = func() error { return nil }

func logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNewStrategy_Watch(t *testing.T) {
	t.Parallel()

	s, err := gitops.NewStrategy(&config.SyncConfig{Strategy: "watch"}, noop, logger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := s.(*gitops.WatchStrategy); !ok {
		t.Fatalf("expected *WatchStrategy, got %T", s)
	}
}

func TestNewStrategy_Webhook(t *testing.T) {
	t.Parallel()

	cfg := &config.SyncConfig{
		Strategy: "webhook",
		Webhook:  config.WebhookConfig{Path: "/_hook", Secret: "test-secret"},
	}

	s, err := gitops.NewStrategy(cfg, noop, logger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := s.(*gitops.WebhookStrategy); !ok {
		t.Fatalf("expected *WebhookStrategy, got %T", s)
	}
}

func TestNewStrategy_Sidecar(t *testing.T) {
	t.Parallel()

	s, err := gitops.NewStrategy(&config.SyncConfig{Strategy: "sidecar"}, noop, logger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := s.(*gitops.SidecarStrategy); !ok {
		t.Fatalf("expected *SidecarStrategy, got %T", s)
	}
}

func TestNewStrategy_Unknown(t *testing.T) {
	t.Parallel()

	_, err := gitops.NewStrategy(&config.SyncConfig{Strategy: "magic"}, noop, logger())
	if err == nil {
		t.Fatal("expected error for unknown strategy, got nil")
	}
}

func TestNewStrategy_NilConfig(t *testing.T) {
	t.Parallel()

	_, err := gitops.NewStrategy(nil, noop, logger())
	if err == nil {
		t.Fatal("expected error for nil config, got nil")
	}
}

func TestNewStrategy_NilReloader(t *testing.T) {
	t.Parallel()

	_, err := gitops.NewStrategy(&config.SyncConfig{Strategy: "watch"}, nil, logger())
	if err == nil {
		t.Fatal("expected error for nil reloader, got nil")
	}
}

func TestNewStrategy_WebhookInvalidPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		path string
	}{
		{"empty", ""},
		{"no_leading_slash", "hook"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := &config.SyncConfig{
				Strategy: "webhook",
				Webhook:  config.WebhookConfig{Path: tc.path},
			}

			_, err := gitops.NewStrategy(cfg, noop, logger())
			if err == nil {
				t.Fatalf("expected error for path %q, got nil", tc.path)
			}
		})
	}
}

func TestStrategy_Name(t *testing.T) {
	t.Parallel()

	cases := []struct {
		strategy string
		want     string
	}{
		{"watch", "watch"},
		{"webhook", "webhook"},
		{"sidecar", "sidecar"},
	}

	for _, tc := range cases {
		t.Run(tc.strategy, func(t *testing.T) {
			t.Parallel()

			cfg := &config.SyncConfig{Strategy: tc.strategy}
			if tc.strategy == "webhook" {
				cfg.Webhook = config.WebhookConfig{Path: "/_hook", Secret: "test-secret"}
			}

			s, err := gitops.NewStrategy(cfg, noop, logger())
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.strategy, err)
			}

			if got := s.Name(); got != tc.want {
				t.Errorf("Name() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStrategy_StartStop(t *testing.T) {
	t.Parallel()

	strategies := []struct {
		name string
		cfg  *config.SyncConfig
	}{
		{"watch", &config.SyncConfig{Strategy: "watch"}},
		{"webhook", &config.SyncConfig{Strategy: "webhook", Webhook: config.WebhookConfig{Path: "/_hook", Secret: "test-secret"}}},
		{"sidecar", &config.SyncConfig{Strategy: "sidecar"}},
	}

	for _, tc := range strategies {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			s, err := gitops.NewStrategy(tc.cfg, noop, logger())
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.name, err)
			}

			if ws, ok := s.(*gitops.WatchStrategy); ok {
				ws.SetDirs(t.TempDir())
			}

			if err := s.Start(ctx); err != nil {
				t.Errorf("%s.Start() error: %v", tc.name, err)
			}

			if err := s.Stop(ctx); err != nil {
				t.Errorf("%s.Stop() error: %v", tc.name, err)
			}
		})
	}
}

func TestStrategy_DoubleStop(t *testing.T) {
	t.Parallel()

	strategies := []struct {
		name string
		cfg  *config.SyncConfig
	}{
		{"watch", &config.SyncConfig{Strategy: "watch"}},
		{"webhook", &config.SyncConfig{Strategy: "webhook", Webhook: config.WebhookConfig{Path: "/_hook", Secret: "test-secret"}}},
		{"sidecar", &config.SyncConfig{Strategy: "sidecar"}},
	}

	for _, tc := range strategies {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			s, err := gitops.NewStrategy(tc.cfg, noop, logger())
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.name, err)
			}

			if ws, ok := s.(*gitops.WatchStrategy); ok {
				ws.SetDirs(t.TempDir())
			}

			if err := s.Start(ctx); err != nil {
				t.Fatalf("%s.Start() error: %v", tc.name, err)
			}

			if err := s.Stop(ctx); err != nil {
				t.Fatalf("%s.Stop() first call error: %v", tc.name, err)
			}

			if err := s.Stop(ctx); err != nil {
				t.Errorf("%s.Stop() second call error: %v", tc.name, err)
			}
		})
	}
}
