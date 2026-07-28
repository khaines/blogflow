package search

import (
	"reflect"
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"lowercase", "Go Internals", "go internals"},
		{"unicode nfc + fold", "CAFÉ", "café"},
		{"already normal", "overlay filesystem", "overlay filesystem"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalize(tt.in); got != tt.want {
				t.Errorf("normalize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"simple", "go go search", []string{"go", "go", "search"}},
		{"punctuation delimiters", "hello, world! foo-bar", []string{"hello", "world", "foo", "bar"}},
		{"mixed case", "Go INTERNALS", []string{"go", "internals"}},
		{"digits kept", "go1.26 release", []string{"go1", "26", "release"}},
		{"empty", "", nil},
		{"only delimiters", "  --  ", nil},
		{"leading/trailing delims", "!go!", []string{"go"}},
		{"unicode accent folded to single token", "café au lait", []string{"café", "au", "lait"}},
		{"cjk kept whole (no bigrams)", "東京", []string{"東京"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenize(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("tokenize(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseQueryTerms_DedupeAndSort(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"dedupe repeated", "go go", []string{"go"}},
		{"sort lexicographically", "search go", []string{"go", "search"}},
		{"dedupe and sort", "Go SEARCH go search", []string{"go", "search"}},
		{"empty", "   ", nil},
		{"punctuation only", "!!! ???", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseQueryTerms(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseQueryTerms(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeQueryAndRuneLen(t *testing.T) {
	// Trimming + normalization is what the handler counts for length limits.
	if got := NormalizeQuery("   Hello World   "); got != "hello world" {
		t.Errorf("NormalizeQuery trim/normalize = %q", got)
	}
	if got := NormalizeQuery("   \t\n "); got != "" {
		t.Errorf("NormalizeQuery whitespace-only = %q, want empty", got)
	}
	// Rune length counts runes, not bytes (é is one rune, two bytes).
	if got := RuneLen("café"); got != 4 {
		t.Errorf("RuneLen(café) = %d, want 4", got)
	}
	if got := RuneLen("東京"); got != 2 {
		t.Errorf("RuneLen(東京) = %d, want 2", got)
	}
}
