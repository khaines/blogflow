package config

import (
	"testing"
)

func TestValidate(t *testing.T) {
	// Empty config should return a validation error (port range, required fields).
	err := Validate(&Config{})
	if err == nil {
		t.Fatal("Validate(&Config{}) returned nil, expected validation error(s)")
	}
	t.Logf("Validate(&Config{}) correctly returned error: %v", err)
}
