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
