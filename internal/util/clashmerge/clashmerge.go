// Package clashmerge applies a rule template variant to its base as Clash Verge applies
// a Merge profile, finds such a variant, and writes Clash YAML that reads back the same.
package clashmerge

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// The keys that add to the base's lists instead of replacing a key.
const (
	PrependRules       = "prepend-rules"
	AppendRules        = "append-rules"
	PrependProxyGroups = "prepend-proxy-groups"
	AppendProxyGroups  = "append-proxy-groups"
)

// listKeys maps each adding key to the list it adds to and whether it goes first.
var listKeys = map[string]struct {
	list    string
	prepend bool
}{
	PrependRules:       {"rules", true},
	AppendRules:        {"rules", false},
	PrependProxyGroups: {"proxy-groups", true},
	AppendProxyGroups:  {"proxy-groups", false},
}

// Change is one key of the base a variant changes: replaced outright, added to, or both.
type Change struct {
	Key      string
	Replaced bool
	Added    int
}

// DropsKeyError is a template without a key its base sets: a variant cannot remove one.
type DropsKeyError struct{ Key string }

func (e *DropsKeyError) Error() string {
	return fmt.Sprintf("the template has no %q, which the base sets, and a variant cannot remove it", e.Key)
}

// RulesInsideError is a template that adds rules between the base's own: a variant
// can only add them before or after.
type RulesInsideError struct{ Count int }

func (e *RulesInsideError) Error() string {
	return fmt.Sprintf("%d of the template's rules sit between the base's own; a variant can only put them first", e.Count)
}

// Apply returns base with the variant applied: a key replaces the base's, the prepend
// and append keys add to its lists, a null changes nothing. Neither map is changed.
func Apply(base, variant map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(variant))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range variant {
		if _, adds := listKeys[key]; adds || value == nil {
			continue
		}
		out[key] = value
	}
	for _, list := range []string{"rules", "proxy-groups"} {
		before, after := added(variant, list, true), added(variant, list, false)
		if len(before) == 0 && len(after) == 0 {
			continue
		}
		current, _ := out[list].([]any)
		merged := make([]any, 0, len(before)+len(current)+len(after))
		merged = append(merged, before...)
		merged = append(merged, current...)
		out[list] = append(merged, after...)
	}
	return out
}

// added lists the entries the variant adds before or after one of the base's lists.
func added(variant map[string]any, list string, prepend bool) []any {
	for key, target := range listKeys {
		if target.list == list && target.prepend == prepend {
			items, _ := variant[key].([]any)
			return items
		}
	}
	return nil
}

// Diff finds the variant that turns base into full. Rules added between the base's own
// move to the front when reorder is set, and are counted; otherwise RulesInsideError.
func Diff(base, full map[string]any, reorder bool) (map[string]any, int, error) {
	for key, value := range base {
		if value != nil && full[key] == nil {
			return nil, 0, &DropsKeyError{Key: key}
		}
	}
	variant := map[string]any{}
	moved := 0
	for key, value := range full {
		if value == nil || reflect.DeepEqual(base[key], value) {
			continue
		}
		var err error
		switch key {
		case "rules":
			inside := refuseInside
			if reorder {
				inside = moveInside
			}
			moved, err = diffList(variant, key, base[key], value, PrependRules, AppendRules, inside)
		case "proxy-groups":
			_, err = diffList(variant, key, base[key], value, PrependProxyGroups, AppendProxyGroups, replaceInside)
		default:
			variant[key] = value
		}
		if err != nil {
			return nil, 0, err
		}
	}
	return variant, moved, nil
}

// insidePolicy is what diffList does with entries added between the base's own.
type insidePolicy int

const (
	replaceInside insidePolicy = iota
	refuseInside
	moveInside
)

// diffList puts into the variant how full's list grows from base's: entries added
// before and after it, or the whole list when base's is not kept in order.
func diffList(variant map[string]any, key string, baseValue, fullValue any, prependKey, appendKey string, inside insidePolicy) (int, error) {
	baseList, _ := baseValue.([]any)
	fullList, ok := fullValue.([]any)
	if !ok || len(baseList) == 0 {
		variant[key] = fullValue
		return 0, nil
	}
	for start := 0; start+len(baseList) <= len(fullList); start++ {
		if reflect.DeepEqual(fullList[start], baseList[0]) && reflect.DeepEqual(fullList[start:start+len(baseList)], baseList) {
			setIfAny(variant, prependKey, fullList[:start])
			setIfAny(variant, appendKey, fullList[start+len(baseList):])
			return 0, nil
		}
	}
	before, between, after, kept := extras(baseList, fullList)
	if !kept || inside == replaceInside {
		variant[key] = fullValue
		return 0, nil
	}
	if inside == refuseInside {
		return 0, &RulesInsideError{Count: len(between)}
	}
	setIfAny(variant, prependKey, append(slices.Clone(before), between...))
	setIfAny(variant, appendKey, after)
	return len(between), nil
}

// extras splits what full adds to base, when full keeps all of base in order, into
// the entries before base's first, between its own, and after its last.
func extras(baseList, fullList []any) (before, inside, after []any, kept bool) {
	matched := 0
	for _, item := range fullList {
		switch {
		case matched < len(baseList) && reflect.DeepEqual(item, baseList[matched]):
			matched++
		case matched == 0:
			before = append(before, item)
		case matched == len(baseList):
			after = append(after, item)
		default:
			inside = append(inside, item)
		}
	}
	return before, inside, after, matched == len(baseList)
}

func setIfAny(variant map[string]any, key string, items []any) {
	if len(items) > 0 {
		variant[key] = slices.Clone(items)
	}
}

// Changes names the base keys a variant changes, in key order.
func Changes(variant map[string]any) []Change {
	byKey := map[string]*Change{}
	at := func(key string) *Change {
		if byKey[key] == nil {
			byKey[key] = &Change{Key: key}
		}
		return byKey[key]
	}
	for key, value := range variant {
		if value == nil {
			continue
		}
		if target, adds := listKeys[key]; adds {
			if items, _ := value.([]any); len(items) > 0 {
				at(target.list).Added += len(items)
			}
			continue
		}
		at(key).Replaced = true
	}
	out := make([]Change, 0, len(byKey))
	for _, change := range byKey {
		out = append(out, *change)
	}
	slices.SortFunc(out, func(a, b Change) int { return strings.Compare(a.Key, b.Key) })
	return out
}
