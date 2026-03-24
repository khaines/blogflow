// Package theme loads Go html/templates from the overlay FS and renders
// blog pages. Base template and partials are shared; page templates are
// cloned per request so their {{define}} blocks don't collide.
package theme

import (
	"bytes"
	"context"
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
	base  atomic.Pointer[template.Template] // base.html + partials
	pages atomic.Pointer[map[string]string] // page template sources keyed by path
	funcs template.FuncMap
}

// NewEngine creates a theme engine that reads templates from the given fs.FS.
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

// Render renders a page using base.html with the named page template's
// block overrides ({{define "content"}}, {{define "title"}}, etc.).
// Each page template is cloned+parsed per request so defines don't collide.
// The context allows callers to cancel long-running renders.
func (e *Engine) Render(ctx context.Context, w io.Writer, name string, data any) error {
	pages := *e.pages.Load()
	src, ok := pages[name]
	if !ok {
		return fmt.Errorf("theme: template %q not found", name)
	}

	clone, err := e.base.Load().Clone()
	if err != nil {
		return fmt.Errorf("theme: clone: %w", err)
	}
	if _, err := clone.Parse(src); err != nil {
		return fmt.Errorf("theme: parse page %s: %w", name, err)
	}

	// Check for cancellation before expensive template execution.
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	var buf bytes.Buffer
	if err := clone.ExecuteTemplate(&buf, "templates/base.html", data); err != nil {
		return err
	}
	_, writeErr := buf.WriteTo(w)
	return writeErr
}

// RenderToString renders a named template to a string.
func (e *Engine) RenderToString(ctx context.Context, name string, data any) (string, error) {
	var b strings.Builder
	if err := e.Render(ctx, &b, name, data); err != nil {
		return "", err
	}
	return b.String(), nil
}

// Reload re-reads all templates from the filesystem.
func (e *Engine) Reload() error {
	return e.loadTemplates()
}

func (e *Engine) loadTemplates() error {
	base := template.New("").Funcs(e.funcs)
	pages := make(map[string]string)

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

		src := string(data)

		// Base template and partials go into the shared base set.
		// Page templates (list, post, page, 404) are stored separately
		// and cloned+parsed per request so their defines don't collide.
		//
		// Convention: partials may use {{define "name"}} blocks to export
		// a short alias (e.g., {{define "post-meta"}}). Without a define
		// block, the partial is registered under its full WalkDir path
		// (e.g., "templates/partials/header.html"). Both patterns work.
		if strings.Contains(path, "partials/") || path == "templates/base.html" {
			if _, parseErr := base.New(path).Parse(src); parseErr != nil {
				return fmt.Errorf("parsing template %s: %w", path, parseErr)
			}
		} else {
			pages[path] = src
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("loading templates: %w", err)
	}

	// Validate base.html exists — all page renders depend on it.
	if base.Lookup("templates/base.html") == nil {
		return fmt.Errorf("loading templates: templates/base.html not found (required)")
	}

	e.base.Store(base)
	e.pages.Store(&pages)
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
		"readingTime": func(content any) int {
			words := len(strings.Fields(fmt.Sprint(content)))
			if m := words / 200; m > 0 {
				return m
			}
			return 1
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
