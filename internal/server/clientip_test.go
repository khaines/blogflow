package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClientIPResolver_InvalidCIDR(t *testing.T) {
	_, err := NewClientIPResolver([]string{"not-a-cidr"})
	if err == nil {
		t.Fatal("expected error for invalid CIDR, got nil")
	}
}

func TestClientIP_NoTrustedProxies(t *testing.T) {
	resolver, err := NewClientIPResolver(nil)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.50:12345"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("X-Real-IP", "5.6.7.8")

	got := resolver.ClientIP(req)
	if got != "203.0.113.50" {
		t.Errorf("ClientIP = %q, want %q (should ignore forwarded headers)", got, "203.0.113.50")
	}
}

func TestClientIP_TrustedProxy_XForwardedFor(t *testing.T) {
	resolver, err := NewClientIPResolver([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 10.0.0.5")

	got := resolver.ClientIP(req)
	if got != "203.0.113.50" {
		t.Errorf("ClientIP = %q, want %q", got, "203.0.113.50")
	}
}

func TestClientIP_TrustedProxy_XRealIP(t *testing.T) {
	resolver, err := NewClientIPResolver([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Real-IP", "203.0.113.99")

	got := resolver.ClientIP(req)
	if got != "203.0.113.99" {
		t.Errorf("ClientIP = %q, want %q", got, "203.0.113.99")
	}
}

func TestClientIP_TrustedProxy_XFFPreferredOverXRealIP(t *testing.T) {
	resolver, err := NewClientIPResolver([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.1")
	req.Header.Set("X-Real-IP", "203.0.113.99")

	got := resolver.ClientIP(req)
	if got != "198.51.100.1" {
		t.Errorf("ClientIP = %q, want %q (XFF should take precedence)", got, "198.51.100.1")
	}
}

func TestClientIP_TrustedProxy_AllXFFTrusted_FallsToXRealIP(t *testing.T) {
	resolver, err := NewClientIPResolver([]string{"10.0.0.0/8", "172.16.0.0/12"})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "10.0.0.2, 172.16.0.5")
	req.Header.Set("X-Real-IP", "203.0.113.77")

	got := resolver.ClientIP(req)
	if got != "203.0.113.77" {
		t.Errorf("ClientIP = %q, want %q (all XFF trusted, should fall to X-Real-IP)", got, "203.0.113.77")
	}
}

func TestClientIP_TrustedProxy_NoHeaders_FallsToRemoteAddr(t *testing.T) {
	resolver, err := NewClientIPResolver([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"

	got := resolver.ClientIP(req)
	if got != "10.0.0.1" {
		t.Errorf("ClientIP = %q, want %q", got, "10.0.0.1")
	}
}

func TestClientIP_UntrustedProxy(t *testing.T) {
	resolver, err := NewClientIPResolver([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1:5555"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")

	got := resolver.ClientIP(req)
	if got != "192.168.1.1" {
		t.Errorf("ClientIP = %q, want %q (proxy not trusted)", got, "192.168.1.1")
	}
}

func TestClientIP_IPv6(t *testing.T) {
	resolver, err := NewClientIPResolver([]string{"fd00::/8"})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "[fd00::1]:8080"
	req.Header.Set("X-Forwarded-For", "2001:db8::1")

	got := resolver.ClientIP(req)
	if got != "2001:db8::1" {
		t.Errorf("ClientIP = %q, want %q", got, "2001:db8::1")
	}
}

func TestClientIP_MultipleXFF_SkipsTrusted(t *testing.T) {
	resolver, err := NewClientIPResolver([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.1, 10.0.0.2, 10.0.0.3")

	got := resolver.ClientIP(req)
	if got != "203.0.113.1" {
		t.Errorf("ClientIP = %q, want %q (should skip trusted proxies right-to-left)", got, "203.0.113.1")
	}
}

func TestExtractIP(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1.2.3.4:5678", "1.2.3.4"},
		{"[::1]:80", "::1"},
		{"1.2.3.4", "1.2.3.4"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := extractIP(tt.input); got != tt.want {
				t.Errorf("extractIP(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
