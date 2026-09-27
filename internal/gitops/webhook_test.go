package gitops_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/khaines/blogflow/internal/config"
	"github.com/khaines/blogflow/internal/gitops"
)

// testIPResolverWL resolves client IPs from RemoteAddr only.
type testIPResolverWL struct{}

func (*testIPResolverWL) ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || host == "" {
		host = r.RemoteAddr
	}
	return host
}

var testResWL = &testIPResolverWL{}

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

	secret := "test-secret-for-minimum-32-bytes-ok!"

	var called atomic.Bool
	reloader := gitops.ContentReloader(func() error {
		called.Store(true)
		return nil
	})

	w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
		Path:         "/hook",
		Secret:       secret,
		BranchFilter: "main",
	}, reloader, webhookLogger(), testResWL)
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

	secret := "correct-secret-for-minimum-32-bytes-ok!"

	var called atomic.Bool
	reloader := gitops.ContentReloader(func() error {
		called.Store(true)
		return nil
	})

	w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
		Path:         "/hook",
		Secret:       secret,
		BranchFilter: "main",
	}, reloader, webhookLogger(), testResWL)
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

	secret := "secret-for-minimum-32-bytes-ok-aaaaaa!"

	w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
		Path:         "/hook",
		Secret:       secret,
		BranchFilter: "main",
	}, reloader, webhookLogger(), testResWL)
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

	secret := "secret-for-minimum-32-bytes-ok-bbbbb!"

	w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
		Path:         "/hook",
		Secret:       "secret-for-minimum-32-bytes-ok-bbbbb!",
		BranchFilter: "main",
	}, reloader, webhookLogger(), testResWL)
	if err != nil {
		t.Fatal(err)
	}

	payload := makePayload("refs/heads/develop")
	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
	req.Header.Set("X-Hub-Signature-256", signPayload([]byte(secret), payload))
	req.RemoteAddr = "1.2.3.4:1234"

	rec := httptest.NewRecorder()
	w.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("got %d, want %d", rec.Code, http.StatusAccepted)
	}

	if rec.Header().Get("X-Blogflow-Branch-Skipped") == "" {
		t.Error("expected X-Blogflow-Branch-Skipped header")
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

	secret := "secret-for-webhook-production-test-res-32bytes!!"

	w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
		Path:         "/hook",
		Secret:       secret,
		BranchFilter: "production",
	}, reloader, webhookLogger(), testResWL)
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
		Secret: "x-secret-for-minimum-32-bytes-ok-aaaaaaa",
	}, func() error { return nil }, webhookLogger(), testResWL)
	if err != nil {
		t.Fatal(err)
	}

	// Payload larger than default 1 MB limit.
	largeBody := strings.NewReader(strings.Repeat("x", 1<<20+1))
	req := httptest.NewRequest(http.MethodPost, "/hook", largeBody)
	req.Header.Set("X-Hub-Signature-256", wellFormedBogusSig) // passes the pre-read shape check

	rec := httptest.NewRecorder()
	w.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("got %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestWebhookHandler_BodyTooLarge_CustomLimit(t *testing.T) {
	t.Parallel()

	w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
		Path:        "/hook",
		Secret:      "x-secret-for-minimum-32-bytes-ok-bbbbbbbb!",
		MaxBodySize: 256, // 256 bytes
	}, func() error { return nil }, webhookLogger(), testResWL)
	if err != nil {
		t.Fatal(err)
	}

	// Payload exceeds custom 256-byte limit.
	largeBody := strings.NewReader(strings.Repeat("x", 512))
	req := httptest.NewRequest(http.MethodPost, "/hook", largeBody)
	req.Header.Set("X-Hub-Signature-256", wellFormedBogusSig) // passes the pre-read shape check

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
	}, func() error { return nil }, webhookLogger(), testResWL)
	if err == nil {
		t.Fatal("expected error for empty secret")
	}
}

func TestWebhookHandler_RateLimited(t *testing.T) {
	t.Parallel()

	secret := "test-secret-for-minimum-32-bytes-ok!"

	var calls atomic.Int64
	reloader := gitops.ContentReloader(func() error {
		calls.Add(1)
		return nil
	})

	w, err := gitops.NewWebhookStrategy(config.WebhookConfig{
		Path:      "/hook",
		Secret:    secret,
		RateLimit: 2,
	}, reloader, webhookLogger(), testResWL)
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
				Secret: "x-secret-for-minimum-32-bytes-ok-cccccccc!",
			}, func() error { return nil }, webhookLogger(), testResWL)
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

	ws, err := gitops.NewWebhookStrategy(cfg, reloader, slog.Default(), testResWL)
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
	req1.RemoteAddr = "10.0.0.1:12345"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first XFF request: expected 200, got %d", rec1.Code)
	}

	// Second from same XFF IP — rate limited
	req2 := httptest.NewRequest(http.MethodPost, "/api/webhook", bytes.NewReader(payload))
	req2.Header.Set("X-Hub-Signature-256", sig)
	req2.Header.Set("X-Forwarded-For", "10.0.0.1")
	req2.RemoteAddr = "10.0.0.1:12346"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("same XFF IP: expected 429, got %d", rec2.Code)
	}

	// Third from different XFF IP — should pass
	req3 := httptest.NewRequest(http.MethodPost, "/api/webhook", bytes.NewReader(payload))
	req3.Header.Set("X-Hub-Signature-256", sig)
	req3.Header.Set("X-Forwarded-For", "10.0.0.2")
	req3.RemoteAddr = "10.0.0.2:12347"
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("different XFF IP: expected 200, got %d", rec3.Code)
	}
}

