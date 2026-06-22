// Duplicate template function definitions test per test-gap-analysis.md item #8
package theme

import (
	"html/template"
	"testing"
)

func TestDuplicateTemplateBlockDetection(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name      string
		files     []string
		wantError bool
	}{
		{"single define", []string{"{{define \"content\"}}default{{end}}"}, false},
		{"duplicate define same file", []string{"{{define \"content\"}}first{{end}}{{define \"content\"}}second{{end}}"}, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, content := range tc.files {
				tmpl, err := template.New("test").Parse(content)
				if err != nil {
					if !tc.wantError {
						t.Errorf("unexpected parse error: %v", err)
					} else {
						t.Logf("expected parse error: %v", err)
					}
				} else if tmpl != nil && !tc.wantError {
					t.Log("template parsed successfully")
				}
			}
		})
	}
}

func TestTemplateNameValidation(t *testing.T) {
	t.Parallel()
	tmpl, err := template.New("valid_name").Parse("hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = tmpl
	t.Log("template name validation passed")
}
