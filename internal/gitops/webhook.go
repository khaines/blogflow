package gitops

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/khaines/blogflow/internal/config"
)

const maxPayloadSize = 1 << 20 // 1 MB

// WebhookStrategy listens for HTTP webhook callbacks to trigger content reload.
// Validates HMAC-SHA256 signatures, filters by branch, and rate-limits.
type WebhookStrategy struct {
	config   config.WebhookConfig
	reloader ContentReloader
	logger   *slog.Logger
	handler  http.HandlerFunc
}

// NewWebhookStrategy creates a new webhook-based sync strategy.
// Path must be non-empty and start with "/". Secret must be non-empty.
func NewWebhookStrategy(cfg config.WebhookConfig, reloader ContentReloader, logger *slog.Logger) (*WebhookStrategy, error) {
	if cfg.Path == "" || cfg.Path[0] != '/' {
		return nil, fmt.Errorf("gitops: webhook path must be non-empty and start with '/' (got %q)", cfg.Path)
	}

	if cfg.Secret == "" {
		return nil, fmt.Errorf("gitops: webhook secret must not be empty")
	}

	w := &WebhookStrategy{
		config:   cfg,
		reloader: reloader,
		logger:   logger,
	}

	rl := newRateLimiter(cfg.RateLimit)
	w.handler = w.buildHandler(rl)

	return w, nil
}

// Handler returns the HTTP handler for registration with the server.
func (w *WebhookStrategy) Handler() http.HandlerFunc { return w.handler }

// Start activates the webhook strategy. The HTTP handler is registered
// externally with the server, so no background goroutine is needed.
func (w *WebhookStrategy) Start(ctx context.Context) error {
	w.logger.Info("webhook strategy active", "path", w.config.Path)
	return nil
}

// Stop gracefully shuts down the webhook strategy.
func (w *WebhookStrategy) Stop(ctx context.Context) error {
	w.logger.Info("webhook strategy stopped")
	return nil
}

// Name returns the strategy name.
func (w *WebhookStrategy) Name() string { return "webhook" }

func (w *WebhookStrategy) buildHandler(rl *rateLimiter) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Rate-limit by remote IP.
		ip := remoteIP(r)
		if rl != nil && !rl.allow(ip) {
			w.logger.Warn("rate limited", "ip", ip)
			http.Error(rw, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		// Limit request body size.
		r.Body = http.MaxBytesReader(rw, r.Body, maxPayloadSize)

		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.logger.Warn("body read error", "error", err)
			http.Error(rw, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}

		// Validate HMAC-SHA256 signature.
		sigHeader := r.Header.Get("X-Hub-Signature-256")
		if sigHeader == "" {
			w.logger.Warn("missing signature header")
			http.Error(rw, "missing signature", http.StatusUnauthorized)
			return
		}

		if !verifySignature([]byte(w.config.Secret), body, sigHeader) {
			w.logger.Warn("invalid signature")
			http.Error(rw, "invalid signature", http.StatusUnauthorized)
			return
		}

		// Filter by branch.
		if w.config.BranchFilter != "" {
			var payload struct {
				Ref string `json:"ref"`
			}

			if err := json.Unmarshal(body, &payload); err != nil {
				w.logger.Warn("invalid JSON payload", "error", err)
				http.Error(rw, "invalid payload", http.StatusBadRequest)
				return
			}

			expectedRef := "refs/heads/" + w.config.BranchFilter
			if payload.Ref != expectedRef {
				w.logger.Debug("ignoring push to non-matching branch",
					"ref", payload.Ref, "filter", w.config.BranchFilter)
				rw.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(rw, "accepted (no action)")
				return
			}
		}

		// Trigger content reload.
		if err := w.reloader(); err != nil {
			w.logger.Error("content reload failed", "error", err)
			http.Error(rw, "reload failed", http.StatusInternalServerError)
			return
		}

		w.logger.Info("content reloaded via webhook")
		rw.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(rw, "ok")
	}
}

// verifySignature validates an HMAC-SHA256 signature with constant-time comparison.
func verifySignature(secret, payload []byte, sigHeader string) bool {
	sig, found := strings.CutPrefix(sigHeader, "sha256=")
	if !found {
		return false
	}

	decoded, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	expected := mac.Sum(nil)

	return hmac.Equal(decoded, expected)
}

// remoteIP extracts the client IP from the request, stripping the port.
// remoteIP extracts the client IP from the request.
// Checks X-Forwarded-For first (for reverse-proxy deployments), falls back to RemoteAddr.
func remoteIP(r *http.Request) string {
	// Trust X-Forwarded-For if present (standard reverse proxy header).
	// Takes the first (leftmost) IP — the original client.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		return addr[:idx]
	}
	return addr
}

// rateLimiter implements a sliding-window rate limiter per IP.
type rateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	clients map[string][]time.Time
}

func newRateLimiter(limit int) *rateLimiter {
	if limit <= 0 {
		return nil
	}

	return &rateLimiter{
		limit:   limit,
		window:  time.Minute,
		clients: make(map[string][]time.Time),
	}
}

func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Periodic eviction: sweep stale entries when map grows large.
	if len(rl.clients) > 1000 {
		for k, timestamps := range rl.clients {
			if len(timestamps) == 0 || timestamps[len(timestamps)-1].Before(cutoff) {
				delete(rl.clients, k)
			}
		}
	}

	timestamps := rl.clients[ip]
	valid := timestamps[:0]

	for _, ts := range timestamps {
		if ts.After(cutoff) {
			valid = append(valid, ts)
		}
	}

	if len(valid) >= rl.limit {
		rl.clients[ip] = valid
		return false
	}

	rl.clients[ip] = append(valid, now)
	return true
}
