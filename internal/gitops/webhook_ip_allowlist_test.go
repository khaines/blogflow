package gitops_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/khaines/blogflow/internal/config"
	"github.com/khaines/blogflow/internal/gitops"
)

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
	}, reloader, webhookLogger())
	if err != nil {
		t.Fatal(err)
	}

	payload := makePayload("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
	req.Header.Set("X-Hub-Signature-256", signPayload([]byte(secret), payload))
	req.Header.Set("X-Forwarded-For", "1.2.3.4") // Not in allowlist

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
	}, reloader, webhookLogger())
	if err != nil {
		t.Fatal(err)
	}

	payload := makePayload("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
	req.Header.Set("X-Hub-Signature-256", signPayload([]byte(secret), payload))
	req.Header.Set("X-Forwarded-For", "10.0.0.1") // In allowlist

	rec := httptest.NewRecorder()
	w.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want %d (IP in allowlist should be accepted)", rec.Code, http.StatusOK)
	}

	if !called.Load() {
		t.Fatal("reloader should be called for allowed IP")
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
	}, reloader, webhookLogger())
	if err != nil {
		t.Fatal(err)
	}

	payload := makePayload("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
	req.Header.Set("X-Hub-Signature-256", signPayload([]byte(secret), payload))
	req.Header.Set("X-Forwarded-For", "99.99.99.99")

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
	}, webhookLogger())
	if err != nil {
		t.Fatal(err)
	}

	sendWithIP := func(ip string) int {
		payload := makePayload("refs/heads/main")
		req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
		req.Header.Set("X-Hub-Signature-256", signPayload([]byte(secret), payload))
		req.Header.Set("X-Forwarded-For", ip)
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
	}, func() error { return nil }, logger)
	if err != nil {
		t.Fatal(err)
	}

	payload := makePayload("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
	req.Header.Set("X-Hub-Signature-256", signPayload([]byte(secret), payload))
	req.Header.Set("X-Forwarded-For", "192.168.0.1")

	rec := httptest.NewRecorder()
	w.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d, want %d", rec.Code, http.StatusForbidden)
	}

	logs := logOutput.String()
	if !strings.Contains(logs, "source IP not in ip_allowlist") {
		t.Fatalf("expected 'source IP not in ip_allowlist' in log, got: %s", logs)
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
		}, func() error { called++; return nil }, webhookLogger())
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
		}, func() error { called++; return nil }, webhookLogger())
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
		}, func() error { called++; return nil }, webhookLogger())
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
			AllowedEvents: []string{"push", "schedule", "release"},
		}, func() error { return nil }, webhookLogger())
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range []string{"push", "schedule", "release"} {
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
