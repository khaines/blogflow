// Package content provides markdown rendering and front matter parsing
// for BlogFlow's content pipeline.
package content

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	maxFrontMatterSize = 64 * 1024 // 64 KB
)

// FrontMatter holds the metadata from a markdown file's YAML front matter.
type FrontMatter struct {
	Title       string    `yaml:"title"`
	Slug        string    `yaml:"slug"`
	Date        time.Time `yaml:"date"`
	Updated     time.Time `yaml:"updated,omitempty"`
	Draft       bool      `yaml:"draft"`
	Tags        []string  `yaml:"tags"`
	Categories  []string  `yaml:"categories"`
	Author      string    `yaml:"author"`
	Description string    `yaml:"description"`
	Template    string    `yaml:"template,omitempty"`
	Image       string    `yaml:"image,omitempty"`
	ReadingTime int       `yaml:"reading_time,omitempty"`
}

// ParseFrontMatter splits a markdown file into front matter and body.
// Returns the parsed front matter, the remaining markdown body, and any error.
// Front matter is delimited by --- at the start of the file.
// ReadingTime in FrontMatter is populated from YAML if present.
// Callers should compute it via ReadingTimeMinutes() if the field is zero.
func ParseFrontMatter(data []byte) (*FrontMatter, []byte, error) {
	if !bytes.HasPrefix(data, []byte("---")) {
		return nil, data, nil
	}

	// Require --- followed by newline or EOF (reject ----, ---text, etc.)
	if len(data) > 3 && data[3] != '\n' && data[3] != '\r' {
		return nil, data, nil // not front matter
	}

	rest := data[3:]
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	} else if len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n' {
		rest = rest[2:]
	}

	end := findClosingDelimiter(rest)
	if end == -1 {
		return nil, nil, fmt.Errorf("front matter: missing closing ---")
	}

	fmData := rest[:end]

	if len(fmData) > maxFrontMatterSize {
		return nil, nil, fmt.Errorf("front matter: exceeds maximum size of %d bytes", maxFrontMatterSize)
	}

	body := rest[end+4:] // skip \n---
	if len(body) > 0 && body[0] == '\n' {
		body = body[1:]
	} else if len(body) > 1 && body[0] == '\r' && body[1] == '\n' {
		body = body[2:]
	}

	var fm FrontMatter
	if err := yaml.Unmarshal(fmData, &fm); err != nil {
		return nil, nil, fmt.Errorf("front matter: %w", err)
	}

	if fm.Image != "" && !isSafeURL([]byte(fm.Image)) {
		return nil, nil, fmt.Errorf("front matter: image URL uses disallowed scheme")
	}

	if fm.Template != "" {
		if strings.Contains(fm.Template, "/") || strings.Contains(fm.Template, "\\") || strings.Contains(fm.Template, "..") || strings.Contains(fm.Template, "\x00") {
			return nil, nil, fmt.Errorf("front matter: template must be a plain filename, got %q", fm.Template)
		}
	}

	if fm.Slug != "" {
		if strings.Contains(fm.Slug, "/") || strings.Contains(fm.Slug, "\\") || strings.Contains(fm.Slug, "..") || strings.Contains(fm.Slug, "\x00") {
			return nil, nil, fmt.Errorf("front matter: slug must be a plain path segment, got %q", fm.Slug)
		}
	}

	return &fm, body, nil
}

// findClosingDelimiter finds the first standalone \n--- line in data.
// A standalone closing delimiter is \n--- followed by \n, \r\n, or EOF.
// Returns the index of the \n that starts the delimiter, or -1 if not found.
func findClosingDelimiter(data []byte) int {
	offset := 0
	for {
		idx := bytes.Index(data[offset:], []byte("\n---"))
		if idx < 0 {
			return -1
		}
		pos := offset + idx
		afterDelim := pos + 4
		// Valid if followed by \n, \r\n, or EOF
		if afterDelim >= len(data) || data[afterDelim] == '\n' || (data[afterDelim] == '\r' && afterDelim+1 < len(data) && data[afterDelim+1] == '\n') {
			return pos
		}
		offset = pos + 1
	}
}

// ReadingTimeMinutes estimates reading time based on word count.
// Average reading speed: ~200 words per minute.
func ReadingTimeMinutes(text string) int {
	words := len(strings.Fields(text))
	minutes := words / 200
	if minutes < 1 {
		return 1
	}
	return minutes
}
