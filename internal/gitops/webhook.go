package gitops

import (
	"container/list"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/khaines/blogflow/internal/config"
)

const defaultMaxPayloadSize int64 = 1 << 20 // 1 MB

// IPResolver extracts the client IP from an HTTP request.
// The server.ClientIPResolver type satisfies this interface.
type IPResolver interface {
	ClientIP(r *http.Request) string
}

// WebhookStrategy listens for HTTP webhook callbacks to trigger content reload.
// Validates HMAC-SHA256 signatures, filters by branch, and rate-limits.
type WebhookStrategy struct {
	config     config.WebhookConfig
	reloader   ContentReloader
	logger     *slog.Logger
	handler    http.HandlerFunc
	ipResolver IPResolver
}

// SetIPResolver updates the IP resolver after construction. Since NewWebhookStrategy
// now requires a non-nil resolver, this method is provided for testing overrides only.
func (w *WebhookStrategy) SetIPResolver(r IPResolver) { w.ipResolver = r }

// NewWebhookStrategy creates a new webhook-based sync strategy.
// Path must be non-empty and start with "/". Secret must be at least 32 bytes.
// IPResolver is mandatory — a built-in X-Forwarded-For fallback is disallowed
// because untrusted XFF data is a server-side request forgery vector.
func NewWebhookStrategy(cfg config.WebhookConfig, reloader ContentReloader, logger *slog.Logger, resolver IPResolver) (*WebhookStrategy, error) {
	if cfg.Path == "" || cfg.Path[0] != '/' {
		return nil, fmt.Errorf("gitops: webhook path must be non-empty and start with '/' (got %q)", cfg.Path)
	}

	if cfg.Secret == "" {
		return nil, fmt.Errorf("gitops: webhook secret must not be empty")
	}
	// F2: Enforce minimum secret length to prevent brute-forcing HMAC-SHA256.
	if len(cfg.Secret) < 32 {
		return nil, fmt.Errorf("gitops: webhook.secret must be at least 32 bytes (got %d bytes)", len(cfg.Secret))
	}

	w := &WebhookStrategy{
		config:   cfg,
		reloader: reloader,
		logger:   logger,
	}

	// F3: IP resolver is mandatory — no fallback to blind X-Forwarded-For trust.
	if resolver == nil {
		return nil, fmt.Errorf("gitops: webhook strategy requires an IP resolver (no trusted-proxy boundary)")
	}
	w.ipResolver = resolver

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

		// Resolve client IP for allowlist and rate-limit lookups.
		ip := w.resolveIP(r)

		// Validate IP against allowlist BEFORE rate limiting.
		if len(w.config.AllowedIPs) > 0 {
			if !ipInCIDRs(ip, w.config.AllowedIPs) {
				w.logger.Warn("source IP not in allowed_ips", "ip", ip)
				http.Error(rw, "source IP not in allowed_ips", http.StatusForbidden)
				return
			}
		}

		// Rate-limit by remote IP (after allowlist validation).
		if rl != nil && !rl.allow(ip) {
			w.logger.Warn("rate limited", "ip", ip)
			http.Error(rw, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		// Limit request body size.
		limit := w.config.MaxBodySize
		if limit <= 0 {
			limit = defaultMaxPayloadSize
		}
		r.Body = http.MaxBytesReader(rw, r.Body, limit)

		body, err := io.ReadAll(r.Body)
		if err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				w.logger.Warn("body too large", "limit", limit, "ip", ip)
				http.Error(rw, "request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			w.logger.Warn("body read error", "error", err)
			http.Error(rw, "failed to read body", http.StatusBadRequest)
			return
		}

		// Validate HMAC-SHA256 signature BEFORE any event/branch filtering
		// to prevent unauthenticated callers from probing allowed event types.
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

		// AllowedEvents filtering (after signature verification).
		if len(w.config.AllowedEvents) > 0 {
			event := r.Header.Get("X-GitHub-Event")
			if event == "" {
				w.logger.Warn("missing event header with active event filter",
					"allowed", w.config.AllowedEvents)
				http.Error(rw, "missing event header", http.StatusForbidden)
				return
			}
			if !slices.Contains(w.config.AllowedEvents, event) {
				w.logger.Warn("event type not allowed",
					"event", event, "allowed", w.config.AllowedEvents)
				http.Error(rw, "event type not allowed", http.StatusForbidden)
				return
			}
		}

		// Filter by branch (after signature verification).
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
				rw.Header().Set("X-Blogflow-Branch-Skipped", payload.Ref)
				rw.WriteHeader(http.StatusAccepted)
				_, _ = fmt.Fprintf(rw, "%s (no action)", payload.Ref)
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

// ipInCIDRs checks if string IP matches any entry in the allowed list.
// Entries in allowedIPs can be bare IPs (exact match) or CIDRs (range match).
// Bare-IP entries are compared via net.ParseIP + net.IP.Equal to handle
// non-canonical representations (e.g., ::1 vs 0:0:0:0:0:0:0:1).
func ipInCIDRs(ip string, allowedIPs []string) bool {
	target := net.ParseIP(ip)
	if target == nil {
		return false
	}
	for _, entry := range allowedIPs {
		entry = strings.TrimSpace(entry)
		// Check CIDR first (covers bare-IP case via /32 or /128 too).
		_, cidr, err := net.ParseCIDR(entry)
		if err == nil {
			if cidr.Contains(target) {
				return true
			}
			continue
		}
		// Not a CIDR — try bare-IP match.
		entryParsed := net.ParseIP(entry)
		if entryParsed != nil && entryParsed.Equal(target) {
			return true
		}
	}
	return false
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

// resolveIP returns the client IP using the configured resolver, or the
// built-in remoteIP heuristic when no resolver is set.
// When resolver is nil, we fall back to RemoteAddr only (never XFF) to
// prevent blind trust in untrusted X-Forwarded-For data.
func (w *WebhookStrategy) resolveIP(r *http.Request) string {
	if w.ipResolver != nil {
		return w.ipResolver.ClientIP(r)
	}
	// Fail closed: use RemoteAddr only (no XFF trust) when resolver is nil.
	// Use net.SplitHostPort for correct IPv6 bracket handling.
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

// remoteIP extracts the client IP from the request.
// WARNING: This function blindly trusts X-Forwarded-For and must never be used
// in production. It exists only to support unit tests that cannot inject a real
// resolver. Production code always provides a resolver via NewWebhookStrategy.
//
//nolint:unused // DEPRECATED: no longer called by resolveIP — kept for backward-compat
func remoteIP(r *http.Request) string {
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

const defaultMaxClients = 10000

// rateLimiter implements a sliding-window rate limiter per IP with LRU
// eviction. Entries whose timestamps are all older than the rate window are
// lazily evicted on each request. When the map reaches capacity, the
// least-recently-seen IP is evicted instead of clearing the entire map.
type rateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	maxSize int
	entries map[string]*list.Element
	lru     *list.List // front = most-recently-seen
	now     func() time.Time
}

type rlEntry struct {
	ip         string
	timestamps []time.Time
}

func newRateLimiter(limit int) *rateLimiter {
	if limit <= 0 {
		return nil
	}

	return &rateLimiter{
		limit:   limit,
		window:  time.Minute,
		maxSize: defaultMaxClients,
		entries: make(map[string]*list.Element),
		lru:     list.New(),
		now:     time.Now,
	}
}

func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := rl.now()
	cutoff := now.Add(-rl.window)

	// Lazy TTL eviction: remove expired entries from the back of the LRU list.
	rl.evictExpired(cutoff)

	if elem, ok := rl.entries[ip]; ok {
		rl.lru.MoveToFront(elem)
		entry := elem.Value.(*rlEntry)

		valid := entry.timestamps[:0]
		for _, ts := range entry.timestamps {
			if ts.After(cutoff) {
				valid = append(valid, ts)
			}
		}

		if len(valid) >= rl.limit {
			entry.timestamps = valid
			return false
		}

		entry.timestamps = append(valid, now)
		return true
	}

	// New IP — evict least-recently-seen if at capacity.
	for len(rl.entries) >= rl.maxSize {
		rl.evictOldest()
	}

	entry := &rlEntry{ip: ip, timestamps: []time.Time{now}}
	elem := rl.lru.PushFront(entry)
	rl.entries[ip] = elem
	return true
}

// evictExpired removes entries from the back of the LRU whose latest
// timestamp is before cutoff.
func (rl *rateLimiter) evictExpired(cutoff time.Time) {
	for rl.lru.Len() > 0 {
		back := rl.lru.Back()
		entry := back.Value.(*rlEntry)
		if len(entry.timestamps) > 0 && entry.timestamps[len(entry.timestamps)-1].After(cutoff) {
			break
		}
		rl.lru.Remove(back)
		delete(rl.entries, entry.ip)
	}
}

// evictOldest removes the least-recently-seen entry.
func (rl *rateLimiter) evictOldest() {
	if back := rl.lru.Back(); back != nil {
		entry := back.Value.(*rlEntry)
		rl.lru.Remove(back)
		delete(rl.entries, entry.ip)
	}
}
