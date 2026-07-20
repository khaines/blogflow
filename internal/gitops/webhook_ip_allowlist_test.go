package gitops_test

import (
	"bytes"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"github.com/khaines/blogflow/internal/config"
	"github.com/khaines/blogflow/internal/gitops"
)

// testResolverIP resolves client IPs from RemoteAddr only.
type testResolverIP struct{}

func (*testResolverIP) ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || host == "" {
		host = r.RemoteAddr
	}
	return host
}

var testResolverIPIns = &testResolverIP{}

func TestWebhookHandler_IPAllowlistBlockedIP(t *testing.T) {
	t.Parallel()

	secret := "test-secure-webhook-secret-long-enough32bytes!!!"

	var called atomic.Bool
	reloader := gitops.ContentReloader(func() error {
		called.Store(true)
		return nil
	})

	w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
		Path:         "/hook",
		Secret:       secret,
		BranchFilter: "main",
		AllowedIPs:   []string{"192.168.1.1", "10.0.0.1"}, // Only these IPs allowed
	}, reloader, webhookLogger(), testResolverIPIns)
	if err != nil {
		t.Fatal(err)
	}

	payload := makePayload("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
	req.Header.Set("X-Hub-Signature-256", signPayload([]byte(secret), payload))
	req.Header.Set("X-Forwarded-For", "1.2.3.4") // Not in allowlist
	req.RemoteAddr = "1.2.3.4:12345"             // Resolver resolves RemoteAddr; set to unmatched IP

	rec := httptest.NewRecorder()
	w.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d, want %d (IP not in allowlist should be rejected)", rec.Code, http.StatusForbidden)
	}

	if called.Load() {
		t.Fatal("reloader should NOT be called for blocked IP")
	}
}

func TestWebhookHandler_IPAllowlistAllowedIP(t *testing.T) {
	t.Parallel()

	secret := "test-secure-webhook-secret-long-enough32bytes!!!"

	var called atomic.Bool
	reloader := gitops.ContentReloader(func() error {
		called.Store(true)
		return nil
	})

	w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
		Path:         "/hook",
		Secret:       secret,
		BranchFilter: "main",
		AllowedIPs:   []string{"192.168.1.1", "10.0.0.1"}, // Allow these IPs
	}, reloader, webhookLogger(), testResolverIPIns)
	if err != nil {
		t.Fatal(err)
	}

	payload := makePayload("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
	req.Header.Set("X-Hub-Signature-256", signPayload([]byte(secret), payload))
	req.Header.Set("X-Forwarded-For", "10.0.0.1") // In allowlist
	req.RemoteAddr = "10.0.0.1:12345"             // Resolver resolves RemoteAddr; set to matched IP

	rec := httptest.NewRecorder()
	w.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want %d (IP in allowlist should be accepted)", rec.Code, http.StatusOK)
	}

	if !called.Load() {
		t.Fatal("reloader should be called for allowed IP")
	}
}

func TestWebhookHandler_IPAllowlistFromConfigEnvOverrideEnforced(t *testing.T) {
	t.Setenv("BLOGFLOW_SYNC_WEBHOOK_ALLOWED_IPS", "203.0.113.10")

	cfg, err := config.NewLoader(fstest.MapFS{}).Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	secret := "test-secure-webhook-secret-long-enough32bytes!!!"
	var called atomic.Int32
	webhookCfg := cfg.Sync.Webhook
	webhookCfg.Path = "/hook"
	webhookCfg.Secret = secret
	webhookCfg.BranchFilter = "main"

	w, err := gitops.NewWebhookStrategy(webhookCfg, gitops.ContentReloader(func() error {
		called.Add(1)
		return nil
	}), webhookLogger(), testResolverIPIns)
	if err != nil {
		t.Fatal(err)
	}

	sendWithIP := func(ip string) int {
		payload := makePayload("refs/heads/main")
		req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
		req.Header.Set("X-Hub-Signature-256", signPayload([]byte(secret), payload))
		req.Header.Set("X-GitHub-Event", "push")
		req.RemoteAddr = ip + ":12345"
		rec := httptest.NewRecorder()
		w.Handler().ServeHTTP(rec, req)
		return rec.Code
	}

	if code := sendWithIP("203.0.113.10"); code != http.StatusOK {
		t.Fatalf("listed IP: got %d, want %d", code, http.StatusOK)
	}
	if code := sendWithIP("198.51.100.10"); code != http.StatusForbidden {
		t.Fatalf("unlisted IP: got %d, want %d", code, http.StatusForbidden)
	}
	if got := called.Load(); got != 1 {
		t.Fatalf("reloader calls = %d, want 1", got)
	}
}

