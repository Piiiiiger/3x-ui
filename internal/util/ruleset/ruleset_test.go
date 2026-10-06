package ruleset

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

const upstreamShape = `# AI Proxy Rules - Global AI Providers
# https://github.com/example/ai-proxy-rules

payload:
  # Example AI / Core
  - DOMAIN-SUFFIX,example-ai.com
  # see https://example-ai.com/docs/network
  - PROCESS-NAME,example

  # Example AI / Network
  - IP-CIDR,192.0.2.0/24,no-resolve
`

// Upstream lists mark blocks with "# Provider / Tier" headings and wrap the rules
// in a payload list; a comment holding a URL is no heading.
func TestParseReadsAPayloadUnderItsHeadings(t *testing.T) {
	want := []Rule{
		{Type: "DOMAIN-SUFFIX", Value: "example-ai.com", Provider: "Example AI", Tier: "Core", Line: 6},
		{Type: "PROCESS-NAME", Value: "example", Provider: "Example AI", Tier: "Core", Line: 8},
		{Type: "IP-CIDR", Value: "192.0.2.0/24", Options: []string{"no-resolve"}, Provider: "Example AI", Tier: "Network", Line: 11},
	}
	got := Parse(upstreamShape)
	if !slices.EqualFunc(got, want, sameRule) {
		t.Fatalf("Parse =\n%+v\nwant\n%+v", got, want)
	}
}

// A set saved as plain lines reads the same as the payload it came from.
func TestParseReadsPlainLines(t *testing.T) {
	plain := "# Example AI / Core\nDOMAIN-SUFFIX,example-ai.com\n\n'PROCESS-NAME,example'\r\n"
	want := []Rule{
		{Type: "DOMAIN-SUFFIX", Value: "example-ai.com", Provider: "Example AI", Tier: "Core", Line: 2},
		{Type: "PROCESS-NAME", Value: "example", Provider: "Example AI", Tier: "Core", Line: 4},
	}
	if got := Parse(plain); !slices.EqualFunc(got, want, sameRule) {
		t.Fatalf("Parse =\n%+v\nwant\n%+v", got, want)
	}
}

func sameRule(a, b Rule) bool {
	return a.Type == b.Type && a.Value == b.Value && slices.Equal(a.Options, b.Options) &&
		a.Provider == b.Provider && a.Tier == b.Tier && a.Line == b.Line
}

// What every subscription would carry is refused when a client could not use it,
// naming the line.
func TestCheckRefusesWhatClientsCannotUse(t *testing.T) {
	cases := []struct {
		name, text, want string
	}{
		{"an unknown type", "GEOSITE,openai", `line 1: unknown rule type "GEOSITE"`},
		{"an ASN rule", "IP-ASN,64496,no-resolve", "line 1: IP-ASN rules need an ASN database"},
		{"no value", "DOMAIN-SUFFIX,", "line 1: DOMAIN-SUFFIX has no value"},
		{"an IPv6 range as IP-CIDR", "IP-CIDR,2001:db8::/32", `line 1: "2001:db8::/32" is not an IPv4 range`},
		{"an IPv4 range as IP-CIDR6", "IP-CIDR6,192.0.2.0/24", `line 1: "192.0.2.0/24" is not an IPv6 range`},
		{"no-resolve on a domain rule", "DOMAIN,example.com,no-resolve", `line 1: DOMAIN does not take the option "no-resolve"`},
		{"another option", "IP-CIDR,192.0.2.0/24,src", `line 1: IP-CIDR does not take the option "src"`},
		{"a broken regex", "DOMAIN-REGEX,^api(\\.example\\.com$", "line 1: \"^api(\\\\.example\\\\.com$\" is not a regular expression"},
		{"a space in the value", "PROCESS-NAME,Example Helper", `line 1: "Example Helper" has a space`},
		{"a repeat", "DOMAIN,a.example\n# x / Core\nDOMAIN,a.example", "line 3: DOMAIN,a.example repeats line 1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Check(c.text)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("Check(%q) = %v, want %q", c.text, err, c.want)
			}
		})
	}
}

// Every subscription carries the whole set, so its size is bounded.
func TestCheckRefusesMoreRulesThanASetMayHold(t *testing.T) {
	var b strings.Builder
	for i := range MaxRules + 1 {
		fmt.Fprintf(&b, "DOMAIN,host%d.example\n", i)
	}
	if err := Check(b.String()); err == nil || !strings.Contains(err.Error(), "more than the 5000") {
		t.Fatalf("Check of %d rules = %v", MaxRules+1, err)
	}
}

// The shape upstream publishes passes once its ASN rules are gone.
func TestCheckAcceptsTheUpstreamShape(t *testing.T) {
	text := upstreamShape + "  - IP-CIDR6,2001:db8::/32,no-resolve\n  - DOMAIN-KEYWORD,example-ai\n  - DOMAIN-REGEX,^api-\\d+\\.example\\.com$\n"
	if err := Check(text); err != nil {
		t.Fatalf("Check = %v", err)
	}
}

// Expanded rules send traffic to the RULE-SET line's target; that line's no-resolve
// reaches IP rules only, and a rule that has the flag keeps one.
func TestExpandSendsEachRuleToTheTarget(t *testing.T) {
	rules := Parse("DOMAIN-SUFFIX,example-ai.com\nIP-CIDR,192.0.2.0/24,no-resolve\nIP-CIDR6,2001:db8::/32\n")
	cases := []struct {
		noResolve bool
		want      []string
	}{
		{false, []string{"DOMAIN-SUFFIX,example-ai.com,AI", "IP-CIDR,192.0.2.0/24,AI,no-resolve", "IP-CIDR6,2001:db8::/32,AI"}},
		{true, []string{"DOMAIN-SUFFIX,example-ai.com,AI", "IP-CIDR,192.0.2.0/24,AI,no-resolve", "IP-CIDR6,2001:db8::/32,AI,no-resolve"}},
	}
	for _, c := range cases {
		if got := Expand(rules, "AI", c.noResolve); !slices.Equal(got, c.want) {
			t.Errorf("Expand(noResolve=%v) =\n%q\nwant\n%q", c.noResolve, got, c.want)
		}
	}
}
