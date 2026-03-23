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
		Webhook:  config.WebhookConfig{Path: "/_hook"},
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
		s, err := gitops.NewStrategy(&config.SyncConfig{Strategy: tc.strategy}, noop, logger())
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", tc.strategy, err)
		}

		if got := s.Name(); got != tc.want {
			t.Errorf("Name() = %q, want %q", got, tc.want)
		}
	}
}

func TestStrategy_StartStop(t *testing.T) {
	t.Parallel()

	strategies := []string{"watch", "webhook", "sidecar"}
	ctx := context.Background()

	for _, name := range strategies {
		s, err := gitops.NewStrategy(&config.SyncConfig{Strategy: name}, noop, logger())
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", name, err)
		}

		if err := s.Start(ctx); err != nil {
			t.Errorf("%s.Start() error: %v", name, err)
		}

		if err := s.Stop(); err != nil {
			t.Errorf("%s.Stop() error: %v", name, err)
		}
	}
}
