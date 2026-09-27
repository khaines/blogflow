package gitops

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
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
