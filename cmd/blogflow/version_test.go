package main

import (
	"strings"
	"testing"
)

func TestVersionString(t *testing.T) {
	got := versionString()
	if !strings.HasPrefix(got, "blogflow version ") {
		t.Errorf("expected version string to start with 'blogflow version ', got: %s", got)
	}
	if !strings.Contains(got, "commit:") {
		t.Errorf("expected version string to contain 'commit:', got: %s", got)
	}
	if !strings.Contains(got, "built:") {
		t.Errorf("expected version string to contain 'built:', got: %s", got)
	}
}

func TestVersionStringDefaults(t *testing.T) {
	expected := "blogflow version dev (commit: unknown, built: unknown)"
	got := versionString()
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}
