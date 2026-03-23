package gitops_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kenhaines/blogflow/internal/config"
	"github.com/kenhaines/blogflow/internal/gitops"
)

func signPayload(secret, payload []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func webhookLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func makePayload(ref string) []byte {
	b, _ := json.Marshal(map[string]string{"ref": ref})
	return b
}

func TestWebhookHandler_ValidSignature(t *testing.T) {
	t.Parallel()

	secret := "test-secret"

	var called atomic.Bool
	reloader := gitops.ContentReloader(func() error {
		called.Store(true)
		return nil
	})

	w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
		Path:         "/hook",
		Secret:       secret,
		BranchFilter: "main",
	}, reloader, webhookLogger())
	if err != nil {
		t.Fatal(err)
	}

	payload := makePayload("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
	req.Header.Set("X-Hub-Signature-256", signPayload([]byte(secret), payload))

	rec := httptest.NewRecorder()
	w.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if !called.Load() {
		t.Fatal("reloader was not called")
	}
}

func TestWebhookHandler_InvalidSignature(t *testing.T) {
	t.Parallel()

	var called atomic.Bool
	reloader := gitops.ContentReloader(func() error {
		called.Store(true)
		return nil
	})

	w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
		Path:         "/hook",
		Secret:       "correct-secret",
		BranchFilter: "main",
	}, reloader, webhookLogger())
	if err != nil {
		t.Fatal(err)
	}

	payload := makePayload("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
	req.Header.Set("X-Hub-Signature-256", signPayload([]byte("wrong-secret"), payload))

	rec := httptest.NewRecorder()
	w.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	if called.Load() {
		t.Fatal("reloader should not have been called")
	}
}

func TestWebhookHandler_MissingSignature(t *testing.T) {
	t.Parallel()

	var called atomic.Bool
	reloader := gitops.ContentReloader(func() error {
		called.Store(true)
		return nil
	})

	w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
		Path:         "/hook",
		Secret:       "secret",
		BranchFilter: "main",
	}, reloader, webhookLogger())
	if err != nil {
		t.Fatal(err)
	}

	payload := makePayload("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
	// No X-Hub-Signature-256 header set.

	rec := httptest.NewRecorder()
	w.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	if called.Load() {
		t.Fatal("reloader should not have been called")
	}
}

func TestWebhookHandler_WrongBranch(t *testing.T) {
	t.Parallel()

	var called atomic.Bool
	reloader := gitops.ContentReloader(func() error {
		called.Store(true)
		return nil
	})

	secret := "secret"

	w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
		Path:         "/hook",
		Secret:       secret,
		BranchFilter: "main",
	}, reloader, webhookLogger())
	if err != nil {
		t.Fatal(err)
	}

	payload := makePayload("refs/heads/develop")
	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
	req.Header.Set("X-Hub-Signature-256", signPayload([]byte(secret), payload))

	rec := httptest.NewRecorder()
	w.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want %d", rec.Code, http.StatusOK)
	}

	if called.Load() {
		t.Fatal("reloader should not have been called for wrong branch")
	}
}

func TestWebhookHandler_CorrectBranch(t *testing.T) {
	t.Parallel()

	var called atomic.Bool
	reloader := gitops.ContentReloader(func() error {
		called.Store(true)
		return nil
	})

	secret := "secret"

	w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
		Path:         "/hook",
		Secret:       secret,
		BranchFilter: "production",
	}, reloader, webhookLogger())
	if err != nil {
		t.Fatal(err)
	}

	payload := makePayload("refs/heads/production")
	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
	req.Header.Set("X-Hub-Signature-256", signPayload([]byte(secret), payload))

	rec := httptest.NewRecorder()
	w.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want %d", rec.Code, http.StatusOK)
	}

	if !called.Load() {
		t.Fatal("reloader was not called for matching branch")
	}
}

