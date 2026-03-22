// Package blogflow embeds the default theme, templates, CSS, and configuration.
package blogflow

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed defaults/*
var defaultsFS embed.FS

// DefaultsFS returns the embedded defaults filesystem with the "defaults/"
// prefix stripped. Paths align with overlay FS expectations
// (e.g., "templates/base.html" not "defaults/templates/base.html").
//
// Note: go:embed excludes files starting with . or _ by default.
var DefaultsFS fs.FS

func init() {
	var err error
	DefaultsFS, err = fs.Sub(defaultsFS, "defaults")
	if err != nil {
		panic(fmt.Sprintf("blogflow: failed to create defaults sub-filesystem: %v", err))
	}
}
