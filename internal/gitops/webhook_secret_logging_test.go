// Webhook secret logging redaction per test-gap-analysis.md item #16
package gitops_test

import (
	"bytes"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/khaines/blogflow/internal/config"
	"github.com/khaines/blogflow/internal/gitops"
)

// testResolverSec resolves client IPs from RemoteAddr only.
type testResolverSec struct{}

func (*testResolverSec) ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || host == "" {
		host = r.RemoteAddr
	}
	return host
}

var testResolverSecIns = &testResolverSec{}

func TestWebhookSecretLoggingRedaction(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	secret := "very-long-webhook-secret-minimum-32-bytes-ok"

	reloader := func() error { return nil }
	ws, err := gitops.NewWebhookStrategy(config.WebhookConfig{
		Path:   "/hook",
		Secret: secret,
	}, reloader, logger, testResolverSecIns)
	if err != nil {
		t.Fatal(err)
	}

	handler := ws.Handler()
	payload := []byte(`{"ref":"refs/heads/main"}`)
	sig := signPayload([]byte(secret), payload)

	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
	req.Header.Set("X-Hub-Signature-256", sig)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	logs := buf.String()
	if logs == "" {
		t.Log("no log output captured")
		return
	}

	if strings.Contains(logs, secret) {
		t.Errorf("raw secret leaked in logs")
	} else {
		t.Logf("webhook secret properly redacted")
	}
}