func TestWebhookHandler_IPAllowlistEmptyAllowsAll(t *testing.T) {
	t.Parallel()

	secret := "test-secure-webhook-secret-long-enough32bytes!!!"

	var called atomic.Bool
	reloader := gitops.ContentReloader(func() error {
		called.Store(true)
		return nil
	})

	w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
		Path:         "/hook",
		Secret:       secret,
		BranchFilter: "main",
		AllowedIPs:   []string{}, // Empty = no allowlist enforcement
	}, reloader, webhookLogger(), testResolverIPIns)
	if err != nil {
		t.Fatal(err)
	}

	payload := makePayload("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
	req.Header.Set("X-Hub-Signature-256", signPayload([]byte(secret), payload))
	req.Header.Set("X-Forwarded-For", "99.99.99.99")
	req.RemoteAddr = "99.99.99.99:12345"

	rec := httptest.NewRecorder()
	w.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want %d (empty allowlist should accept all)", rec.Code, http.StatusOK)
	}

	if !called.Load() {
		t.Fatal("reloader should be called when allowlist is empty")
	}
}

func TestWebhookHandler_IPAllowlistMultipleIPs(t *testing.T) {
	t.Parallel()

	secret := "test-secure-webhook-secret-32-bytes-long-!!!-more"
	called := 0

	w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
		Path:       "/hook",
		Secret:     secret,
		AllowedIPs: []string{"172.16.0.1", "172.16.0.2", "172.16.0.3"},
	}, func() error {
		called++
		return nil
	}, webhookLogger(), testResolverIPIns)
	if err != nil {
		t.Fatal(err)
	}

	sendWithIP := func(ip string) int {
		payload := makePayload("refs/heads/main")
		req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
		req.Header.Set("X-Hub-Signature-256", signPayload([]byte(secret), payload))
		req.Header.Set("X-Forwarded-For", ip)
		req.RemoteAddr = ip + ":12345"
		rec := httptest.NewRecorder()
		w.Handler().ServeHTTP(rec, req)
		return rec.Code
	}

	// All three allowed IPs should pass
	for _, ip := range []string{"172.16.0.1", "172.16.0.2", "172.16.0.3"} {
		if code := sendWithIP(ip); code != http.StatusOK {
			t.Fatalf("IP %s: got %d, want %d", ip, code, http.StatusOK)
		}
	}

	// Unlisted IPs should be rejected
	for _, ip := range []string{"172.16.0.4", "8.8.8.8", "0.0.0.0"} {
		if code := sendWithIP(ip); code != http.StatusForbidden {
			t.Fatalf("IP %s: got %d, want %d", ip, code, http.StatusForbidden)
		}
	}

	if called != 3 {
		t.Fatalf("expected 3 reload calls, got %d", called)
	}
}

func TestWebhookHandler_IPAllowlistLogOutput(t *testing.T) {
	t.Parallel()

	var logOutput strings.Builder
	logger := slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelWarn}))

	secret := "test-secret-long-enough-to-be-valid32bytes!!!"

	w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
		Path:       "/hook",
		Secret:     secret,
		AllowedIPs: []string{"10.0.0.1"},
	}, func() error { return nil }, logger, testResolverIPIns)
	if err != nil {
		t.Fatal(err)
	}

	payload := makePayload("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
	req.Header.Set("X-Hub-Signature-256", signPayload([]byte(secret), payload))
	req.Header.Set("X-Forwarded-For", "192.168.0.1")
	req.RemoteAddr = "192.168.0.1:12345"

	rec := httptest.NewRecorder()
	w.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d, want %d", rec.Code, http.StatusForbidden)
	}

	logs := logOutput.String()
	if !strings.Contains(logs, "source IP not in allowed_ips") {
		t.Fatalf("expected 'source IP not in allowed_ips' in log, got: %s", logs)
	}
	if !strings.Contains(logs, "192.168.0.1") {
		t.Fatalf("expected blocked IP in log, got: %s", logs)
	}
}

