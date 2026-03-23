package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// writeXMLCached serialises v as XML and writes it to w with HTTP caching
// headers (Cache-Control, ETag, Last-Modified). It handles conditional
// requests (If-None-Match, If-Modified-Since) by returning 304 Not Modified
// when appropriate.
func writeXMLCached(w http.ResponseWriter, r *http.Request, contentType string, v any, lastMod time.Time) {
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(v); err != nil {
		slog.Error("XML encode failed", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	body := buf.Bytes()

	hash := sha256.Sum256(body)
	etag := fmt.Sprintf(`"%x"`, hash[:8])

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("ETag", etag)
	if !lastMod.IsZero() {
		w.Header().Set("Last-Modified", lastMod.UTC().Format(http.TimeFormat))
	}

	// Conditional: If-None-Match takes precedence (RFC 7232 §3.2).
	// When present, If-Modified-Since MUST be ignored (RFC 7232 §3.3).
	if inm := r.Header.Get("If-None-Match"); inm != "" {
		if etagMatches(inm, etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		// ETag did not match — serve fresh response; skip If-Modified-Since.
	} else if !lastMod.IsZero() {
		// Conditional: If-Modified-Since (only when If-None-Match is absent).
		if ims := r.Header.Get("If-Modified-Since"); ims != "" {
			if t, err := http.ParseTime(ims); err == nil {
				if !lastMod.UTC().Truncate(time.Second).After(t.UTC()) {
					w.WriteHeader(http.StatusNotModified)
					return
				}
			}
		}
	}

	_, _ = w.Write(body)
}

// etagMatches reports whether the If-None-Match header value matches the
// given ETag. It handles the wildcard "*" and comma-separated lists of ETags
// per RFC 7232 §3.2.
func etagMatches(inmHeader, etag string) bool {
	if strings.TrimSpace(inmHeader) == "*" {
		return true
	}
	for _, field := range strings.Split(inmHeader, ",") {
		field = strings.TrimSpace(field)
		// Strip weak prefix for comparison (RFC 7232 §2.3.2 weak comparison).
		field = strings.TrimPrefix(field, "W/")
		if field == etag {
			return true
		}
	}
	return false
}
