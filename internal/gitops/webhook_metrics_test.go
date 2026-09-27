package gitops

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/khaines/blogflow/internal/config"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

type fixedIP string

func (f fixedIP) ClientIP(*http.Request) string { return string(f) }

func TestWebhookRequestsTotal_CountsSilentRejections(t *testing.T) {
	ws, err := NewWebhookStrategy(config.WebhookConfig{
		Path:      "/hook",
		Secret:    "test-secret-min-32-bytes-long!!!!",
		RateLimit: 1,
	}, func() error { return nil }, slog.New(slog.DiscardHandler), fixedIP("192.0.2.77"))
	if err != nil {
		t.Fatal(err)
	}

	missing := testutil.ToFloat64(webhookRequestsTotal.WithLabelValues(outcomeMissingSignature))
	budget := testutil.ToFloat64(webhookRequestsTotal.WithLabelValues(outcomeFailureBudget))

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/hook", strings.NewReader("{}"))
		ws.Handler().ServeHTTP(httptest.NewRecorder(), req)
	}

	if d := testutil.ToFloat64(webhookRequestsTotal.WithLabelValues(outcomeMissingSignature)) - missing; d != 1 {
		t.Errorf("missing_signature delta = %v, want 1", d)
	}
	if d := testutil.ToFloat64(webhookRequestsTotal.WithLabelValues(outcomeFailureBudget)) - budget; d != 2 {
		t.Errorf("failure_budget_exceeded delta = %v, want 2", d)
	}
}

func TestWebhookRequestsTotal_CountsEveryExit(t *testing.T) {
	const secret = "test-secret-min-32-bytes-long!!!!"
	push := []byte(`{"ref":"refs/heads/main"}`)
	other := []byte(`{"ref":"refs/heads/dev"}`)

	cases := []struct {
		name     string
		cfg      config.WebhookConfig
		reload   error
		method   string
		event    string
		body     []byte
		outcome  string
		wantCode int
	}{
		{name: "ok", body: push, outcome: outcomeOK, wantCode: http.StatusOK},
		{name: "method", method: http.MethodGet, outcome: outcomeMethodNotAllowed, wantCode: http.StatusMethodNotAllowed},
		{name: "forbidden ip", cfg: config.WebhookConfig{AllowedIPs: []string{"198.51.100.0/24"}}, body: push, outcome: outcomeForbiddenIP, wantCode: http.StatusForbidden},
		{name: "missing event", cfg: config.WebhookConfig{AllowedEvents: []string{"push"}}, body: push, outcome: outcomeEventRejected, wantCode: http.StatusForbidden},
		{name: "wrong event", cfg: config.WebhookConfig{AllowedEvents: []string{"push"}}, event: "issues", body: push, outcome: outcomeEventRejected, wantCode: http.StatusForbidden},
		{name: "invalid payload", cfg: config.WebhookConfig{BranchFilter: "main"}, body: []byte("not json"), outcome: outcomeInvalidPayload, wantCode: http.StatusBadRequest},
		{name: "branch skipped", cfg: config.WebhookConfig{BranchFilter: "main"}, body: other, outcome: outcomeBranchSkipped, wantCode: http.StatusAccepted},
		{name: "reload failed", reload: errors.New("boom"), body: push, outcome: outcomeReloadFailed, wantCode: http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.cfg
			cfg.Path, cfg.Secret = "/hook", secret
			ws, err := NewWebhookStrategy(cfg, func() error { return tc.reload },
				slog.New(slog.DiscardHandler), fixedIP("192.0.2.88"))
			if err != nil {
				t.Fatal(err)
			}

			before := testutil.ToFloat64(webhookRequestsTotal.WithLabelValues(tc.outcome))

			method := tc.method
			if method == "" {
				method = http.MethodPost
			}
			req := httptest.NewRequest(method, "/hook", bytes.NewReader(tc.body))
			mac := hmac.New(sha256.New, []byte(secret))
			mac.Write(tc.body)
			req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
			if tc.event != "" {
				req.Header.Set("X-GitHub-Event", tc.event)
			}
			rec := httptest.NewRecorder()
			ws.Handler().ServeHTTP(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantCode)
			}
			if d := testutil.ToFloat64(webhookRequestsTotal.WithLabelValues(tc.outcome)) - before; d != 1 {
				t.Errorf("%s delta = %v, want 1", tc.outcome, d)
			}
		})
	}
}

// Every outcome is pre-initialised so dashboards see zero-valued series.
func TestWebhookRequestsTotal_PreInitialised(t *testing.T) {
	n := testutil.CollectAndCount(webhookRequestsTotal, "blogflow_webhook_requests_total")
	if n != len(webhookOutcomes) {
		t.Fatalf("series = %d, want %d", n, len(webhookOutcomes))
	}
}

// A disallowed source IP is charged to the failures budget so it cannot
// flood the logs.
func TestWebhookHandler_ForbiddenIPUsesFailureBudget(t *testing.T) {
	ws, err := NewWebhookStrategy(config.WebhookConfig{
		Path:       "/hook",
		Secret:     "test-secret-min-32-bytes-long!!!!",
		RateLimit:  1,
		AllowedIPs: []string{"198.51.100.0/24"},
	}, func() error { return nil }, slog.New(slog.DiscardHandler), fixedIP("192.0.2.99"))
	if err != nil {
		t.Fatal(err)
	}

	codes := make([]int, 3)
	for i := range codes {
		rec := httptest.NewRecorder()
		ws.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/hook", strings.NewReader("{}")))
		codes[i] = rec.Code
	}
	want := []int{http.StatusForbidden, http.StatusTooManyRequests, http.StatusTooManyRequests}
	if !slices.Equal(codes, want) {
		t.Fatalf("codes = %v, want %v", codes, want)
	}
}