func TestWebhookHandler_AllowedEventsFiltering(t *testing.T) {
	t.Parallel()

	secret := "valid-test-secret-long-enough-32bytes!!!" //nolint:gosec // test fixture

	t.Run("allowed_event_accepted", func(t *testing.T) {
		t.Parallel()
		called := 0
		w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
			Path:          "/hook",
			Secret:        secret,
			AllowedEvents: []string{"push"},
		}, func() error { called++; return nil }, webhookLogger(), testResolverIPIns)
		if err != nil {
			t.Fatal(err)
		}
		payload := makePayload("refs/heads/main")
		req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
		req.Header.Set("X-Hub-Signature-256", signPayload([]byte(secret), payload))
		req.Header.Set("X-GitHub-Event", "push")
		rec := httptest.NewRecorder()
		w.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("allowed event: got %d, want %d", rec.Code, http.StatusOK)
		}
		if called != 1 {
			t.Fatalf("expected 1 reload call, got %d", called)
		}
	})

	t.Run("disallowed_event_rejected", func(t *testing.T) {
		t.Parallel()
		called := 0
		w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
			Path:          "/hook",
			Secret:        secret,
			AllowedEvents: []string{"push"},
		}, func() error { called++; return nil }, webhookLogger(), testResolverIPIns)
		if err != nil {
			t.Fatal(err)
		}
		payload := makePayload("refs/heads/main")
		req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
		req.Header.Set("X-Hub-Signature-256", signPayload([]byte(secret), payload))
		req.Header.Set("X-GitHub-Event", "pull_request")
		rec := httptest.NewRecorder()
		w.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("disallowed event: got %d, want %d", rec.Code, http.StatusForbidden)
		}
		if called != 0 {
			t.Fatal("reloader should not be called for disallowed event")
		}
		if !strings.Contains(rec.Body.String(), "event type not allowed") {
			t.Errorf("body should contain event type not allowed, got: %s", rec.Body.String())
		}
	})

	t.Run("missing_event_header_rejected", func(t *testing.T) {
		t.Parallel()
		called := 0
		w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
			Path:          "/hook",
			Secret:        secret,
			AllowedEvents: []string{"push"},
		}, func() error { called++; return nil }, webhookLogger(), testResolverIPIns)
		if err != nil {
			t.Fatal(err)
		}
		payload := makePayload("refs/heads/main")
		req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
		req.Header.Set("X-Hub-Signature-256", signPayload([]byte(secret), payload))
		rec := httptest.NewRecorder()
		w.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("missing event: got %d, want %d", rec.Code, http.StatusForbidden)
		}
		if called != 0 {
			t.Fatal("reloader should not be called for missing event header")
		}
	})

	t.Run("multiple_allowed_events", func(t *testing.T) {
		t.Parallel()
		w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
			Path:          "/hook",
			Secret:        secret,
			AllowedEvents: []string{"push", "ping", "release"},
		}, func() error { return nil }, webhookLogger(), testResolverIPIns)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range []string{"push", "ping", "release"} {
			t.Run(event, func(t *testing.T) {
				payload := makePayload("refs/heads/main")
				req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
				req.Header.Set("X-Hub-Signature-256", signPayload([]byte(secret), payload))
				req.Header.Set("X-GitHub-Event", event)
				rec := httptest.NewRecorder()
				w.Handler().ServeHTTP(rec, req)
				if rec.Code != http.StatusOK {
					t.Fatalf("event %s: got %d, want %d", event, rec.Code, http.StatusOK)
				}
			})
		}
	})
}