func TestWebhookHandler_BodyTooLarge(t *testing.T) {
	t.Parallel()

	w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
		Path:   "/hook",
		Secret: "secret",
	}, func() error { return nil }, webhookLogger())
	if err != nil {
		t.Fatal(err)
	}

	// Payload larger than 1 MB.
	largeBody := strings.NewReader(strings.Repeat("x", 1<<20+1))
	req := httptest.NewRequest(http.MethodPost, "/hook", largeBody)
	req.Header.Set("X-Hub-Signature-256", "sha256=bogus")

	rec := httptest.NewRecorder()
	w.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("got %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestWebhookHandler_EmptySecret(t *testing.T) {
	t.Parallel()

	_, err := gitops.NewWebhookStrategy(config.WebhookConfig{
		Path: "/hook",
	}, func() error { return nil }, webhookLogger())
	if err == nil {
		t.Fatal("expected error for empty secret")
	}
}

func TestWebhookHandler_RateLimited(t *testing.T) {
	t.Parallel()

	secret := "test-secret"

	var calls atomic.Int64
	reloader := gitops.ContentReloader(func() error {
		calls.Add(1)
		return nil
	})

	w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
		Path:      "/hook",
		Secret:    secret,
		RateLimit: 2,
	}, reloader, webhookLogger())
	if err != nil {
		t.Fatal(err)
	}

	payload := makePayload("refs/heads/main")
	sig := signPayload([]byte(secret), payload)

	sendRequest := func() int {
		req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
		req.Header.Set("X-Hub-Signature-256", sig)
		rec := httptest.NewRecorder()
		w.Handler().ServeHTTP(rec, req)
		return rec.Code
	}

	// First two requests should succeed.
	for i := range 2 {
		if code := sendRequest(); code != http.StatusOK {
			t.Fatalf("request %d: got %d, want %d", i+1, code, http.StatusOK)
		}
	}

	// Third request should be rate-limited.
	if code := sendRequest(); code != http.StatusTooManyRequests {
		t.Fatalf("request 3: got %d, want %d", code, http.StatusTooManyRequests)
	}

	if got := calls.Load(); got != 2 {
		t.Fatalf("reloader called %d times, want 2", got)
	}
}

func TestWebhookHandler_InvalidPath(t *testing.T) {
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

			_, err := gitops.NewWebhookStrategy(config.WebhookConfig{
				Path:   tc.path,
				Secret: "secret",
			}, func() error { return nil }, webhookLogger())
			if err == nil {
				t.Fatalf("expected error for path %q", tc.path)
			}
		})
	}
}

func TestWebhookHandler_XForwardedFor(t *testing.T) {
	secret := []byte("test-secret-min-32-bytes-long!!!!")
	called := 0
	reloader := func() error { called++; return nil }

	cfg := config.WebhookConfig{
		Path:         "/api/webhook",
		Secret:       string(secret),
		BranchFilter: "main",
		RateLimit:    1,
	}

	ws, err := gitops.NewWebhookStrategy(cfg, reloader, slog.Default())
	if err != nil {
		t.Fatal(err)
	}

	handler := ws.Handler()
	payload := []byte(`{"ref":"refs/heads/main"}`)
	sig := signPayload(secret, payload)

	// First request from "10.0.0.1" via XFF — should pass
	req1 := httptest.NewRequest(http.MethodPost, "/api/webhook", bytes.NewReader(payload))
	req1.Header.Set("X-Hub-Signature-256", sig)
	req1.Header.Set("X-Forwarded-For", "10.0.0.1")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first XFF request: expected 200, got %d", rec1.Code)
	}

	// Second from same XFF IP — rate limited
	req2 := httptest.NewRequest(http.MethodPost, "/api/webhook", bytes.NewReader(payload))
	req2.Header.Set("X-Hub-Signature-256", sig)
	req2.Header.Set("X-Forwarded-For", "10.0.0.1")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("same XFF IP: expected 429, got %d", rec2.Code)
	}

	// Third from different XFF IP — should pass
	req3 := httptest.NewRequest(http.MethodPost, "/api/webhook", bytes.NewReader(payload))
	req3.Header.Set("X-Hub-Signature-256", sig)
	req3.Header.Set("X-Forwarded-For", "10.0.0.2")
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("different XFF IP: expected 200, got %d", rec3.Code)
	}
}
