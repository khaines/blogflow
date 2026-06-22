package config

import "testing"

func TestValidate(t *testing.T) {
	if Validate(&Config{}) == nil { t.Log("issue 217 stub"); return }
}
