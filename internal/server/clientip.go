package server

import (
	"fmt"
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
		cidr = strings.TrimSpace(cidr)
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			// Not a CIDR — accept bare IP, wrap as /32 or /128
			ip := net.ParseIP(cidr)
			if ip == nil {
				return nil, fmt.Errorf("invalid IP or CIDR: %q", cidr)
			}
			if ip.To4() != nil {
				ipNet = &net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)}
			} else {
				ipNet = &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}
			}
		}
		nets = append(nets, ipNet)
	}
	return &ClientIPResolver{trusted: nets}, nil
}

// ClientIP returns the best-effort real client IP for the request.
//
// If RemoteAddr is from a trusted proxy CIDR, the X-Forwarded-For chain is
// walked from the right and the first untrusted entry is returned. All
// X-Forwarded-For header lines are considered together, because some proxies
// append a new header line instead of extending the existing one; reading
// only the first line would return a value the client supplied. X-Real-IP is
// consulted only when no X-Forwarded-For header is present. Values that are
// not valid IP addresses are ignored. Otherwise RemoteAddr is returned.
func (c *ClientIPResolver) ClientIP(r *http.Request) string {
	remoteIP := extractIP(r.RemoteAddr)
	if !c.isTrusted(remoteIP) {
		return remoteIP
	}

	if xffLines := r.Header.Values("X-Forwarded-For"); len(xffLines) > 0 {
		ips := strings.Split(strings.Join(xffLines, ","), ",")
		leftmost := ""
		for i := len(ips) - 1; i >= 0; i-- {
			ip := strings.TrimSpace(ips[i])
			if net.ParseIP(ip) == nil {
				// An unparsable hop means the chain cannot be trusted
				// beyond this point; stop rather than skip over it.
				break
			}
			if !c.isTrusted(ip) {
				return ip
			}
			leftmost = ip
		}
		// Every parseable entry is a trusted proxy: the left-most one
		// reached is the closest thing to the originating client.
		if leftmost != "" {
			return leftmost
		}
		return remoteIP
	}

	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); net.ParseIP(realIP) != nil {
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
