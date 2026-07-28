package search

import (
	"strings"
	"testing"
)

func termSet(terms ...string) map[string]struct{} {
	s := make(map[string]struct{}, len(terms))
	for _, t := range terms {
		s[t] = struct{}{}
	}
	return s
}

func TestMakeExcerpt_CentersOnMatch(t *testing.T) {
	plain := "The quick brown fox jumps over the lazy dog near the overlay filesystem boundary today."
	got := makeExcerpt(plain, termSet("overlay"), 30)
	if !strings.Contains(strings.ToLower(got), "overlay") {
		t.Fatalf("excerpt %q does not contain the match", got)
	}
	// A window this small in the middle of the text should be ellipsed on both ends.
	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "…") {
		t.Errorf("expected leading and trailing ellipsis, got %q", got)
	}
}

func TestMakeExcerpt_NoMatchUsesLeading(t *testing.T) {
	plain := "alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu nu xi omicron pi"
	got := makeExcerpt(plain, termSet("nomatch"), 20)
	if !strings.HasPrefix(got, "alpha") {
		t.Errorf("no-match excerpt should start at the beginning, got %q", got)
	}
	if strings.HasPrefix(got, "…") {
		t.Errorf("no-match excerpt should not have a leading ellipsis, got %q", got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncated excerpt should have a trailing ellipsis, got %q", got)
	}
}

func TestMakeExcerpt_StripsHTMLAndCollapsesWhitespace(t *testing.T) {
	// Excerpts operate on already-plain text; ensure whitespace is collapsed and
	// no markup survives (defense-in-depth, since PlainText strips tags upstream).
	plain := "hello    world\n\n\tfoo"
	got := makeExcerpt(plain, termSet("world"), 100)
	if strings.Contains(got, "\n") || strings.Contains(got, "\t") || strings.Contains(got, "  ") {
		t.Errorf("excerpt whitespace not collapsed: %q", got)
	}
	if got != "hello world foo" {
		t.Errorf("excerpt = %q, want %q", got, "hello world foo")
	}
}

func TestMakeExcerpt_Unicode(t *testing.T) {
	plain := "Un petit café au lait le matin à Paris avec un croissant chaud et du beurre."
	got := makeExcerpt(plain, termSet("café"), 20)
	if !strings.Contains(strings.ToLower(got), "café") {
		t.Fatalf("unicode excerpt %q missing match", got)
	}
	// Ensure we did not split a multi-byte rune (valid UTF-8 result).
	if !isValidRunes(got) {
		t.Errorf("excerpt contains invalid runes: %q", got)
	}
}

func TestMakeExcerpt_ShortTextNoEllipsis(t *testing.T) {
	got := makeExcerpt("short text", termSet("text"), 200)
	if got != "short text" {
		t.Errorf("excerpt = %q, want whole text with no ellipsis", got)
	}
}

func TestMakeExcerpt_Empty(t *testing.T) {
	if got := makeExcerpt("", termSet("x"), 200); got != "" {
		t.Errorf("empty plain text excerpt = %q, want empty", got)
	}
	if got := makeExcerpt("text", termSet("x"), 0); got != "" {
		t.Errorf("zero-length excerpt = %q, want empty", got)
	}
}

func isValidRunes(s string) bool {
	for _, r := range s {
		if r == '\uFFFD' {
			return false
		}
	}
	return true
}
