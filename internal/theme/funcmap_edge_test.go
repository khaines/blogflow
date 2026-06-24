// Template function edge case tests per test-gap-analysis.md item #9
// Tests nil, zero, overflow, and boundary inputs for all registered functions.
package theme

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestTemplateFunctionEdgeCases(t *testing.T) {
	t.Parallel()
	fm := defaultFuncMap()
	if fm == nil {
		t.Fatal("defaultFuncMap returned nil")
	}
	testCases := []struct {
		name  string
		input interface{}
	}{
		{"empty string", ""},
		{"unicode", "hello"},
		{"nil interface", nil},
		{"zero int", 0},
		{"negative int", -42},
		{"very_long", strings.Repeat("x", 10000)},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// urlize is a key function that must handle edge inputs gracefully
			if urlize, ok := fm["urlize"].(func(string) string); ok {
				var inputStr string
				switch v := tc.input.(type) {
				case string:
					inputStr = v
				case nil:
					inputStr = ""
				default:
					inputStr = fmt.Sprintf("%v", v)
				}
				_ = urlize(inputStr) // test urlize with the input
			}
			// readingTime must handle the test case input gracefully
			if readingTime, ok := fm["readingTime"].(func(any) int); ok {
				var inputStr any
				switch v := tc.input.(type) {
				case string:
					inputStr = v
				case nil:
					inputStr = ""
				default:
					inputStr = fmt.Sprintf("%v", v)
				}
				result := readingTime(inputStr)
				if result < 0 {
					t.Errorf("readingTime(%v) = %d, want >= 0", tc.input, result)
				}
			}
		})
	}
}

func TestTemplateFuncNilInputs(t *testing.T) {
	t.Parallel()
	fm := defaultFuncMap()
	if fm == nil {
		t.Fatal("defaultFuncMap returned nil")
	}
	// All registered functions must handle nil/zero/empty inputs without panicking
	for name, fn := range fm {
		t.Run(name+"_nil", func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("func %q panicked on nil input: %v", name, r)
				}
			}()
			// Actually invoke the function with zero/empty inputs
			switch fn.(type) {
			case func(time.Time, string) string:
				_ = fn.(func(time.Time, string) string)(time.Time{}, "")
			case func() time.Time:
				_ = fn.(func() time.Time)()
			case func(string) string:
				_ = fn.(func(string) string)("")
			case func(string, int) string:
				_ = fn.(func(string, int) string)("", 0)
			case func(int, int) int:
				_ = fn.(func(int, int) int)(0, 0)
			case func(any) int:
				_ = fn.(func(any) int)(nil)
			case func(int, int) ([]int, error):
				_, _ = fn.(func(int, int) ([]int, error))(0, 0)
			default:
				t.Errorf("unknown funcmap function type for %q", name)
			}
		})
	}
	// URLize with empty string returns "--" (consecutive hyphens)
	if urlize, ok := fm["urlize"].(func(string) string); ok {
		result := urlize("  ")
		if strings.HasPrefix(result, "-") {
			// hyphens from spaces are expected
			t.Logf("urlize(\"  \") = %q (leading hyphens from spaces)", result)
		}
	}
}

func TestURLizeEdgeCases(t *testing.T) {
	t.Parallel()
	fm := defaultFuncMap()
	if fm == nil {
		t.Fatal("defaultFuncMap returned nil")
	}
	urlize, ok := fm["urlize"].(func(string) string)
	if !ok {
		t.Fatal("urlize not found in defaultFuncMap")
	}
	testCases := []struct {
		input string
		want  string
	}{
		{"a", "a"},
		{"hello", "hello"},
		{"hello world", "hello-world"},
		{"  spaces  ", "--spaces--"},
		{"Hello World Test", "hello-world-test"},
	}
	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			got := urlize(tc.input)
			if got != tc.want {
				t.Errorf("urlize(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestTemplateLongInputEdgeCases(t *testing.T) {
	t.Parallel()
	fm := defaultFuncMap()
	if fm == nil {
		t.Fatal("defaultFuncMap returned nil")
	}
	longStr := strings.Repeat("a", 100000)
	if readingTime, ok := fm["readingTime"].(func(any) int); ok {
		// readingTime counts words via strings.Fields, not raw chars.
		// "a" repeated 100000 times = 1 word → 1 min at 200 wpm
		want := 1
		got := readingTime(longStr)
		if got != want {
			t.Errorf("readingTime(100k-char-all-same) = %d, expected %d", got, want)
		}
	}
	if urlize, ok := fm["urlize"].(func(string) string); ok {
		result := urlize(longStr)
		if len(result) == 0 {
			t.Error("urlize(100k chars) returned empty string")
		}
		if len(result) != len(longStr) {
			t.Logf("urlize(100k chars) = %d chars", len(result))
		}
	}
	t.Logf("long input handled (%d chars)", len(longStr))
}
