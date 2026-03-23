package server

import (
	"net"
	"net/http"
	"strings"
)

// ClientIPResolver resolves the real client IP from an HTTP request, taking
// into account trusted reverse-proxy headers when the request originates
// from a trusted proxy CIDR.
type ClientIPResolver struct {
	trusted []*net.IPNet
}

// NewClientIPResolver creates a resolver with the given list of trusted
// proxy CIDRs (e.g. "10.0.0.0/8", "172.16.0.0/12"). When the list is
// empty, forwarded headers are never trusted and RemoteAddr is always used.
func NewClientIPResolver(cidrs []string) (*ClientIPResolver, error) {
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, err
		}
		nets = append(nets, ipNet)
	}
	return &ClientIPResolver{trusted: nets}, nil
}

// ClientIP returns the best-effort real client IP for the request.
//
// If RemoteAddr is from a trusted proxy CIDR, X-Forwarded-For (right-most
// untrusted entry) or X-Real-IP is used. Otherwise RemoteAddr is returned.
func (c *ClientIPResolver) ClientIP(r *http.Request) string {
	remoteIP := extractIP(r.RemoteAddr)
	if !c.isTrusted(remoteIP) {
		return remoteIP
	}

	// Prefer X-Forwarded-For: walk from right to left, skip trusted proxies.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		for i := len(ips) - 1; i >= 0; i-- {
			ip := strings.TrimSpace(ips[i])
			if ip == "" {
				continue
			}
			if !c.isTrusted(ip) {
				return ip
			}
		}
		// All entries are trusted — fall through to X-Real-IP / RemoteAddr.
	}

	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}

	return remoteIP
}

// isTrusted reports whether ip falls within any configured trusted CIDR.
func (c *ClientIPResolver) isTrusted(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, n := range c.trusted {
		if n.Contains(parsed) {
			return true
		}
	}
	return false
}

// extractIP strips the port from an address like "1.2.3.4:5678" or "[::1]:80".
func extractIP(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}
