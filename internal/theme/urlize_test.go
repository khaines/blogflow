package theme

import "testing"

func TestUrlize(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Backward-compatible ASCII behavior.
		{name: "simple ascii", input: "Hello World", want: "hello-world"},
		{name: "punctuation stripped", input: "Hello, World!", want: "hello-world"},
		{name: "multiple spaces", input: "a  b", want: "a--b"},
		{name: "empty string", input: "", want: ""},
		{name: "already a slug", input: "my-post", want: "my-post"},

		// Latin diacritics — decompose + strip combining marks.
		{name: "cafe acute", input: "café", want: "cafe"},
		{name: "german umlauts", input: "Ärger über Öl", want: "arger-uber-ol"},
		{name: "german eszett", input: "Straße", want: "strasse"},
		{name: "spanish enye", input: "Año Nuevo", want: "ano-nuevo"},
		{name: "mixed diacritics", input: "Crème Brûlée", want: "creme-brulee"},

		// Non-decomposable Latin characters — explicit transliteration.
		{name: "ae ligature", input: "Ærodynamik", want: "aerodynamik"},
		{name: "oe ligature", input: "Œuvre", want: "oeuvre"},
		{name: "danish oe", input: "Ørsted", want: "orsted"},
		{name: "polish l stroke", input: "Łódź", want: "lodz"},
		{name: "icelandic thorn", input: "Þór", want: "thor"},
		{name: "icelandic eth", input: "Norðurland", want: "nordurland"},

		// Turkish İ — combining dot above must be stripped after NFKD.
		{name: "turkish dotted I", input: "İstanbul", want: "istanbul"},

		// Zero-width / invisible characters — must be stripped.
		{name: "zero-width space", input: "hello\u200Bworld", want: "helloworld"},
		{name: "zero-width joiner", input: "hello\u200Dworld", want: "helloworld"},
		{name: "zero-width non-joiner", input: "hello\u200Cworld", want: "helloworld"},
		{name: "soft hyphen", input: "break\u00ADpoint", want: "breakpoint"},
		{name: "BOM", input: "\uFEFFtitle", want: "title"},

		// CJK — characters preserved as-is.
		{name: "chinese title", input: "你好世界", want: "你好世界"},
		{name: "japanese title", input: "東京タワー", want: "東京タワー"},
		{name: "korean title", input: "서울 타워", want: "서울-타워"},

		// Mixed scripts.
		{name: "mixed latin-cjk", input: "Go语言 Blog", want: "go语言-blog"},
		{name: "cyrillic", input: "Привет Мир", want: "привет-мир"},
		{name: "arabic", input: "مرحبا بالعالم", want: "مرحبا-بالعالم"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := urlize(tt.input)
			if got != tt.want {
				t.Errorf("urlize(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
