package main

import (
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"
)

// newTestHealthServer starts an HTTP server that responds to /healthz
// with the given status code. It returns the listener (caller must close).
func newTestHealthServer(t *testing.T, status int) net.Listener {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, "ok")
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := &http.Server{
		Handler:     mux,
		ReadTimeout: 5 * time.Second,
	}
	go srv.Serve(ln) //nolint:errcheck

	return ln
}

func TestHealthcheckDefaultPort(t *testing.T) {
	tests := []struct {
		name    string
		private string
		ops     string
		metrics string
		port    string
		want    int
	}{
		{"defaults", "", "", "", "", defaultPort},
		{"private health uses ops port", "true", "8081", "", "", 8081},
		{"private health prefers ops over metrics alias", "true", "8081", "9090", "", 8081},
		{"private health falls back to metrics alias", "true", "", "9090", "", 9090},
		{"private health without ops/metrics falls back to server port", "true", "", "", "9000", 9000},
		{"private health without any port falls back to default", "true", "", "", "", defaultPort},
		{"custom server port", "", "", "", "9090", 9090},
		{"private false ignores ops and metrics", "false", "8081", "9090", "", defaultPort},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BLOGFLOW_SERVER_PRIVATE_HEALTH", tt.private)
			t.Setenv("BLOGFLOW_SERVER_OPS_PORT", tt.ops)
			t.Setenv("BLOGFLOW_SERVER_METRICS_PORT", tt.metrics)
			t.Setenv("BLOGFLOW_SERVER_PORT", tt.port)
			if got := healthcheckDefaultPort(); got != tt.want {
				t.Errorf("healthcheckDefaultPort() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRunHealthcheck_Healthy(t *testing.T) {
	ln := newTestHealthServer(t, http.StatusOK)
	defer ln.Close() //nolint:errcheck

	port := ln.Addr().(*net.TCPAddr).Port
	code := runHealthcheck([]string{"--port", fmt.Sprintf("%d", port)})
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestRunHealthcheck_Unhealthy(t *testing.T) {
	ln := newTestHealthServer(t, http.StatusServiceUnavailable)
	defer ln.Close() //nolint:errcheck

	port := ln.Addr().(*net.TCPAddr).Port
	code := runHealthcheck([]string{"--port", fmt.Sprintf("%d", port)})
	if code != 1 {
		t.Errorf("expected exit 1, got %d", code)
	}
}

func TestRunHealthcheck_ConnectionRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close() // close immediately so nothing is listening

	code := runHealthcheck([]string{"--port", fmt.Sprintf("%d", port)})
	if code != 1 {
		t.Errorf("expected exit 1, got %d", code)
	}
}

func TestRunHealthcheck_InvalidFlag(t *testing.T) {
	code := runHealthcheck([]string{"--bogus"})
	if code != 1 {
		t.Errorf("expected exit 1, got %d", code)
	}
}
