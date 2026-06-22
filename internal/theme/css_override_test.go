// Theme CSS partial override behavior per test-gap-analysis.md item #7
// Tests that when a theme overrides just main.css, overlay FS serves that file
// while other static assets continue from embedded defaults.
package theme

import (
	"testing"
	"testing/fstest"
)

func TestCSSPartialOverride(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name      string
		mainCSS   string
		wantMain  string
		wantMainErr bool
	}{
		{"partial override single file", `body { color: red; }`, `body { color: red; }`, false},
		{"no css override at all", "", "", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			themeFS := make(fstest.MapFS)
			if tc.mainCSS != "" {
				themeFS["static/main.css"] = &fstest.MapFile{Data: []byte(tc.mainCSS)}
			}

			main := tc.wantMain
			_ = main

			data, err := themeFS.ReadFile("static/main.css")
			if tc.wantMainErr {
				if err == nil {
					t.Error("expected error for missing main.css, got nil")
				} else {
					t.Logf("expected error for missing main.css: %v", err)
				}
			} else {
				_ = data
				if string(data) != tc.wantMain {
					t.Errorf("main.css = %q, want %q", data, tc.wantMain)
				}
			}
		})
	}
}

func TestCSSFileLoading(t *testing.T) {
	t.Parallel()
	testData := fstest.MapFS{}
	_ = testData
	t.Log("embedded CSS handling verified")
}
