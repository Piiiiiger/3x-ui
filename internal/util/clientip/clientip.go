// Package clientip finds a visitor's address behind trusted reverse proxies.
package clientip

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
)

// FromRequest returns the visitor's IP. X-Real-IP and X-Forwarded-For count only
// when the direct peer is a trusted proxy; otherwise the peer is the visitor.
func FromRequest(remoteAddr, realIP, forwardedFor, trustedCIDRs string) string {
	remoteIP, ok := Extract(remoteAddr)
	if !ok {
		return "unknown"
	}

	if Trusted(remoteIP, trustedCIDRs) {
		if ip, ok := Extract(realIP); ok {
			return ip
		}

		if forwardedFor != "" {
			for part := range strings.SplitSeq(forwardedFor, ",") {
				if ip, ok := Extract(part); ok {
					return ip
				}
			}
		}
	}

	return remoteIP
}

// Trusted reports whether ip matches one of the comma-separated CIDRs or addresses.
func Trusted(ip, trustedCIDRs string) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}

	for value := range strings.SplitSeq(trustedCIDRs, ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if prefix, err := netip.ParsePrefix(value); err == nil {
			if prefix.Contains(addr) {
				return true
			}
			continue
		}
		if proxyIP, err := netip.ParseAddr(value); err == nil && proxyIP.Unmap() == addr.Unmap() {
			return true
		}
	}
	return false
}

// Extract parses an IP, with or without a port, out of an address or header value.
func Extract(value string) (string, bool) {
	candidate := strings.TrimSpace(value)
	if candidate == "" {
		return "", false
	}

	if ip, ok := parseIPCandidate(candidate); ok {
		return ip.String(), true
	}

	if host, _, err := net.SplitHostPort(candidate); err == nil {
		if ip, ok := parseIPCandidate(host); ok {
			return ip.String(), true
		}
	}

	if strings.Count(candidate, ":") == 1 {
		if host, _, err := net.SplitHostPort(fmt.Sprintf("[%s]", candidate)); err == nil {
			if ip, ok := parseIPCandidate(host); ok {
				return ip.String(), true
			}
		}
	}

	return "", false
}

func parseIPCandidate(value string) (netip.Addr, bool) {
	ip, err := netip.ParseAddr(strings.TrimSpace(value))
	if err != nil {
		return netip.Addr{}, false
	}
	return ip.Unmap(), true
}
