// Package search provides BlogFlow's optional server-side full-text search:
// a lightweight in-memory inverted index built from the content scan, with
// Unicode-aware tokenization, deterministic weighted TF-IDF ranking, and
// on-demand plain-text excerpts. It requires no client-side JavaScript.
//
// The index is immutable once built and is published together with the
// content generation it was derived from, so a query never mixes a search
// index with content from a different generation.
package search

import (
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// normalize applies the canonical v1 search normalization: Unicode NFC
// normalization followed by lower-casing. Query strings and document text are
// normalized identically before tokenization and before any rune-length
// counting so that matching is deterministic and case-insensitive.
func normalize(s string) string {
	return strings.ToLower(norm.NFC.String(s))
}

// isTokenRune reports whether r is part of a token. Tokens are maximal runs of
// letters and digits; everything else (whitespace, punctuation, symbols) is a
// delimiter. This preserves CJK text as whole-rune tokens; CJK bigrams are
// deliberately deferred to a future revision.
func isTokenRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// tokenizeInto appends the tokens of an already-normalized string to dst and
// returns the extended slice. Splitting is allocation-light and uses no
// regular expressions (ReDoS-safe).
func tokenizeInto(dst []string, normalized string) []string {
	start := -1
	for i, r := range normalized {
		if isTokenRune(r) {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			dst = append(dst, normalized[start:i])
			start = -1
		}
	}
	if start >= 0 {
		dst = append(dst, normalized[start:])
	}
	return dst
}

// tokenizeNormalized splits an already-normalized string into tokens.
func tokenizeNormalized(normalized string) []string {
	return tokenizeInto(nil, normalized)
}

// tokenize normalizes then tokenizes a raw string.
func tokenize(s string) []string {
	return tokenizeNormalized(normalize(s))
}

// parseQueryTerms normalizes, tokenizes, de-duplicates, and lexicographically
// sorts the query's terms. Duplicate normalized terms are collapsed so that a
// query like "go go" scores identically to "go". Sorting makes score
// accumulation order deterministic.
func parseQueryTerms(q string) []string {
	raw := tokenize(q)
	if len(raw) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(raw))
	terms := make([]string, 0, len(raw))
	for _, t := range raw {
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		terms = append(terms, t)
	}
	sort.Strings(terms)
	return terms
}

// countRunes returns the number of runes in s. Used for query length limits,
// which are expressed in normalized runes rather than bytes.
func countRunes(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

// NormalizeQuery trims surrounding whitespace and applies the canonical search
// normalization (Unicode NFC + lower-case). The handler uses it to detect
// empty/whitespace queries and to count query length in normalized runes.
func NormalizeQuery(raw string) string {
	return normalize(strings.TrimSpace(raw))
}

// RuneLen returns the number of runes in s. Exposed so the handler can count
// normalized query length for min/max length validation.
func RuneLen(s string) int {
	return countRunes(s)
}

// CountQueryTerms returns the number of unique normalized terms in a raw query.
// The handler uses it as a span attribute without exposing the raw query.
func CountQueryTerms(q string) int {
	return len(parseQueryTerms(q))
}
