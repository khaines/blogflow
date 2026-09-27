package gitops

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
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

type failingBody struct{}

func (failingBody) Read([]byte) (int, error) { return 0, errors.New("connection reset") }

// These tests read the global counter, so they must not use t.Parallel.
func TestWebhookRequestsTotal_CountsEveryExit(t *testing.T) {
	const secret = "test-secret-min-32-bytes-long!!!!"
	push := []byte(`{"ref":"refs/heads/main"}`)
	other := []byte(`{"ref":"refs/heads/dev"}`)

	cases := []struct {
		name     string
		cfg      config.WebhookConfig
		reload   error
		event    string
		body     []byte
		sig      string    // overrides the computed signature
		reader   io.Reader // overrides body as the request body
		prior    int       // identical requests sent before the measured one
		outcome  string
		wantCode int
	}{
		{name: "ok", body: push, outcome: outcomeOK, wantCode: http.StatusOK},
		{name: "forbidden ip", cfg: config.WebhookConfig{AllowedIPs: []string{"198.51.100.0/24"}}, body: push, outcome: outcomeForbiddenIP, wantCode: http.StatusForbidden},
		{name: "missing event", cfg: config.WebhookConfig{AllowedEvents: []string{"push"}}, body: push, outcome: outcomeEventRejected, wantCode: http.StatusForbidden},
		{name: "wrong event", cfg: config.WebhookConfig{AllowedEvents: []string{"push"}}, event: "issues", body: push, outcome: outcomeEventRejected, wantCode: http.StatusForbidden},
		{name: "invalid payload", cfg: config.WebhookConfig{BranchFilter: "main"}, body: []byte("not json"), outcome: outcomeInvalidPayload, wantCode: http.StatusBadRequest},
		{name: "branch skipped", cfg: config.WebhookConfig{BranchFilter: "main"}, body: other, outcome: outcomeBranchSkipped, wantCode: http.StatusAccepted},
		{name: "reload failed", reload: errors.New("boom"), body: push, outcome: outcomeReloadFailed, wantCode: http.StatusInternalServerError},
		{name: "missing signature", body: push, sig: "-", outcome: outcomeMissingSignature, wantCode: http.StatusUnauthorized},
		{name: "malformed signature", body: push, sig: "sha256=bogus", outcome: outcomeInvalidSignature, wantCode: http.StatusUnauthorized},
		{name: "wrong signature", body: push, sig: "sha256=" + strings.Repeat("0", 64), outcome: outcomeInvalidSignature, wantCode: http.StatusUnauthorized},
		{name: "body too large", cfg: config.WebhookConfig{MaxBodySize: 8}, body: push, outcome: outcomeBodyTooLarge, wantCode: http.StatusRequestEntityTooLarge},
		{name: "body read error", body: push, reader: failingBody{}, outcome: outcomeBodyReadError, wantCode: http.StatusBadRequest},
		{name: "failure budget", cfg: config.WebhookConfig{RateLimit: 1}, body: push, sig: "-", prior: 1, outcome: outcomeFailureBudget, wantCode: http.StatusTooManyRequests},
		{name: "verified budget", cfg: config.WebhookConfig{RateLimit: 1}, body: push, prior: 1, outcome: outcomeVerifiedBudget, wantCode: http.StatusTooManyRequests},
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

			newReq := func() *http.Request {
				var body io.Reader = bytes.NewReader(tc.body)
				if tc.reader != nil {
					body = tc.reader
				}
				req := httptest.NewRequest(http.MethodPost, "/hook", body)
				mac := hmac.New(sha256.New, []byte(secret))
				mac.Write(tc.body)
				switch tc.sig {
				case "":
					req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
				case "-": // no header
				default:
					req.Header.Set("X-Hub-Signature-256", tc.sig)
				}
				if tc.event != "" {
					req.Header.Set("X-GitHub-Event", tc.event)
				}
				return req
			}
			for i := 0; i < tc.prior; i++ {
				ws.Handler().ServeHTTP(httptest.NewRecorder(), newReq())
			}

			before := map[string]float64{}
			for _, o := range webhookOutcomes {
				before[o] = testutil.ToFloat64(webhookRequestsTotal.WithLabelValues(o))
			}

			rec := httptest.NewRecorder()
			ws.Handler().ServeHTTP(rec, newReq())

			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantCode)
			}
			// Exactly one outcome moves, by exactly one.
			for _, o := range webhookOutcomes {
				want := 0.0
				if o == tc.outcome {
					want = 1
				}
				if d := testutil.ToFloat64(webhookRequestsTotal.WithLabelValues(o)) - before[o]; d != want {
					t.Errorf("%s delta = %v, want %v", o, d, want)
				}
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

// Signed deliveries that the event or branch filters drop must not use up the
// verified budget, or pushes to other branches could crowd out the one that
// matters.
func TestWebhookHandler_FilteredDeliveriesSkipVerifiedBudget(t *testing.T) {
	const secret = "test-secret-min-32-bytes-long!!!!"
	ws, err := NewWebhookStrategy(config.WebhookConfig{
		Path:         "/hook",
		Secret:       secret,
		RateLimit:    1,
		BranchFilter: "main",
	}, func() error { return nil }, slog.New(slog.DiscardHandler), fixedIP("192.0.2.66"))
	if err != nil {
		t.Fatal(err)
	}
	send := func(ref string) int {
		body := []byte(`{"ref":"refs/heads/` + ref + `"}`)
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(body))
		req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		rec := httptest.NewRecorder()
		ws.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	for i := 0; i < 3; i++ {
		if code := send("dev"); code != http.StatusAccepted {
			t.Fatalf("dev push %d: got %d, want 202", i, code)
		}
	}
	if code := send("main"); code != http.StatusOK {
		t.Fatalf("main push after filtered pushes: got %d, want 200", code)
	}
}
