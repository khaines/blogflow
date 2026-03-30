package handlers

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
)

type contextKey string

const previewKey contextKey = "preview"

// IsPreview reports whether the request has preview mode enabled.
func IsPreview(ctx context.Context) bool {
	v, _ := ctx.Value(previewKey).(bool)
	return v
}

// withPreview returns a new context with the preview flag set.
func withPreview(ctx context.Context) context.Context {
	return context.WithValue(ctx, previewKey, true)
}

// PreviewMiddleware checks for a valid preview token in the request.
// If the token matches, the request context is annotated with a preview flag
// that handlers use to include draft posts.
//
// Supported token sources (checked in order):
//  1. Query parameter: ?preview=true&token=<secret>
//  2. Authorization header: Bearer <secret>
//
// When the configured token is empty, preview mode is disabled and the
// middleware is a no-op passthrough.
func PreviewMiddleware(deps *Deps) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cfg := deps.LoadConfig()
			token := cfg.Site.PreviewToken
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}

			if matchesPreviewToken(r, token) {
				r = r.WithContext(withPreview(r.Context()))
			}

			next.ServeHTTP(w, r)
		})
	}
}

// matchesPreviewToken checks query params and Authorization header for a
// valid preview token. Uses constant-time comparison to prevent timing attacks.
func matchesPreviewToken(r *http.Request, token string) bool {
	// Check query parameter: ?preview=true&token=<secret>
	if r.URL.Query().Get("preview") == "true" {
		candidate := r.URL.Query().Get("token")
		if candidate != "" && subtle.ConstantTimeCompare([]byte(candidate), []byte(token)) == 1 {
			return true
		}
	}

	// Check Authorization header: Bearer <secret>
	if auth := r.Header.Get("Authorization"); auth != "" {
		candidate := strings.TrimPrefix(auth, "Bearer ")
		if candidate != auth && subtle.ConstantTimeCompare([]byte(candidate), []byte(token)) == 1 {
			return true
		}
	}

	return false
}
