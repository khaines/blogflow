package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"log/slog"
	"net/http"
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

	// Conditional: If-None-Match
	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// Conditional: If-Modified-Since
	if !lastMod.IsZero() {
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
