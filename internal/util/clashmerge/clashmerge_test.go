package clashmerge

import (
	"errors"
	"reflect"
	"testing"
)

func rules(items ...string) []any {
	out := make([]any, len(items))
	for i, item := range items {
		out[i] = item
	}
	return out
}

func group(name string) map[string]any {
	return map[string]any{"name": name, "type": "select", "proxies": []any{"__PROXY_NODES__"}}
}

func groups(names ...string) []any {
	out := make([]any, len(names))
	for i, name := range names {
		out[i] = group(name)
	}
	return out
}

func baseDoc() map[string]any {
	return map[string]any{
		"mode":         "rule",
		"dns":          map[string]any{"enable": true, "nameserver": []any{"223.5.5.5"}},
		"proxies":      nil,
		"proxy-groups": groups("PROXY", "HK"),
		"rules":        rules("DOMAIN-SUFFIX,a.example,DIRECT", "GEOIP,CN,DIRECT", "MATCH,PROXY"),
	}
}

func mustEqual(t *testing.T, what string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s:\n got %#v\nwant %#v", what, got, want)
	}
}

func TestApplyReplacesTheKeysAVariantSets(t *testing.T) {
	dns := map[string]any{"enable": false}
	listeners := []any{map[string]any{"name": "in", "type": "mixed", "port": 7891}}
	out := Apply(baseDoc(), map[string]any{"dns": dns, "listeners": listeners})

	mustEqual(t, "dns", out["dns"], dns)
	mustEqual(t, "listeners", out["listeners"], listeners)
	mustEqual(t, "mode", out["mode"], "rule")
	mustEqual(t, "rules", out["rules"], baseDoc()["rules"])
}

func TestApplyAddsRulesAndGroupsAroundTheBases(t *testing.T) {
	out := Apply(baseDoc(), map[string]any{
		PrependRules:       rules("DOMAIN-SUFFIX,mine.example,PROXY"),
		AppendRules:        rules("DOMAIN-SUFFIX,late.example,DIRECT"),
		PrependProxyGroups: groups("Mine"),
		AppendProxyGroups:  groups("Last"),
	})

	mustEqual(t, "rules", out["rules"], rules("DOMAIN-SUFFIX,mine.example,PROXY",
		"DOMAIN-SUFFIX,a.example,DIRECT", "GEOIP,CN,DIRECT", "MATCH,PROXY", "DOMAIN-SUFFIX,late.example,DIRECT"))
	mustEqual(t, "proxy-groups", out["proxy-groups"], groups("Mine", "PROXY", "HK", "Last"))
	for _, key := range []string{PrependRules, AppendRules, PrependProxyGroups, AppendProxyGroups} {
		if _, ok := out[key]; ok {
			t.Errorf("%s leaked into the merged document", key)
		}
	}
}

// Rules a variant puts in front go before the rules it sets, not before the base's.
func TestApplyAddsAroundTheListTheVariantSets(t *testing.T) {
	out := Apply(baseDoc(), map[string]any{
		"rules":      rules("MATCH,DIRECT"),
		PrependRules: rules("DOMAIN,first.example,PROXY"),
	})
	mustEqual(t, "rules", out["rules"], rules("DOMAIN,first.example,PROXY", "MATCH,DIRECT"))
}

// Templates ship keys left null to mean "not set"; in a variant that changes nothing.
func TestApplyKeepsTheBasesValueForANullKey(t *testing.T) {
	out := Apply(baseDoc(), map[string]any{"dns": nil, PrependRules: nil})
	mustEqual(t, "dns", out["dns"], baseDoc()["dns"])
	mustEqual(t, "rules", out["rules"], baseDoc()["rules"])
}

// One parsed base may serve several merges; spare capacity in its lists must not
// let one merge overwrite another's rules.
func TestApplyChangesNeitherInput(t *testing.T) {
	base := baseDoc()
	base["rules"] = append(make([]any, 0, 8), base["rules"].([]any)...)
	variant := map[string]any{AppendRules: rules("DOMAIN,x.example,DIRECT"), "mode": "global"}
	first := Apply(base, variant)
	Apply(base, map[string]any{AppendRules: rules("DOMAIN,y.example,DIRECT")})

	mustEqual(t, "first merge", first["rules"], rules("DOMAIN-SUFFIX,a.example,DIRECT", "GEOIP,CN,DIRECT",
		"MATCH,PROXY", "DOMAIN,x.example,DIRECT"))
	mustEqual(t, "base", base, baseDoc())
	mustEqual(t, "variant", variant, map[string]any{AppendRules: rules("DOMAIN,x.example,DIRECT"), "mode": "global"})
}

func TestDiffOfTheBaseItselfIsEmpty(t *testing.T) {
	variant, moved, err := Diff(baseDoc(), baseDoc(), false)
	if err != nil || moved != 0 || len(variant) != 0 {
		t.Fatalf("Diff(base, base) = %v, %d, %v; want an empty variant", variant, moved, err)
	}
}

