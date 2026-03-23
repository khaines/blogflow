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