// TestWebhookHandler_IPAllowlistCIDR verifies that AllowedIPs entries
// supporting CIDR notation actually work — a CIDR entry should allow any
// client IP within that range (the production fix that was missing from the
// prior RFL rounds).
func TestWebhookHandler_IPAllowlistCIDR(t *testing.T) {
	const (
		cidr   = "10.0.0.0/8"
		secret = "abcdefghijklmnopqrstuvwxyz123456"
	)

	secretBytes := []byte(secret)

	t.Run("CIDR_entry_allows_inside_range", func(t *testing.T) {
		reloaderCalled := atomic.Bool{}
		reloader := func() error { reloaderCalled.Store(true); return nil }

		resolver := &testIPRes{ipFn: func(*http.Request) string { return "10.0.0.1" }}
		w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
			Path:       "/hook",
			Secret:     secret,
			AllowedIPs: []string{cidr},
		}, reloader, webhookLogger(), resolver)
		if err != nil {
			t.Fatalf("NewWebhookStrategy: %v", err)
		}
		payload := makePayload("refs/heads/main")
		req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
		req.Header.Set("X-Hub-Signature-256", signPayload(secretBytes, payload))
		req.Header.Set("X-GitHub-Event", "push")
		rec := httptest.NewRecorder()
		w.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for IP inside CIDR, got %d; body: %s", rec.Code, rec.Body.String())
		}
		if !reloaderCalled.Load() {
			t.Fatal("reloader should have been called")
		}
	})

	t.Run("CIDR_entry_blocks_outside_range", func(t *testing.T) {
		reloaderCalled := atomic.Bool{}
		resolver := &testIPRes{ipFn: func(*http.Request) string { return "192.168.1.1" }}
		w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
			Path:       "/hook",
			Secret:     secret,
			AllowedIPs: []string{cidr},
		}, func() error { reloaderCalled.Store(true); return nil }, webhookLogger(), resolver)
		if err != nil {
			t.Fatalf("NewWebhookStrategy: %v", err)
		}
		payload := makePayload("refs/heads/main")
		req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
		req.Header.Set("X-Hub-Signature-256", signPayload(secretBytes, payload))
		req.Header.Set("X-GitHub-Event", "push")
		rec := httptest.NewRecorder()
		w.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for IP outside CIDR, got %d", rec.Code)
		}
		if reloaderCalled.Load() {
			t.Fatal("reloader should NOT have been called for blocked IP")
		}
	})

	t.Run("mixed_bare_IP_and_CIDR", func(t *testing.T) {
		resolver := &testIPRes{ipFn: func(*http.Request) string { return "10.0.0.2" }}
		w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
			Path:       "/hook",
			Secret:     secret,
			AllowedIPs: []string{"192.168.1.1", cidr},
		}, func() error { return nil }, webhookLogger(), resolver)
		if err != nil {
			t.Fatalf("NewWebhookStrategy: %v", err)
		}
		payload := makePayload("refs/heads/main")
		req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
		req.Header.Set("X-Hub-Signature-256", signPayload(secretBytes, payload))
		req.Header.Set("X-GitHub-Event", "push")
		rec := httptest.NewRecorder()
		w.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for IP matched by CIDR (mixed allowlist), got %d", rec.Code)
		}
	})
}

