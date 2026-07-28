package config

import "testing"

func hasFieldError(err error, field string) bool {
	cfgErr, ok := err.(*ConfigError)
	if !ok {
		return false
	}
	for _, fe := range cfgErr.Errors {
		if fe.Field == field {
			return true
		}
	}
	return false
}

func TestValidate_SearchDefaultsValid(t *testing.T) {
	cfg := Default()
	cfg.Search.Enabled = true
	if err := Validate(cfg); err != nil {
		t.Fatalf("default search config should validate, got %v", err)
	}
}

func TestValidate_SearchCapsValidatedEvenWhenDisabled(t *testing.T) {
	// Search tunables are consumed by the restart-scoped reloader whenever the
	// route was registered at startup, so caps are validated regardless of
	// enabled. An explicitly invalid cap must fail even with search disabled.
	cfg := Default()
	cfg.Search.Enabled = false
	cfg.Search.MaxResults = 0
	cfg.Search.MaxIndexBytes = 0
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for invalid search caps even when disabled")
	}
	if !hasFieldError(err, "search.max_results") || !hasFieldError(err, "search.max_index_bytes") {
		t.Errorf("expected search cap field errors, got %v", err)
	}
}

func TestValidate_SearchDefaultDisabledValid(t *testing.T) {
	// The default disabled config (valid caps) must still pass.
	cfg := Default()
	cfg.Search.Enabled = false
	if err := Validate(cfg); err != nil {
		t.Fatalf("default disabled search config should validate, got %v", err)
	}
}

func TestValidate_SearchBounds(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		field  string
	}{
		{"max_results too low", func(c *Config) { c.Search.MaxResults = 0 }, "search.max_results"},
		{"max_results too high", func(c *Config) { c.Search.MaxResults = 101 }, "search.max_results"},
		{"min_query_length zero", func(c *Config) { c.Search.MinQueryLength = 0 }, "search.min_query_length"},
		{"max_query_length too low", func(c *Config) { c.Search.MaxQueryLength = 4 }, "search.max_query_length"},
		{"max < min", func(c *Config) { c.Search.MinQueryLength = 10; c.Search.MaxQueryLength = 8 }, "search.max_query_length"},
		{"max_query_terms zero", func(c *Config) { c.Search.MaxQueryTerms = 0 }, "search.max_query_terms"},
		{"excerpt too low", func(c *Config) { c.Search.ExcerptLength = 10 }, "search.excerpt_length"},
		{"max_docs zero", func(c *Config) { c.Search.MaxDocs = 0 }, "search.max_docs"},
		{"max_tokens zero", func(c *Config) { c.Search.MaxTokens = 0 }, "search.max_tokens"},
		{"max_index_bytes below 1MiB", func(c *Config) { c.Search.MaxIndexBytes = 1024 }, "search.max_index_bytes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			cfg.Search.Enabled = true
			tt.mutate(cfg)
			err := Validate(cfg)
			if err == nil {
				t.Fatalf("expected validation error for %s", tt.field)
			}
			if !hasFieldError(err, tt.field) {
				t.Errorf("expected field error %q, got %v", tt.field, err)
			}
		})
	}
}
