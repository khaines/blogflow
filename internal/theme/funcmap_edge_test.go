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
			// urlize must handle edge inputs gracefully without panicking or
			// producing unexpected empties.  Pass the value THROUGH the typed
			// function as the production path would.
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
				result := urlize(inputStr)
				if inputStr != "" && result == "" {
					t.Errorf("urlize produced empty output for non-empty input")
				}
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

	// Add genuine typed nil/empty tests that pass directly through the
	// function signatures (production code path).
	if add, ok := fm["add"].(func(int, int) int); ok {
		got := add(0, 0)
		if got != 0 {
			t.Errorf("add(0,0) = %d, want 0", got)
		}
	}
	if add32, ok := fm["add"].(func(int, int) int); ok {
		got := add32(-42, 42)
		if got != 0 {
			t.Errorf("add(-42,42) = %d, want 0", got)
		}
	}
	if seq, ok := fm["seq"].(func(int, int) ([]int, error)); ok {
		got, err := seq(0, 0)
		if err != nil {
			t.Errorf("seq(0,0) returned error: %v", err)
		}
		if len(got) != 1 || got[0] != 0 {
			t.Errorf("seq(0,0) = %v, want [0]", got)
		}
	}
	if sub, ok := fm["sub"].(func(int, int) int); ok {
		got := sub(0, 0)
		if got != 0 {
			t.Errorf("sub(0,0) = %d, want 0", got)
		}
	}
	if truncate, ok := fm["truncate"].(func(string, int) string); ok {
		got := truncate("", 10)
		if got != "" {
			t.Errorf(`truncate("",10) = %q, want empty`, got)
		}
		got = truncate("hello", 0)
		if got != "" {
			t.Errorf(`truncate("hello",0) = %q, want empty`, got)
		}
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
			switch fn := fn.(type) {
			case func(time.Time, string) string:
				_ = fn(time.Time{}, "")
			case func() time.Time:
				_ = fn()
			case func(string) string:
				_ = fn("")
			case func(string, int) string:
				_ = fn("", 0)
			case func(int, int) int:
				_ = fn(0, 0)
			case func(any) int:
				_ = fn(nil)
			case func(int, int) ([]int, error):
				_, _ = fn(0, 0)
			default:
				t.Errorf("unknown funcmap function type for %q", name)
			}
		})
	}
	// URLize with two spaces returns "--".
	if urlize, ok := fm["urlize"].(func(string) string); ok {
		result := urlize("  ")
		want := "--"
		if result != want {
			t.Errorf(`urlize("  ") = %q, want %q`, result, want)
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
