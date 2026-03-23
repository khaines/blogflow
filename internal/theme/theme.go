// Package theme loads Go html/templates from the overlay FS and renders pages.
//
// The Engine reads .html files from a templates/ directory within the provided
// fs.FS (typically an OverlayFS), parses them with custom template functions,
// and caches the result. Callers render named templates with arbitrary data.
package theme

import (
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"strings"
	"time"
)

// Engine loads and caches templates from the overlay FS.
type Engine struct {
	fs    fs.FS
	tmpl  *template.Template
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
func (e *Engine) Render(w io.Writer, name string, data any) error {
	return e.tmpl.ExecuteTemplate(w, name, data)
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

	e.tmpl = tmpl
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
			if len(s) <= n {
				return s
			}
			if idx := strings.LastIndex(s[:n], " "); idx > 0 {
				return s[:idx] + "…"
			}
			return s[:n] + "…"
		},
		"readingTime": func(content string) int {
			words := len(strings.Fields(content))
			if m := words / 200; m > 0 {
				return m
			}
			return 1
		},
		// WARNING: safeHTML bypasses html/template auto-escaping.
		// Only use for content that has already been sanitized by the
		// goldmark render pipeline (which strips raw HTML by default).
		// Never pass user-supplied strings through this function.
		"safeHTML": func(s string) template.HTML {
			return template.HTML(s)
		},
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
		"seq": func(start, end int) []int {
			var s []int
			for i := start; i <= end; i++ {
				s = append(s, i)
			}
			return s
		},
	}
}
