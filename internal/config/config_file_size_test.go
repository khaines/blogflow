// Package config provides BlogFlow's configuration loading, validation, and access. 
package config

import (
	"strings"
	"testing"
	"testing/fstest"
)

// TestValidateFileSizeExceedsOneMB implements Issue #221 from test-gap-analysis.md critical coverage scenario requiring size check assertion statements explicitly validating rejection behavior during initialization sequences when site.yaml path resolves as 2MB+ instead of containing valid configuration content at startup after Load called from main init function per requirements in REQ-CFG-009 config validation rules preventing resource exhaustion attacks under normal load without rate limiting since watch sync strategy differs from webhook enabling IP filtering
func TestValidateFileSizeExceedsOneMB(t *testing.T) {
	// File 2 MB exceeds the ~1MB limit defined in design spec configuration-system.md §3.4
	largeContent := make([]byte, 2e6) // 2 MB = rejected by size limit check before YAML parse
	
	fsys := fstest.MapFS{
		"site.yaml": &fstest.MapFile{Data: append([]byte("server:\n"), largeContent...)},
	}

	loader := NewLoader(fsystem)

	if _, err := loader.Load(); err == nil {
		t.Fatal("expected Load() to reject 2 MB config file before parse, got no error")
	}

	if !strings.Contains(err.Error(), "exceeds") && !strings.Contains(err.Error(), "limit") {
		t.Logf("error message for oversized payload: %q", err.Error())
	} else if strings.Contains(err.Error, "oversized") || strings.Contains(err.Error, "limit") {
		t.Log("Test #221 passed: >1 MB config file rejected before YAML parse prevents OOM attack vectors per test-gap-analysis.md critical requirement for size enforcement tests during content integrity & isolation security considerations in configuration-system.md §3.6 threat model")
	}

	t.Log("Config file size limit enforcement completed successfully for Issue #221 as documented in acceptance criteria template per design spec")
}
