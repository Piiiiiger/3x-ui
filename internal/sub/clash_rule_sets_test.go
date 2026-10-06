package sub

import (
	"testing"

	yaml "github.com/goccy/go-yaml"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

const aiRuleSet = "# Example AI / Core\nDOMAIN-SUFFIX,example-ai.com\n# Example AI / Network\nIP-CIDR6,2001:db8::/32\n"

// setRuleSet gives the seeded rule set its reviewed rules.
func setRuleSet(t *testing.T, name, rules string) {
	t.Helper()
	res := database.GetDB().Model(&model.RuleSet{}).Where("name = ?", name).Update("rules", rules)
	if res.Error != nil || res.RowsAffected != 1 {
		t.Fatalf("set rule set %s: %v (%d rows)", name, res.Error, res.RowsAffected)
	}
}

func clashRules(t *testing.T, out string) []string {
	t.Helper()
	var got struct {
		Rules []string `yaml:"rules"`
	}
	if err := yaml.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse %s: %v", out, err)
	}
	return got.Rules
}

// A template names a rule set where its rules belong; each goes to that line's
// target, and the line's no-resolve reaches the IP rules.
func TestClashWritesARuleSetWhereTheTemplateNamesIt(t *testing.T) {
	seedPlanSub(t, model.VLESS, vlessPlanSettings, tcpStream)
	setRuleSet(t, "ai", aiRuleSet)
	setRuleSet(t, "ai-cn", "DOMAIN-SUFFIX,example-ai.cn\n")
	putS1OnPlan(t, seedRuleTemplate(t, "plan",
		"DOMAIN-SUFFIX,first.example,DIRECT\nRULE-SET,ai-cn,DIRECT\nRULE-SET,ai,PROXY,no-resolve\nMATCH,PROXY", false))

	assertStrings(t, "rules", clashRules(t, clashFor(t)), []string{
		"DOMAIN-SUFFIX,first.example,DIRECT",
		"DOMAIN-SUFFIX,example-ai.cn,DIRECT",
		"DOMAIN-SUFFIX,example-ai.com,PROXY",
		"IP-CIDR6,2001:db8::/32,PROXY,no-resolve",
		"MATCH,PROXY",
	})
}

// A RULE-SET line naming one of the template's own rule-providers, or no set at
// all, is the client's to resolve and stays as written.
func TestClashLeavesOtherRuleSetLinesAsWritten(t *testing.T) {
	seedPlanSub(t, model.VLESS, vlessPlanSettings, tcpStream)
	setRuleSet(t, "ai", aiRuleSet)
	putS1OnPlan(t, seedRuleTemplate(t, "plan", `rule-providers:
  ai:
    type: http
    behavior: classical
    url: https://rules.example/ai.yaml
    path: ./ai.yaml
rules:
  - RULE-SET,ai,PROXY
  - RULE-SET,elsewhere,DIRECT
  - MATCH,PROXY
`, false))

	assertStrings(t, "rules", clashRules(t, clashFor(t)), []string{"RULE-SET,ai,PROXY", "RULE-SET,elsewhere,DIRECT", "MATCH,PROXY"})
}

// A person's private rules are checked against the whole config: the template's
// rule sets are written out before that, or the check would refuse them.
func TestClashWritesRuleSetsBeforePrivateRules(t *testing.T) {
	seedPlanSub(t, model.VLESS, vlessPlanSettings, tcpStream)
	setRuleSet(t, "ai", aiRuleSet)
	putS1OnPlan(t, seedRuleTemplate(t, "plan", "RULE-SET,ai,PROXY\nMATCH,PROXY", false))
	var client model.ClientRecord
	if err := database.GetDB().Where("sub_id = ?", "s1").First(&client).Error; err != nil {
		t.Fatal(err)
	}
	private := &model.ClientSubscriptionCustomization{ClientId: client.Id, Rules: "rules:\n  - DOMAIN-SUFFIX,mine.example,DIRECT\n"}
	if err := database.GetDB().Create(private).Error; err != nil {
		t.Fatal(err)
	}

	assertStrings(t, "rules", clashRules(t, clashFor(t)), []string{
		"DOMAIN-SUFFIX,mine.example,DIRECT",
		"DOMAIN-SUFFIX,example-ai.com,PROXY",
		"IP-CIDR6,2001:db8::/32,PROXY",
		"MATCH,PROXY",
	})
}

// A preview tries proposed rules for a set on a plan before a save serves them.
func TestPreviewClashUsesProposedRuleSets(t *testing.T) {
	seedPlanSub(t, model.VLESS, vlessPlanSettings, tcpStream)
	setRuleSet(t, "ai", aiRuleSet)
	out, err := PreviewClash("s1", "req.example.com", "", RuleTemplateSource{Content: "RULE-SET,ai,PROXY\nMATCH,PROXY"},
		map[string]string{"ai": "DOMAIN-SUFFIX,proposed.example\n"})
	if err != nil {
		t.Fatalf("PreviewClash: %v", err)
	}
	assertStrings(t, "rules", clashRules(t, out), []string{"DOMAIN-SUFFIX,proposed.example,PROXY", "MATCH,PROXY"})
}

// RULE-SET,<set>:<provider> takes only the rules under that provider's headings, so
// a template can send one service elsewhere and keep doing so as the set grows.
func TestClashWritesOneProviderOfARuleSet(t *testing.T) {
	seedPlanSub(t, model.VLESS, vlessPlanSettings, tcpStream)
	setRuleSet(t, "ai", "# Example AI / Core\nDOMAIN-SUFFIX,example-ai.com\n# Other AI / Core\nDOMAIN-SUFFIX,other-ai.com\n# Other AI / Third-Party\nDOMAIN,cdn.other-ai.net\n")
	putS1OnPlan(t, seedRuleTemplate(t, "plan", "RULE-SET,ai:Other AI,DIRECT\nRULE-SET,ai,PROXY\nMATCH,PROXY", false))

	assertStrings(t, "rules", clashRules(t, clashFor(t)), []string{
		"DOMAIN-SUFFIX,other-ai.com,DIRECT",
		"DOMAIN,cdn.other-ai.net,DIRECT",
		"DOMAIN-SUFFIX,example-ai.com,PROXY",
		"DOMAIN-SUFFIX,other-ai.com,PROXY",
		"DOMAIN,cdn.other-ai.net,PROXY",
		"MATCH,PROXY",
	})
}
