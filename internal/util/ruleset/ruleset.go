// Package ruleset reads the rule sets templates reference with RULE-SET: classical
// Clash rules without a target, as plain lines or an upstream payload list.
package ruleset

import (
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
)

// MaxRules bounds a set: every subscription that references it carries all of it.
const MaxRules = 5000

// Rule is one classical rule without its target. Provider and Tier come from the
// "# Provider / Tier" heading above it, the way upstream lists mark their blocks.
type Rule struct {
	Type     string
	Value    string
	Options  []string
	Provider string
	Tier     string
	Line     int
}

// Key identifies the rule whatever its heading, options or line.
func (r Rule) Key() string { return r.Type + "," + r.Value }

// String is the rule as a set holds it.
func (r Rule) String() string {
	return strings.Join(append([]string{r.Type, r.Value}, r.Options...), ",")
}

// ipRule tells the rule types a set may hold, and which of them match an address.
var ipRule = map[string]bool{
	"DOMAIN": false, "DOMAIN-SUFFIX": false, "DOMAIN-KEYWORD": false, "DOMAIN-REGEX": false,
	"PROCESS-NAME": false, "IP-CIDR": true, "IP-CIDR6": true,
}

var heading = regexp.MustCompile(`^#\s*(.+?)\s+/\s+([A-Za-z][A-Za-z-]*)\s*$`)

// Parse reads every rule of a set, keeping types it does not know: Check says
// whether a subscription can use them.
func Parse(text string) []Rule {
	var rules []Rule
	provider, tier := "", ""
	for i, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || line == "payload:" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if m := heading.FindStringSubmatch(line); m != nil {
				provider, tier = m[1], m[2]
			}
			continue
		}
		line = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "- ")), `'"`)
		parts := strings.Split(line, ",")
		for j := range parts {
			parts[j] = strings.TrimSpace(parts[j])
		}
		r := Rule{Type: strings.ToUpper(parts[0]), Provider: provider, Tier: tier, Line: i + 1}
		if len(parts) > 1 {
			r.Value = parts[1]
		}
		if len(parts) > 2 {
			r.Options = parts[2:]
		}
		rules = append(rules, r)
	}
	return rules
}

// Check refuses a set a client could not use, naming the line: an unknown or ASN
// rule, a value that does not parse, an option other than no-resolve on an IP rule.
func Check(text string) error {
	rules := Parse(text)
	if len(rules) > MaxRules {
		return fmt.Errorf("%d rules, more than the %d a set may hold", len(rules), MaxRules)
	}
	seen := make(map[string]int, len(rules))
	for _, r := range rules {
		if err := checkRule(r); err != nil {
			return fmt.Errorf("line %d: %w", r.Line, err)
		}
		if first, repeated := seen[r.Key()]; repeated {
			return fmt.Errorf("line %d: %s repeats line %d", r.Line, r.Key(), first)
		}
		seen[r.Key()] = r.Line
	}
	return nil
}

func checkRule(r Rule) error {
	ip, known := ipRule[r.Type]
	switch {
	case r.Type == "IP-ASN":
		return errors.New("IP-ASN rules need an ASN database clients cannot always download; list the IP ranges instead")
	case !known:
		return fmt.Errorf("unknown rule type %q", r.Type)
	case r.Value == "":
		return fmt.Errorf("%s has no value", r.Type)
	case strings.ContainsAny(r.Value, " \t"):
		return fmt.Errorf("%q has a space", r.Value)
	}
	for _, option := range r.Options {
		if !ip || !strings.EqualFold(option, "no-resolve") {
			return fmt.Errorf("%s does not take the option %q", r.Type, option)
		}
	}
	switch r.Type {
	case "IP-CIDR", "IP-CIDR6":
		v4 := r.Type == "IP-CIDR"
		if prefix, err := netip.ParsePrefix(r.Value); err != nil || prefix.Addr().Is4() != v4 {
			family := "IPv6"
			if v4 {
				family = "IPv4"
			}
			return fmt.Errorf("%q is not an %s range", r.Value, family)
		}
	case "DOMAIN-REGEX":
		if _, err := regexp.Compile(r.Value); err != nil {
			return fmt.Errorf("%q is not a regular expression: %w", r.Value, err)
		}
	}
	return nil
}

// Expand writes the rules as Clash rules sending traffic to target. noResolve gives
// IP rules the flag, as the RULE-SET line's own no-resolve would.
func Expand(rules []Rule, target string, noResolve bool) []string {
	out := make([]string, 0, len(rules))
	for _, r := range rules {
		parts := append([]string{r.Type, r.Value, target}, r.Options...)
		if noResolve && ipRule[r.Type] && !hasNoResolve(r.Options) {
			parts = append(parts, "no-resolve")
		}
		out = append(out, strings.Join(parts, ","))
	}
	return out
}

func hasNoResolve(options []string) bool {
	for _, option := range options {
		if strings.EqualFold(option, "no-resolve") {
			return true
		}
	}
	return false
}
