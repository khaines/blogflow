package server

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
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
// walked from the right and the first untrusted hop is returned. All
// X-Forwarded-For header lines are joined first, because some proxies add a
// new header line instead of extending the existing one; reading only the
// first line would return a value the client supplied.
//
// Each hop is normalised before use: surrounding whitespace, a trailing
// ":port", IPv6 brackets and an IPv6 zone are removed. Empty hops are
// skipped. A non-empty hop that is still not an IP address (e.g. "unknown")
// ends the walk, because nothing to its left can be attributed to a trusted
// proxy. If every hop reached is trusted, the left-most one is returned.
//
// X-Real-IP is consulted only when no X-Forwarded-For header is present, and
// only if it is a valid IP. Otherwise RemoteAddr is returned.
func (c *ClientIPResolver) ClientIP(r *http.Request) string {
	remoteIP := extractIP(r.RemoteAddr)
	if !c.isTrusted(remoteIP) {
		return remoteIP
	}

	if xffLines := r.Header.Values("X-Forwarded-For"); len(xffLines) > 0 {
		hops := strings.Split(strings.Join(xffLines, ","), ",")
		leftmost := ""
		for i := len(hops) - 1; i >= 0; i-- {
			raw := strings.TrimSpace(hops[i])
			if raw == "" {
				continue
			}
			ip, ok := normalizeHop(raw)
			if !ok {
				break
			}
			if !c.isTrusted(ip) {
				return ip
			}
			leftmost = ip
		}
		if leftmost != "" {
			return leftmost
		}
		return remoteIP
	}

	if ip, ok := normalizeHop(strings.TrimSpace(r.Header.Get("X-Real-IP"))); ok {
		return ip
	}

	return remoteIP
}

// normalizeHop parses a forwarded-for hop that may carry a port, IPv6
// brackets or an IPv6 zone, and returns the bare canonical IP.
func normalizeHop(hop string) (string, bool) {
	if hop == "" {
		return "", false
	}
	if addr, err := netip.ParseAddr(hop); err == nil {
		return addr.WithZone("").Unmap().String(), true
	}
	if ap, err := netip.ParseAddrPort(hop); err == nil {
		return ap.Addr().WithZone("").Unmap().String(), true
	}
	// "[v6]" without a port.
	if len(hop) > 2 && hop[0] == '[' && hop[len(hop)-1] == ']' {
		if addr, err := netip.ParseAddr(hop[1 : len(hop)-1]); err == nil {
			return addr.WithZone("").Unmap().String(), true
		}
	}
	return "", false
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
