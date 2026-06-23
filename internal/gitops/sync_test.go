package gitops_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/khaines/blogflow/internal/config"
	"github.com/khaines/blogflow/internal/gitops"
)

var noop gitops.ContentReloader = func() error { return nil }

func logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// testIPRes is a simple test IP resolver that returns a fixed IP.
type testIPRes struct {
	ipFn func(*http.Request) string
}

func (r *testIPRes) ClientIP(req *http.Request) string {
	return r.ipFn(req)
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

	res := &testIPRes{
		ipFn: func(*http.Request) string { return "10.0.0.1" },
	}
	cfg := &config.SyncConfig{
		Strategy: "webhook",
		Webhook:  config.WebhookConfig{Path: "/_hook", Secret: "test-secret-minimum-32-bytes-required!!!"},
	}

	s, err := gitops.NewStrategy(cfg, noop, logger(), res)
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

func TestNewStrategy_Poll(t *testing.T) {
	t.Parallel()

	cfg := &config.SyncConfig{
		Strategy:     "poll",
		PollInterval: "5m",
	}

	s, err := gitops.NewStrategy(cfg, noop, logger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := s.(*gitops.PollStrategy); !ok {
		t.Fatalf("expected *PollStrategy, got %T", s)
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

	res := &testIPRes{
		ipFn: func(*http.Request) string { return "10.0.0.1" },
	}

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

			_, err := gitops.NewStrategy(cfg, noop, logger(), res)
			if err == nil {
				t.Fatalf("expected error for path %q, got nil", tc.path)
			}
		})
	}
}

func TestStrategy_Name(t *testing.T) {
	t.Parallel()

	res := &testIPRes{
		ipFn: func(*http.Request) string { return "10.0.0.1" },
	}

	cases := []struct {
		strategy string
		want     string
	}{
		{"watch", "watch"},
		{"webhook", "webhook"},
		{"sidecar", "sidecar"},
		{"poll", "poll"},
	}

	for _, tc := range cases {
		t.Run(tc.strategy, func(t *testing.T) {
			t.Parallel()

			cfg := &config.SyncConfig{Strategy: tc.strategy}
			if tc.strategy == "webhook" {
				cfg.Webhook = config.WebhookConfig{Path: "/_hook", Secret: "test-secret-minimum-32-bytes-required!!!"}
			}
			if tc.strategy == "poll" {
				cfg.PollInterval = "5m"
			}

			var s gitops.Strategy
			var err error
			if tc.strategy == "webhook" {
				s, err = gitops.NewStrategy(cfg, noop, logger(), res)
			} else {
				s, err = gitops.NewStrategy(cfg, noop, logger())
			}
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

	res := &testIPRes{
		ipFn: func(*http.Request) string { return "10.0.0.1" },
	}

	strategies := []struct {
		name  string
		cfg   *config.SyncConfig
		needs bool
	}{
		{"watch", &config.SyncConfig{Strategy: "watch"}, false},
		{"webhook", &config.SyncConfig{Strategy: "webhook", Webhook: config.WebhookConfig{Path: "/_hook", Secret: "test-secret-minimum-32-bytes-required!!!"}}, true},
		{"sidecar", &config.SyncConfig{Strategy: "sidecar"}, false},
		{"poll", &config.SyncConfig{Strategy: "poll", PollInterval: "5m"}, false},
	}

	for _, tc := range strategies {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			var s gitops.Strategy
			var err error
			if tc.needs {
				s, err = gitops.NewStrategy(tc.cfg, noop, logger(), res)
			} else {
				s, err = gitops.NewStrategy(tc.cfg, noop, logger())
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.name, err)
			}

			if ws, ok := s.(*gitops.WatchStrategy); ok {
				ws.SetDirs(t.TempDir())
			}
			if ss, ok := s.(*gitops.SidecarStrategy); ok {
				ss.SetDir(t.TempDir())
			}
			if ps, ok := s.(*gitops.PollStrategy); ok {
				ps.SetPuller(&fakePuller{}, "https://example.com/repo.git", "main", t.TempDir())
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

func TestNewWebhookStrategy_SecretBoundary(t *testing.T) {
	t.Parallel()

	reloader := func() error { return nil }

	cases := []struct {
		name     string
		secret   string
		wantErr  bool
	}{
		{"31_bytes", strings.Repeat("a", 31), true},
		{"32_bytes", strings.Repeat("a", 32), false},
		{"64_bytes", strings.Repeat("a", 64), false},
	}

	testResolver := &testIPRes{ipFn: func(*http.Request) string { return "10.0.0.1" }}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var err error
			if tc.wantErr {
				_, err = gitops.NewWebhookStrategy(config.WebhookConfig{
					Path:   "/hook",
					Secret: tc.secret,
				}, reloader, logger(), nil)
			} else {
				_, err = gitops.NewWebhookStrategy(config.WebhookConfig{
					Path:   "/hook",
					Secret: tc.secret,
				}, reloader, logger(), testResolver)
			}
			if (err != nil) != tc.wantErr {
				t.Errorf("NewWebhookStrategy(secret=%q) error = %v, wantErr = %v",
					tc.secret, err, tc.wantErr)
			}
		})
	}
}

func TestStrategy_DoubleStop(t *testing.T) {
	t.Parallel()

	res := &testIPRes{
		ipFn: func(*http.Request) string { return "10.0.0.1" },
	}

	strategies := []struct {
		name  string
		cfg   *config.SyncConfig
		needs bool
	}{
		{"watch", &config.SyncConfig{Strategy: "watch"}, false},
		{"webhook", &config.SyncConfig{Strategy: "webhook", Webhook: config.WebhookConfig{Path: "/_hook", Secret: "test-secret-minimum-32-bytes-required!!!"}}, true},
		{"sidecar", &config.SyncConfig{Strategy: "sidecar"}, false},
		{"poll", &config.SyncConfig{Strategy: "poll", PollInterval: "5m"}, false},
	}

	for _, tc := range strategies {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			var s gitops.Strategy
			var err error
			if tc.needs {
				s, err = gitops.NewStrategy(tc.cfg, noop, logger(), res)
			} else {
				s, err = gitops.NewStrategy(tc.cfg, noop, logger())
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.name, err)
			}

			if ws, ok := s.(*gitops.WatchStrategy); ok {
				ws.SetDirs(t.TempDir())
			}
			if ss, ok := s.(*gitops.SidecarStrategy); ok {
				ss.SetDir(t.TempDir())
			}
			if ps, ok := s.(*gitops.PollStrategy); ok {
				ps.SetPuller(&fakePuller{}, "https://example.com/repo.git", "main", t.TempDir())
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
