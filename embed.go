// Package blogflow embeds the default theme, templates, CSS, and configuration.
//
// embed.go lives at the package root (not internal/defaults/) so the //go:embed
// directive can reference the defaults/ directory directly. The overlay FS and
// config system import the Defaults variable from here.
package blogflow

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed defaults/*
var defaultsFS embed.FS

// Defaults is the embedded defaults filesystem with the "defaults/" prefix
// stripped. Paths align with overlay FS expectations
// (e.g., "templates/base.html" not "defaults/templates/base.html").
//
// Note: go:embed excludes files starting with . or _ by default.
var Defaults fs.FS

func init() {
	var err error
	// TODO: emit an slog.Debug message here once the logger is wired up.
	Defaults, err = fs.Sub(defaultsFS, "defaults")
	if err != nil {
		panic(fmt.Sprintf("blogflow: failed to create defaults sub-filesystem: %v", err))
	}
}