func TestWebhookStrategy_IPv6Allowlist(t *testing.T) {
	secret := "very-long-webhook-secret-minimum-32-bytes-ok"
	secretBytes := []byte(secret)

	t.Run("non_canonical_IP6_matches_bare_IP", func(t *testing.T) {
		resolver := &testIPRes{ipFn: func(*http.Request) string { return "fd00:0:0:0:0:0:0:1" }}
		w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
			Path:       "/hook",
			Secret:     secret,
			AllowedIPs: []string{"fd00::1"},
		}, func() error { return nil }, webhookLogger(), resolver)
		if err != nil {
			t.Fatalf("NewWebhookStrategy: %v", err)
		}
		payload := makePayload("refs/heads/main")
		req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
		req.Header.Set("X-Hub-Signature-256", signPayload(secretBytes, payload))
		req.Header.Set("X-GitHub-Event", "push")
		rec := httptest.NewRecorder()
		w.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("fd00:0:0:0:0:0:0:1 should match fd00::1 allowlist; got %d", rec.Code)
		}
	})

	t.Run("different_prefix_IP6_blocked", func(t *testing.T) {
		resolver := &testIPRes{ipFn: func(*http.Request) string { return "0:0:0:0:0:0:0:1" }}
		w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
			Path:       "/hook",
			Secret:     secret,
			AllowedIPs: []string{"fd00::1"},
		}, func() error { return nil }, webhookLogger(), resolver)
		if err != nil {
			t.Fatalf("NewWebhookStrategy: %v", err)
		}
		payload := makePayload("refs/heads/main")
		req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
		req.Header.Set("X-Hub-Signature-256", signPayload(secretBytes, payload))
		req.Header.Set("X-GitHub-Event", "push")
		rec := httptest.NewRecorder()
		w.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("::1 should NOT match fd00::1 allowlist; got %d", rec.Code)
		}
	})

	t.Run("IPv6_CIDR_allowlist", func(t *testing.T) {
		resolver := &testIPRes{ipFn: func(*http.Request) string { return "fd00:a:b::1" }}
		w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
			Path:       "/hook",
			Secret:     secret,
			AllowedIPs: []string{"fd00::/8"},
		}, func() error { return nil }, webhookLogger(), resolver)
		if err != nil {
			t.Fatalf("NewWebhookStrategy: %v", err)
		}
		payload := makePayload("refs/heads/main")
		req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
		req.Header.Set("X-Hub-Signature-256", signPayload(secretBytes, payload))
		req.Header.Set("X-GitHub-Event", "push")
		rec := httptest.NewRecorder()
		w.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("fd00:a:b::1 should match fd00::/8; got %d", rec.Code)
		}
	})
}

func TestWebhookHandler_NilResolverFailClosed(t *testing.T) {
	secret := "very-long-webhook-secret-minimum-32-bytes-ok"
	payload := makePayload("refs/heads/main")

	t.Run("IPv4_allowlist_passes_via_remoteaddr_when_resolver_nil", func(t *testing.T) {
		resolver := &testIPRes{ipFn: func(*http.Request) string { return "10.0.0.1" }}
		w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
			Path:       "/hook",
			Secret:     secret,
			AllowedIPs: []string{"10.0.0.1"},
		}, func() error { return nil }, webhookLogger(), resolver)
		if err != nil {
			t.Fatalf("NewWebhookStrategy: %v", err)
		}
		w.SetIPResolver(nil) // triggers fail-closed path

		req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
		req.RemoteAddr = "10.0.0.1:443"
		req.Header.Set("X-Hub-Signature-256", signPayload([]byte(secret), payload))
		req.Header.Set("X-GitHub-Event", "push")
		rec := httptest.NewRecorder()
		w.Handler().ServeHTTP(rec, req)
		// 200 means IP check passed (allowlist matched), signature matched, reload succeeded
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 for IP passed and valid sig with nil resolver; got %d", rec.Code)
		}
	})

	t.Run("IPv6_allowlist_passes_via_remoteaddr_when_resolver_nil", func(t *testing.T) {
		resolver := &testIPRes{ipFn: func(*http.Request) string { return "fd00::1" }}
		w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
			Path:       "/hook",
			Secret:     secret,
			AllowedIPs: []string{"fd00::1"},
		}, func() error { return nil }, webhookLogger(), resolver)
		if err != nil {
			t.Fatalf("NewWebhookStrategy: %v", err)
		}
		w.SetIPResolver(nil) // triggers fail-closed path

		req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
		req.RemoteAddr = "[fd00::1]:443" // IPv6 with brackets
		req.Header.Set("X-Hub-Signature-256", signPayload([]byte(secret), payload))
		req.Header.Set("X-GitHub-Event", "push")
		rec := httptest.NewRecorder()
		w.Handler().ServeHTTP(rec, req)
		// splitHostPort("[fd00::1]:443") → host="fd00::1" — should match allowlist
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 for IPv6 fd00::1 in allowlist with nil resolver; got %d", rec.Code)
		}
	})
}
