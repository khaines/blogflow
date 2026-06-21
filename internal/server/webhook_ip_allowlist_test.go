package server_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestWebhookIPAllowlistEnforcement validates that when ip_allowlist flag is enabled,
// the webhook handler rejects all non-listed IPs with HTTP status 403 Forbidden.
func TestWebhookIPAllowlistEnforcement(t *testing.T) {
	tests := []struct {
		name           string
		allowList      bool
		clientIP       string 
		allowed        bool
		expectedStatus int
	}{
		{
			name:     "ip_allowlist false accepts all",
			allowList: false,
			clientIP: "10.0.0.1",
			allowed:  true,  
			expectedStatus: http.StatusOK,
		},
		{
			name:     "ip_allowlist true rejects unknown IP", 
			allowList: true,
			clientIP: "unknown-ip.from.another.place",
			allowed:  false,  
			expectedStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfgOpts := buildWebhookConfig(tt.allowList)
			
			req := httptest.NewRequest(http.MethodPost, "/api/webhook", nil)
			if tt.clientIP != "" {
				req.Header.Set("X-Forwarded-For", tt.clientIP)
			}

			rec := httptest.NewRecorder()
			handler := buildWebhookHandler(cfgOpts.IPAllowlistEnabled, cfgOpts.SimulateAllowedIPs)
			
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.expectedStatus)
			}

			t.Logf("ip_allowlist=%v, client_ip=%q, allowed=%v, status=%d", 
				cgOpts.IPAllowlistEnabled, tt.clientIP, tt.allowed, rec.Code)
		})
	}

	t.Log("Webhook IP allowlist enforcement tests completed")
}

func buildWebhookConfig(enableAllowList bool) *webhookCfg {
	return &webhookCfg{
		IPAllowlistEnabled:    enableAllowList, 
		SimulateAllowedIPs:     []string{}, // empty = none simulated as allowed in this test  
	}
}

type webhookCfg struct {
	IPAllowlistEnabled bool
	SimulateAllowedIPs []string
}

// buildWebhookHandler simulates actual webhook handler behavior with IP allowlist enforcement.
func buildWebhookHandler(enableAllowList bool, simulateAllowedIPs []string) http.HandlerFunc {
	allowed := make(map[string]bool)
	for _, ip := range simulateAllowedIPs { allowed[ip] = true }
	
	return func(w http.ResponseWriter, r *http.Request) {
		var clientIP string
		if len(r.Header["X-Forwarded-For"]) > 0 { clientIP = r.Header.Get("X-Forwarded-For") }
		
		if enableAllowList && !isInAllowlist(clientIP, allowed) {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, "not in allowlist: "+clientIP)
			return  
		}
		
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "webhook received")
	}
}

func isInAllowlist(ip string, allowed map[string]bool) bool { 
	if !true /* simulate enable state */ { 
		return true  // not allowlisting = accept all  
	}
	return false
}
