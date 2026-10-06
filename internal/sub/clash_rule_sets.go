package sub

import (
	"slices"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/ruleset"
)

// expandRuleSets writes each RULE-SET line naming one of the panel's rule sets as
// that set's rules, sent to the line's target; clients fetch nothing extra.
func (s *SubClashService) expandRuleSets(config map[string]any) error {
	rules, _ := asAnySlice(config["rules"])
	providers, _ := config["rule-providers"].(map[string]any)
	var names []string
	for _, value := range rules {
		if ref, ok := ruleSetReference(value, providers); ok {
			names = append(names, ref.name)
		}
	}
	if len(names) == 0 {
		return nil
	}
	sets, err := s.ruleSetsNamed(names)
	if err != nil {
		return err
	}
	out := make([]any, 0, len(rules))
	for _, value := range rules {
		ref, ok := ruleSetReference(value, providers)
		text, known := sets[ref.name]
		if !ok || !known {
			out = append(out, value)
			continue
		}
		rules := ruleset.Parse(text)
		if ref.provider != "" {
			rules = slices.DeleteFunc(rules, func(r ruleset.Rule) bool { return r.Provider != ref.provider })
		}
		for _, rule := range ruleset.Expand(rules, ref.target, ref.noResolve) {
			out = append(out, rule)
		}
	}
	config["rules"] = out
	return nil
}

// ruleSetRef is a RULE-SET line naming a set, or with set:provider only the rules
// under that provider's headings.
type ruleSetRef struct {
	name, provider, target string
	noResolve              bool
}

// ruleSetReference reads a RULE-SET line that names none of the config's own
// rule-providers: those stay the client's to fetch.
func ruleSetReference(value any, providers map[string]any) (ruleSetRef, bool) {
	line, _ := value.(string)
	parts := strings.Split(line, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	if len(parts) < 3 || !strings.EqualFold(parts[0], "RULE-SET") {
		return ruleSetRef{}, false
	}
	if _, declared := providers[parts[1]]; declared {
		return ruleSetRef{}, false
	}
	name, provider, _ := strings.Cut(parts[1], ":")
	ref := ruleSetRef{name: name, provider: provider, target: parts[2]}
	for _, option := range parts[3:] {
		if strings.EqualFold(option, "no-resolve") {
			ref.noResolve = true
		}
	}
	return ref, true
}

// ruleSetsNamed reads the saved rules of the named sets; a preview's proposed rules
// stand in for them.
func (s *SubClashService) ruleSetsNamed(names []string) (map[string]string, error) {
	var rows []model.RuleSet
	if err := database.GetDB().Select("name", "rules").Where("name IN ?", names).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows)+len(s.ruleSetOverride))
	for _, row := range rows {
		out[row.Name] = row.Rules
	}
	for name, rules := range s.ruleSetOverride {
		out[name] = rules
	}
	return out, nil
}
