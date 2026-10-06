package sub

import (
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// The times below read 2026-10-06 23:58 and so on in Beijing time.
var (
	templateChanged   = time.Date(2026, 10, 6, 15, 58, 0, 0, time.UTC)
	setChanged        = time.Date(2026, 10, 7, 1, 30, 0, 0, time.UTC)
	baseChanged       = time.Date(2026, 10, 7, 2, 0, 0, 0, time.UTC)
	privateChangedOld = time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	privateChangedNew = time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
)

func changedAt(t *testing.T, row any, where string, key any, at time.Time) {
	t.Helper()
	res := database.GetDB().Model(row).Where(where, key).UpdateColumn("updated_at", at.UnixMilli())
	if res.Error != nil || res.RowsAffected != 1 {
		t.Fatalf("date %T %v: %v (%d rows)", row, key, res.Error, res.RowsAffected)
	}
}

// withoutRulesDate drops the date line live profiles start with and previews lack.
func withoutRulesDate(out string) string {
	if line, rest, ok := strings.Cut(out, "\n"); ok && strings.HasPrefix(line, "# 规则最近更新：") {
		return rest
	}
	return out
}

func wantRulesDate(t *testing.T, out, want string) {
	t.Helper()
	if line := "# 规则最近更新：" + want + "（北京时间）\n"; !strings.HasPrefix(out, line) {
		t.Fatalf("profile starts %q, want %q", strings.SplitN(out, "\n", 2)[0], line)
	}
}

// A profile opens with when its rules last changed: a rule set the template
// uses counts as soon as it is newer than the template.
func TestClashProfileSaysWhenItsRulesLastChanged(t *testing.T) {
	seedPlanSub(t, model.VLESS, vlessPlanSettings, tcpStream)
	setRuleSet(t, "ai", aiRuleSet)
	tpl := seedRuleTemplate(t, "plan", "RULE-SET,ai,PROXY\nMATCH,PROXY", false)
	putS1OnPlan(t, tpl)
	changedAt(t, &model.RuleTemplate{}, "id = ?", tpl, templateChanged)
	changedAt(t, &model.RuleSet{}, "name = ?", "ai", setChanged.Add(-48*time.Hour))
	wantRulesDate(t, clashFor(t), "2026-10-06 23:58")

	changedAt(t, &model.RuleSet{}, "name = ?", "ai", setChanged)
	wantRulesDate(t, clashFor(t), "2026-10-07 09:30")
}

// A variant's rules change when its base does.
func TestClashProfileDatesAVariantByItsBaseToo(t *testing.T) {
	seedPlanSub(t, model.VLESS, vlessPlanSettings, tcpStream)
	base := seedRuleTemplate(t, "base", mmwxStyleTemplate, false)
	variant := seedVariant(t, "variant", base, mineVariant)
	putS1OnPlan(t, variant)
	changedAt(t, &model.RuleTemplate{}, "id = ?", variant, templateChanged)
	changedAt(t, &model.RuleTemplate{}, "id = ?", base, baseChanged)
	wantRulesDate(t, clashFor(t), "2026-10-07 10:00")
}

// A person's own rules count too; once they replace the whole list, the
// template no longer shapes the rules and its changes stop counting.
func TestClashProfileDatesPrivateRules(t *testing.T) {
	seedPlanSub(t, model.VLESS, vlessPlanSettings, tcpStream)
	tpl := seedRuleTemplate(t, "plan", "DOMAIN-SUFFIX,plan.example,DIRECT\nMATCH,PROXY", false)
	putS1OnPlan(t, tpl)
	changedAt(t, &model.RuleTemplate{}, "id = ?", tpl, templateChanged)
	var client model.ClientRecord
	if err := database.GetDB().Where("sub_id = ?", "s1").First(&client).Error; err != nil {
		t.Fatal(err)
	}
	private := &model.ClientSubscriptionCustomization{ClientId: client.Id, Rules: "rules:\n  - DOMAIN-SUFFIX,mine.example,DIRECT\n"}
	if err := database.GetDB().Create(private).Error; err != nil {
		t.Fatal(err)
	}
	changedAt(t, &model.ClientSubscriptionCustomization{}, "client_id = ?", client.Id, privateChangedNew)
	wantRulesDate(t, clashFor(t), "2026-10-08 08:00")

	whole := "rules:\n  - DOMAIN-SUFFIX,mine.example,DIRECT\n  - MATCH,PROXY\n"
	if err := database.GetDB().Model(private).UpdateColumn("rules", whole).Error; err != nil {
		t.Fatal(err)
	}
	changedAt(t, &model.ClientSubscriptionCustomization{}, "client_id = ?", client.Id, privateChangedOld)
	wantRulesDate(t, clashFor(t), "2026-10-05 08:00")
}

// Without template rules there is nothing to date, and a preview's rules were
// never saved, so neither starts with the line.
func TestClashProfileWithoutSavedRulesHasNoDate(t *testing.T) {
	seedPlanSub(t, model.VLESS, vlessPlanSettings, tcpStream)
	if out := clashFor(t); strings.HasPrefix(out, "#") {
		t.Fatalf("a profile without template rules starts %q", strings.SplitN(out, "\n", 2)[0])
	}
	putS1OnPlan(t, seedRuleTemplate(t, "plan", planClashRule, false))
	setRuleSet(t, "ai", aiRuleSet)
	changedAt(t, &model.RuleSet{}, "name = ?", "ai", setChanged)
	out, err := PreviewClash("s1", "req.example.com", "", RuleTemplateSource{Content: "RULE-SET,ai,PROXY\nMATCH,PROXY"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(out, "#") {
		t.Fatalf("a preview starts %q", strings.SplitN(out, "\n", 2)[0])
	}
}
