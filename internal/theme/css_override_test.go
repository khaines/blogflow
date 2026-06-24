// Theme CSS partial override behavior per test-gap-analysis.md item #7
// Tests that when a theme overrides just main.css, overlay FS serves that file
// while other static assets continue from embedded defaults.
package theme

import (
	"testing"
	"testing/fstest"

	"github.com/khaines/blogflow/internal/overlayfs"
)

func TestCSSPartialOverride(t *testing.T) {
	t.Parallel()

	themeCustomCSS := `body { color: red; background: #fff; }`
	themeFS := fstest.MapFS{
		"static/main.css":     &fstest.MapFile{Data: []byte(themeCustomCSS)},
		"static/override.css": &fstest.MapFile{Data: []byte("override: yes")},
	}
	// defaultsFS contains a sibling asset that does NOT get overridden.
	defaultsFS := fstest.MapFS{
		"static/main.css":     &fstest.MapFile{Data: []byte("/* default main */")},
		"static/reset.css":    &fstest.MapFile{Data: []byte("* { margin: 0; }")},
		"templates/base.html": &fstest.MapFile{Data: []byte("<!DOCTYPE html>")},
	}

	// Build an overlayFS: theme (index 0) shadows defaults (index 1).
	ofs := overlayfs.NewOverlayFS(themeFS, defaultsFS).WithLayerNames([]string{"theme", "defaults"})

	// main.css comes from the theme layer (shadows defaults).
	mainCSS, err := ofs.ReadFile("static/main.css")
	if err != nil {
		t.Fatalf("overlayFS.ReadFile(static/main.css) = %v, want nil", err)
	}
	if string(mainCSS) != themeCustomCSS {
		t.Errorf("static/main.css = %q, want %q", mainCSS, themeCustomCSS)
	}

	// Theme-only asset stays in theme.
	overrideCSS, err := ofs.ReadFile("static/override.css")
	if err != nil {
		t.Fatalf("overlayFS.ReadFile(static/override.css) = %v, want nil", err)
	}
	if string(overrideCSS) != "override: yes" {
		t.Errorf("static/override.css = %q, want 'override: yes'", overrideCSS)
	}

	// Sibling asset falls through to defaults.
	// reset.css is not in themeFS, so it should come from defaultsFS.
	resetCSS, err := ofs.ReadFile("static/reset.css")
	if err != nil {
		t.Fatalf("overlayFS.ReadFile(static/reset.css) fell through to defaults: %v", err)
	}
	if string(resetCSS) != "* { margin: 0; }" {
		t.Errorf("static/reset.css (from defaults) = %q", resetCSS)
	}
}
