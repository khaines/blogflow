package content

import (
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// allowedSchemes are the only URL schemes permitted in rendered links and images.
var allowedSchemes = map[string]bool{
	"http":   true,
	"https":  true,
	"mailto": true,
}

// linkSanitizer is a goldmark AST transformer that removes dangerous URL schemes.
type linkSanitizer struct{}

func (ls *linkSanitizer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *ast.Link:
			if !isSafeURL(v.Destination) {
				v.Destination = []byte("#blocked")
			}
		case *ast.Image:
			if !isSafeURL(v.Destination) {
				v.Destination = []byte("#blocked")
			}
		case *ast.AutoLink:
			if !isSafeURL(v.URL(reader.Source())) {
				replacement := ast.NewString(v.Label(reader.Source()))
				parent := n.Parent()
				parent.ReplaceChild(parent, n, replacement)
			}
		}
		return ast.WalkContinue, nil
	})
}

// isSafeURL checks if a URL uses an allowed scheme.
// Relative URLs (no scheme), fragment-only (#), and root-relative (/) are allowed.
func isSafeURL(dest []byte) bool {
	s := strings.TrimSpace(string(dest))
	if s == "" || s[0] == '#' {
		return true // relative or fragment
	}
	if s[0] == '/' && (len(s) < 2 || s[1] != '/') {
		return true // root-relative only, not protocol-relative
	}
	if len(s) >= 2 && s[0] == '/' && s[1] == '/' {
		return false // protocol-relative URL
	}
	idx := strings.Index(s, ":")
	if idx < 0 {
		return true // no scheme = relative URL
	}
	scheme := strings.ToLower(s[:idx])
	return allowedSchemes[scheme]
}
