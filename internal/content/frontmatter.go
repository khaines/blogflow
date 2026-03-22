package content

import (
	"bytes"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
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
func ParseFrontMatter(data []byte) (*FrontMatter, []byte, error) {
	if !bytes.HasPrefix(data, []byte("---")) {
		return nil, data, nil
	}

	rest := data[3:]
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	} else if len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n' {
		rest = rest[2:]
	}

	end := bytes.Index(rest, []byte("\n---"))
	if end == -1 {
		return nil, nil, fmt.Errorf("front matter: missing closing ---")
	}

	fmData := rest[:end]
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

	return &fm, body, nil
}

// ReadingTimeMinutes estimates reading time based on word count.
// Average reading speed: ~200 words per minute.
func ReadingTimeMinutes(text string) int {
	words := len(bytes.Fields([]byte(text)))
	minutes := words / 200
	if minutes < 1 {
		return 1
	}
	return minutes
}