func TestDiffKeepsOnlyWhatDiffers(t *testing.T) {
	full := baseDoc()
	full["dns"] = map[string]any{"enable": false}
	full["listeners"] = []any{map[string]any{"name": "in", "port": 7891}}
	full["proxy-groups"] = groups("PROXY")
	full["rules"] = rules("DOMAIN,mine.example,PROXY", "DOMAIN-SUFFIX,a.example,DIRECT", "GEOIP,CN,DIRECT",
		"MATCH,PROXY", "DOMAIN,late.example,DIRECT")

	variant, moved, err := Diff(baseDoc(), full, false)
	if err != nil || moved != 0 {
		t.Fatalf("Diff: moved %d, err %v", moved, err)
	}
	mustEqual(t, "variant", variant, map[string]any{
		"dns":          full["dns"],
		"listeners":    full["listeners"],
		"proxy-groups": groups("PROXY"),
		PrependRules:   rules("DOMAIN,mine.example,PROXY"),
		AppendRules:    rules("DOMAIN,late.example,DIRECT"),
	})
	mustEqual(t, "merged back", Apply(baseDoc(), variant), full)
}

func TestDiffAddsGroupsAroundTheBases(t *testing.T) {
	full := baseDoc()
	full["proxy-groups"] = groups("Mine", "PROXY", "HK")
	variant, _, err := Diff(baseDoc(), full, false)
	if err != nil {
		t.Fatal(err)
	}
	mustEqual(t, "variant", variant, map[string]any{PrependProxyGroups: groups("Mine")})
}

// A group's place is what the person sees in their app's list, so it is never moved:
// the variant keeps the whole list instead.
func TestDiffKeepsGroupsAddedInsideTheBasesAsTheWholeList(t *testing.T) {
	full := baseDoc()
	full["proxy-groups"] = groups("PROXY", "Mine", "HK")
	variant, moved, err := Diff(baseDoc(), full, true)
	if err != nil || moved != 0 {
		t.Fatalf("Diff: moved %d, err %v", moved, err)
	}
	mustEqual(t, "variant", variant, map[string]any{"proxy-groups": groups("PROXY", "Mine", "HK")})
}

// Rules added between the base's own cannot keep their place in a variant.
func TestDiffMovesRulesFromInsideTheBasesOnlyWhenAllowed(t *testing.T) {
	full := baseDoc()
	full["rules"] = rules("DOMAIN-SUFFIX,a.example,DIRECT", "DOMAIN,mine.example,PROXY", "GEOIP,CN,DIRECT",
		"DOMAIN,also.example,PROXY", "MATCH,PROXY")

	_, _, err := Diff(baseDoc(), full, false)
	var inside *RulesInsideError
	if !errors.As(err, &inside) || inside.Count != 2 {
		t.Fatalf("err = %v, want a RulesInsideError for 2 rules", err)
	}

	variant, moved, err := Diff(baseDoc(), full, true)
	if err != nil || moved != 2 {
		t.Fatalf("with reordering: moved %d, err %v", moved, err)
	}
	mustEqual(t, "variant", variant, map[string]any{
		PrependRules: rules("DOMAIN,mine.example,PROXY", "DOMAIN,also.example,PROXY"),
	})
}

// A variant cannot take a base rule away, so the whole list is the variant's own.
func TestDiffReplacesRulesThatDropOneOfTheBases(t *testing.T) {
	full := baseDoc()
	full["rules"] = rules("DOMAIN,mine.example,PROXY", "GEOIP,CN,DIRECT", "MATCH,PROXY")
	variant, moved, err := Diff(baseDoc(), full, true)
	if err != nil || moved != 0 {
		t.Fatalf("Diff: moved %d, err %v", moved, err)
	}
	mustEqual(t, "variant", variant, map[string]any{"rules": full["rules"]})
}

func TestDiffRefusesATemplateWithoutAKeyTheBaseSets(t *testing.T) {
	full := baseDoc()
	delete(full, "dns")
	_, _, err := Diff(baseDoc(), full, true)
	var drops *DropsKeyError
	if !errors.As(err, &drops) || drops.Key != "dns" {
		t.Fatalf("err = %v, want a DropsKeyError for dns", err)
	}
}

func TestChangesNameWhatAVariantReplacesAndAdds(t *testing.T) {
	got := Changes(map[string]any{
		"dns":              map[string]any{},
		"proxy-groups":     groups("PROXY"),
		PrependRules:       rules("a", "b", "c", "d"),
		AppendRules:        rules("e"),
		AppendProxyGroups:  groups("Last"),
		"listeners":        nil,
		PrependProxyGroups: nil,
	})
	mustEqual(t, "changes", got, []Change{
		{Key: "dns", Replaced: true},
		{Key: "proxy-groups", Replaced: true, Added: 1},
		{Key: "rules", Added: 5},
	})
}