// Unsigned or badly signed requests from the same client IP as the real
// sender (e.g. everyone behind one ingress) must not exhaust the budget for
// signature-verified deliveries.
func TestWebhookHandler_UnauthenticatedCannotStarveVerified(t *testing.T) {
	secret := []byte("test-secret-min-32-bytes-long!!!!")
	called := 0
	reloader := func() error { called++; return nil }

	ws, err := gitops.NewWebhookStrategy(config.WebhookConfig{
		Path:         "/api/webhook",
		Secret:       string(secret),
		BranchFilter: "main",
		RateLimit:    2,
	}, reloader, webhookLogger(), testResWL)
	if err != nil {
		t.Fatal(err)
	}
	handler := ws.Handler()
	payload := []byte(`{"ref":"refs/heads/main"}`)

	send := func(sig string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/webhook", bytes.NewReader(payload))
		if sig != "" {
			req.Header.Set("X-Hub-Signature-256", sig)
		}
		req.RemoteAddr = "10.0.0.1:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	// Attacker burns through and past the failure budget.
	for i := 0; i < 5; i++ {
		send("sha256=deadbeef")
		send("")
	}
	if code := send("sha256=deadbeef"); code != http.StatusTooManyRequests {
		t.Fatalf("attacker after budget: expected 429, got %d", code)
	}

	// A genuine delivery from the same IP is still accepted.
	if code := send(signPayload(secret, payload)); code != http.StatusOK {
		t.Fatalf("verified delivery after junk: expected 200, got %d", code)
	}
	if called != 1 {
		t.Fatalf("expected 1 reload, got %d", called)
	}
}

// wellFormedBogusSig has the shape of a GitHub signature but will never
// verify; it gets past the pre-read shape check to exercise body handling.
var wellFormedBogusSig = "sha256=" + strings.Repeat("0", 64)

// Requests without a well-formed signature are rejected before the body is
// read, and every rejection past the per-IP failures budget is a silent 429,
// including oversized bodies.
func TestWebhookHandler_RejectionsShareFailureBudget(t *testing.T) {
	secret := []byte("test-secret-min-32-bytes-long!!!!")
	ws, err := gitops.NewWebhookStrategy(config.WebhookConfig{
		Path:        "/api/webhook",
		Secret:      string(secret),
		RateLimit:   2,
		MaxBodySize: 64,
	}, func() error { return nil }, webhookLogger(), testResWL)
	if err != nil {
		t.Fatal(err)
	}
	handler := ws.Handler()

	send := func(sig, body string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/webhook", strings.NewReader(body))
		if sig != "" {
			req.Header.Set("X-Hub-Signature-256", sig)
		}
		req.RemoteAddr = "10.0.0.7:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	oversized := strings.Repeat("x", 128)
	if code := send(wellFormedBogusSig, oversized); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("first oversized: got %d, want 413", code)
	}
	if code := send("sha256=bogus", "{}"); code != http.StatusUnauthorized {
		t.Fatalf("malformed signature: got %d, want 401", code)
	}
	// Budget of 2 is now spent; further rejections of any kind are 429.
	for _, tc := range []struct{ sig, body string }{
		{wellFormedBogusSig, oversized},
		{"", "{}"},
		{wellFormedBogusSig, "{}"},
	} {
		if code := send(tc.sig, tc.body); code != http.StatusTooManyRequests {
			t.Fatalf("over budget (sig=%q): got %d, want 429", tc.sig, code)
		}
	}

	// A correctly signed delivery from the same IP still succeeds.
	payload := []byte(`{"ref":"refs/heads/main"}`)
	if code := send(signPayload(secret, payload), string(payload)); code != http.StatusOK {
		t.Fatalf("verified delivery: got %d, want 200", code)
	}
}

// A request without a well-formed signature header must be rejected without
// reading the body.
func TestWebhookHandler_MalformedSignatureSkipsBodyRead(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"missing":      "",
		"bogus":        "sha256=bogus",
		"wrong prefix": "sha1=" + strings.Repeat("0", 64),
		"63 hex":       "sha256=" + strings.Repeat("0", 63),
		"65 hex":       "sha256=" + strings.Repeat("0", 65),
		"non-hex":      "sha256=" + strings.Repeat("g", 64),
	}
	for name, sig := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ws, err := gitops.NewWebhookStrategy(config.WebhookConfig{
				Path:   "/api/webhook",
				Secret: "test-secret-min-32-bytes-long!!!!",
			}, func() error { return nil }, webhookLogger(), testResWL)
			if err != nil {
				t.Fatal(err)
			}

			body := &countingReader{r: strings.NewReader(strings.Repeat("x", 1024))}
			req := httptest.NewRequest(http.MethodPost, "/api/webhook", body)
			if sig != "" {
				req.Header.Set("X-Hub-Signature-256", sig)
			}
			rec := httptest.NewRecorder()
			ws.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("got %d, want 401", rec.Code)
			}
			if body.n != 0 {
				t.Fatalf("body was read (%d bytes) for signature %q", body.n, sig)
			}
		})
	}
}

type countingReader struct {
	r *strings.Reader
	n int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}
