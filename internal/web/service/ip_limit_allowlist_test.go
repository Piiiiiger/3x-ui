package service

import (
	"net/netip"
	"testing"
)

func allowlisted(l ipLimitAllowlist, ip string) bool {
	addr, _ := netip.ParseAddr(ip)
	return l.contains(addr)
}

// Addresses in the examples below come from the documentation ranges reserved
// by RFC 5737 and RFC 3849.
func TestIpLimitAllowlistMatchesAddressesAndNetworks(t *testing.T) {
	list := parseIpLimitAllowlist("203.0.113.10, 198.51.100.0/24 , 2001:db8::/32, not-an-ip")

	for _, ip := range []string{"203.0.113.10", "198.51.100.7", "2001:db8::1"} {
		if !allowlisted(list, ip) {
			t.Fatalf("%s should be allowlisted", ip)
		}
	}
	for _, ip := range []string{"203.0.113.11", "192.0.2.5", "2001:db9::1", ""} {
		if allowlisted(list, ip) {
			t.Fatalf("%s must not be allowlisted", ip)
		}
	}
}

// A typo must not disable the limit for everybody, so an unparsable entry is
// dropped and the rest of the list keeps working.
func TestIpLimitAllowlistIgnoresUnparsableEntries(t *testing.T) {
	list := parseIpLimitAllowlist("nonsense, 203.0.113.0/24")
	if !allowlisted(list, "203.0.113.5") {
		t.Fatal("a valid entry stopped working because a neighbouring one was malformed")
	}
	if allowlisted(list, "192.0.2.1") {
		t.Fatal("a malformed entry must not widen the allowlist")
	}
	if parseIpLimitAllowlist("nonsense").empty() != true {
		t.Fatal("a list of only malformed entries must be empty, not permissive")
	}
}
