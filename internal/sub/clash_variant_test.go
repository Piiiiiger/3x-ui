package sub

import (
	"testing"

	yaml "github.com/goccy/go-yaml"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// seedVariant saves a template that changes baseId's in the way content says.
func seedVariant(t *testing.T, name string, baseId int, content string) int {
	t.Helper()
	tpl := &model.RuleTemplate{Name: name, Content: content, BaseId: baseId}
	if err := database.GetDB().Create(tpl).Error; err != nil {
		t.Fatalf("create variant %s: %v", name, err)
	}
	return tpl.Id
}

const mineVariant = `mode: global
listeners:
  - name: mixed-in
    type: mixed
    port: 7891
prepend-rules:
  - DOMAIN-SUFFIX,mine.example,🔰 节点选择
`

// mmwxStyleTemplate with mineVariant applied, written out in full.
const mineFullTemplate = `port: 7890
mode: global
listeners:
  - name: mixed-in
    type: mixed
    port: 7891
proxies: null
proxy-groups:
  - name: 🔰 节点选择
    type: select
    proxies: [__PROXY_NODES__, 🎯 全球直连, 🇸🇬 新加坡（自动）-Edge]
  - name: 🇸🇬 新加坡（自动）-Edge
    type: url-test
    url: http://www.gstatic.com/generate_204
    interval: 300
    filter: 新加坡
  - name: 🎯 全球直连
    type: select
    proxies: [DIRECT, __PROXY_NODES__]
rules:
  - DOMAIN-SUFFIX,mine.example,🔰 节点选择
  - DOMAIN-SUFFIX,cn,🎯 全球直连
  - MATCH,🔰 节点选择
`

func TestClashMergesAVariantOntoItsBase(t *testing.T) {
	seedPlanSub(t, model.VLESS, vlessPlanSettings, tcpStream)
	base := seedRuleTemplate(t, "base", mmwxStyleTemplate, false)
	putS1OnPlan(t, seedVariant(t, "variant", base, mineVariant))

	var got struct {
		Mode   string `yaml:"mode"`
		Groups []struct {
			Name    string   `yaml:"name"`
			Proxies []string `yaml:"proxies"`
		} `yaml:"proxy-groups"`
		Rules []string `yaml:"rules"`
	}
	out := clashFor(t)
	if err := yaml.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse %s: %v", out, err)
	}
	if got.Mode != "global" {
		t.Fatalf("mode = %q, want the variant's global", got.Mode)
	}
	assertStrings(t, "rules", got.Rules,
		[]string{"DOMAIN-SUFFIX,mine.example,🔰 节点选择", "DOMAIN-SUFFIX,cn,🎯 全球直连", "MATCH,🔰 节点选择"})
	if len(got.Groups) != 3 || got.Groups[0].Name != "🔰 节点选择" {
		t.Fatalf("groups = %+v, want the base's three", got.Groups)
	}
	// The base's group is filled with the subscription's node in place of __PROXY_NODES__.
	members := got.Groups[0].Proxies
	if len(members) != 3 || members[0] == clashProxyNodesPlaceholder {
		t.Fatalf("base group members = %v, want the node then the two groups", members)
	}
	assertStrings(t, "base group", members[1:], []string{"🎯 全球直连", "🇸🇬 新加坡（自动）-Edge"})
}

// What makes moving a plan to a variant safe: a variant that merges to a full
// template renders exactly the bytes that template did.
func TestClashRendersAVariantAsTheFullTemplateItStandsFor(t *testing.T) {
	seedPlanSub(t, model.VLESS, vlessPlanSettings, tcpStream)
	full := seedRuleTemplate(t, "full", mineFullTemplate, false)
	putS1OnPlan(t, full)
	want := clashFor(t)

	variant := seedVariant(t, "variant", seedRuleTemplate(t, "base", mmwxStyleTemplate, false), mineVariant)
	if err := database.GetDB().Model(&model.Plan{}).Where("template_id = ?", full).
		Update("template_id", variant).Error; err != nil {
		t.Fatalf("move plan: %v", err)
	}
	if got := clashFor(t); got != want {
		t.Fatalf("the variant renders differently from the full template:\n--- variant\n%s\n--- full\n%s", got, want)
	}
}

// Plans without a template, and people without a plan, get the default one, which
// may itself be a variant.
func TestClashMergesTheDefaultTemplateWhenItIsAVariant(t *testing.T) {
	seedPlanSub(t, model.VLESS, vlessPlanSettings, tcpStream)
	putS1OnPlan(t, seedRuleTemplate(t, "full", mineFullTemplate, false))
	want := clashFor(t)

	variant := seedVariant(t, "variant", seedRuleTemplate(t, "base", mmwxStyleTemplate, false), mineVariant)
	db := database.GetDB()
	if err := db.Model(&model.RuleTemplate{}).Where("id = ?", variant).Update("is_default", true).Error; err != nil {
		t.Fatalf("make default: %v", err)
	}
	if err := db.Model(&model.Plan{}).Where("1 = 1").Update("template_id", 0).Error; err != nil {
		t.Fatalf("clear plan template: %v", err)
	}
	if got := clashFor(t); got != want {
		t.Fatalf("the default variant renders differently from its full template:\n%s", got)
	}
}

// The rules page previews a variant being edited, before it is saved.
func TestPreviewMergesAnUnsavedVariantOntoItsBase(t *testing.T) {
	seedPlanSub(t, model.VLESS, vlessPlanSettings, tcpStream)
	putS1OnPlan(t, seedRuleTemplate(t, "full", mineFullTemplate, false))
	want := clashFor(t)

	got, err := PreviewClash("s1", "req.example.com", "", RuleTemplateSource{
		Content: mineVariant, Base: mmwxStyleTemplate, Variant: true,
	}, nil)
	if err != nil {
		t.Fatalf("PreviewClash: %v", err)
	}
	if got != want {
		t.Fatalf("preview of the variant differs from its full template:\n--- preview\n%s\n--- full\n%s", got, want)
	}
}
