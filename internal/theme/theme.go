// Package theme loads Go html/templates from the overlay FS and renders pages.
//
// The Engine reads .html files from a templates/ directory within the provided
// fs.FS (typically an OverlayFS), parses them with custom template functions,
// and caches the result. Callers render named templates with arbitrary data.
package theme

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"strings"
	"sync/atomic"
	"time"
)

// Engine loads and caches templates from the overlay FS.
type Engine struct {
	fs    fs.FS
	tmpl  atomic.Pointer[template.Template]
	funcs template.FuncMap
}

// NewEngine creates a theme engine that reads templates from the given fs.FS.
// The fs should be an OverlayFS or similar — templates are resolved through it.
func NewEngine(fsys fs.FS) (*Engine, error) {
	e := &Engine{
		fs:    fsys,
		funcs: defaultFuncMap(),
	}
	if err := e.loadTemplates(); err != nil {
		return nil, err
	}
	return e, nil
}

// Render renders a named template to the writer with the given data.
// Output is buffered so that partial content is never written on error.
func (e *Engine) Render(w io.Writer, name string, data any) error {
	var buf bytes.Buffer
	if err := e.tmpl.Load().ExecuteTemplate(&buf, name, data); err != nil {
		return err
	}
	_, err := buf.WriteTo(w)
	return err
}

// RenderToString renders a named template to a string.
func (e *Engine) RenderToString(name string, data any) (string, error) {
	var b strings.Builder
	if err := e.Render(&b, name, data); err != nil {
		return "", err
	}
	return b.String(), nil
}

// Reload re-reads all templates from the filesystem.
// Called after content/theme changes for hot-reload.
func (e *Engine) Reload() error {
	return e.loadTemplates()
}

func (e *Engine) loadTemplates() error {
	tmpl := template.New("").Funcs(e.funcs)

	err := fs.WalkDir(e.fs, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".html") {
			return nil
		}

		data, readErr := fs.ReadFile(e.fs, path)
		if readErr != nil {
			return fmt.Errorf("reading template %s: %w", path, readErr)
		}

		// Use the full path as template name (e.g., "templates/post.html").
		if _, parseErr := tmpl.New(path).Parse(string(data)); parseErr != nil {
			return fmt.Errorf("parsing template %s: %w", path, parseErr)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("loading templates: %w", err)
	}

	e.tmpl.Store(tmpl)
	return nil
}

// defaultFuncMap returns the standard template functions.
func defaultFuncMap() template.FuncMap {
	return template.FuncMap{
		"formatDate": func(t time.Time, layout string) string {
			return t.Format(layout)
		},
		"now": func() time.Time {
			return time.Now()
		},
		"lower": strings.ToLower,
		"upper": strings.ToUpper,
		"truncate": func(s string, n int) string {
			if n <= 0 {
				return ""
			}
			runes := []rune(s)
			if len(runes) <= n {
				return s
			}
			truncated := string(runes[:n])
			if idx := strings.LastIndex(truncated, " "); idx > 0 {
				return truncated[:idx] + "…"
			}
			return truncated + "…"
		},
		"readingTime": func(content string) int {
			words := len(strings.Fields(content))
			if m := words / 200; m > 0 {
				return m
			}
			return 1
		},
		// safeHTML was intentionally removed — content from the scanner is
		// already typed as template.HTML, so a conversion helper is unnecessary
		// and would widen the XSS attack surface.
		"urlize": func(s string) string {
			s = strings.ToLower(s)
			s = strings.ReplaceAll(s, " ", "-")
			var b strings.Builder
			for _, r := range s {
				if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
					b.WriteRune(r)
				}
			}
			return b.String()
		},
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		"seq": func(start, end int) ([]int, error) {
			if end-start > 10000 || end-start < 0 {
				return nil, fmt.Errorf("seq: range %d exceeds maximum 10000", end-start)
			}
			s := make([]int, 0, end-start+1)
			for i := start; i <= end; i++ {
				s = append(s, i)
			}
			return s, nil
		},
	}
}
