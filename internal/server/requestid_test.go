package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

var uuidV4Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestGenerateUUID_Format(t *testing.T) {
	seen := make(map[string]bool, 100)
	for range 100 {
		id, err := generateUUID()
		if err != nil {
			t.Fatalf("generateUUID() error: %v", err)
		}
		if !uuidV4Re.MatchString(id) {
			t.Errorf("generateUUID() = %q, does not match UUID v4 pattern", id)
		}
		if seen[id] {
			t.Errorf("duplicate UUID in 100 iterations: %s", id)
		}
		seen[id] = true
	}
}

func TestIsValidRequestID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid uuid", "550e8400-e29b-41d4-a716-446655440000", true},
		{"valid short", "abc123", true},
		{"empty", "", false},
		{"too long", string(make([]byte, 129)), false},
		{"has space", "abc 123", false},
		{"has tab", "abc\t123", false},
		{"has newline", "abc\n123", false},
		{"has null", "abc\x00def", false},
		{"non-ascii", "café", false},
		{"printable symbols", "req-123_456.test", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidRequestID(tt.input); got != tt.want {
				t.Errorf("isValidRequestID(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func newRequestIDTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := defaultTestConfig()
	s := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.RegisterRoutes(testRouteOptions())
	return s
}

func TestRequestIDMiddleware_GeneratesNew(t *testing.T) {
	s := newRequestIDTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	id := rec.Header().Get(requestIDHeader)
	if id == "" {
		t.Fatal("expected X-Request-Id response header, got empty")
	}
	if !uuidV4Re.MatchString(id) {
		t.Errorf("generated ID %q does not match UUID v4 pattern", id)
	}
}

func TestRequestIDMiddleware_PropagatesValid(t *testing.T) {
	s := newRequestIDTestServer(t)

	incoming := "trace-abc-123"
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set(requestIDHeader, incoming)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	if got := rec.Header().Get(requestIDHeader); got != incoming {
		t.Errorf("X-Request-Id = %q, want %q", got, incoming)
	}
}

func TestRequestIDMiddleware_RejectsInvalid(t *testing.T) {
	s := newRequestIDTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set(requestIDHeader, "bad\nheader")
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	id := rec.Header().Get(requestIDHeader)
	if id == "bad\nheader" {
		t.Error("invalid request ID should not be propagated")
	}
	if !uuidV4Re.MatchString(id) {
		t.Errorf("expected new UUID for invalid input, got %q", id)
	}
}

func TestRequestIDMiddleware_InContext(t *testing.T) {
	cfg := defaultTestConfig()
	s := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	var ctxID string
	opts := testRouteOptions()
	opts.HomeHandler = func(w http.ResponseWriter, r *http.Request) {
		ctxID = RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}
	s.RegisterRoutes(opts)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	headerID := rec.Header().Get(requestIDHeader)
	if ctxID == "" {
		t.Fatal("request ID not found in context")
	}
	if ctxID != headerID {
		t.Errorf("context ID %q != header ID %q", ctxID, headerID)
	}
}

func TestRequestIDFromContext_EmptyContext(t *testing.T) {
	if got := RequestIDFromContext(httptest.NewRequest(http.MethodGet, "/", nil).Context()); got != "" {
		t.Errorf("RequestIDFromContext on empty context = %q, want empty", got)
	}
}
