package service

import (
	"reflect"
	"strings"
	"testing"

	yaml "github.com/goccy/go-yaml"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

const baseRules = `proxy-groups:
  - name: PROXY
    type: select
    proxies: [__PROXY_NODES__]
rules:
  - DOMAIN-SUFFIX,a.example,DIRECT
  - GEOIP,CN,DIRECT
  - MATCH,PROXY
`

func mustCreateVariant(t *testing.T, name string, baseId int, content string) *model.RuleTemplate {
	t.Helper()
	tpl, err := (&RuleTemplateService{}).Create(RuleTemplateInput{Name: name, Content: content, BaseId: baseId})
	if err != nil {
		t.Fatalf("create variant %s: %v", name, err)
	}
	return tpl
}

func mustParseMap(t *testing.T, content string) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		t.Fatalf("parse %q: %v", content, err)
	}
	return doc
}

func templateRow(t *testing.T, id int) *model.RuleTemplate {
	t.Helper()
	tpl, err := (&RuleTemplateService{}).Get(id)
	if err != nil {
		t.Fatalf("get template %d: %v", id, err)
	}
	return tpl
}

func TestVariantSavesOnlyWhatItChanges(t *testing.T) {
	setupConflictDB(t)
	base := mustCreateTemplate(t, "base", baseRules)
	variant := mustCreateVariant(t, "mine", base.Id, "prepend-rules:\n  - DOMAIN,mine.example,PROXY\n")
	got := templateRow(t, variant.Id)
	if got.BaseId != base.Id || !strings.Contains(got.Content, "mine.example") || strings.Contains(got.Content, "GEOIP") {
		t.Fatalf("saved variant = base %d, content %q", got.BaseId, got.Content)
	}
}

// A variant that could not render is refused on save, as a full template would be.
func TestVariantRefusesWhatCouldNotRender(t *testing.T) {
	setupConflictDB(t)
	base := mustCreateTemplate(t, "base", baseRules)
	lines := mustCreateTemplate(t, "lines", "MATCH,DIRECT")
	variant := mustCreateVariant(t, "mine", base.Id, "mode: global\n")
	for _, tc := range []struct {
		name    string
		baseId  int
		content string
		want    string
	}{
		{"no base", 9999, "mode: global\n", "base template not found"},
		{"variant base", variant.Id, "mode: rule\n", "not another variant"},
		{"lines base", lines.Id, "mode: global\n", "must be a YAML document"},
		{"list content", base.Id, "- MATCH,PROXY\n", "a YAML map"},
		{"no nodes", base.Id, "proxy-groups:\n  - name: P\n    type: select\n    proxies: [DIRECT]\n", "__PROXY_NODES__"},
	} {
		_, err := (&RuleTemplateService{}).Create(RuleTemplateInput{Name: tc.name, Content: tc.content, BaseId: tc.baseId})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it refused with %q", tc.name, err, tc.want)
		}
	}
}

// Its variants depend on a base: it can neither go nor become a variant itself,
// and a save must leave them something to merge onto.
func TestBaseKeepsWhatItsVariantsNeed(t *testing.T) {
	setupConflictDB(t)
	base := mustCreateTemplate(t, "base", baseRules)
	other := mustCreateTemplate(t, "other", baseRules)
	mustCreateVariant(t, "mine", base.Id, "mode: global\n")
	s := &RuleTemplateService{}

	if err := s.Delete(base.Id); err == nil || !strings.Contains(err.Error(), "1 variant") {
		t.Errorf("deleting a base: err = %v, want it refused naming its variant", err)
	}
	if _, err := s.Update(base.Id, RuleTemplateInput{Name: "base", Content: baseRules, BaseId: other.Id}); err == nil ||
		!strings.Contains(err.Error(), "variant") {
		t.Errorf("making a base a variant: err = %v, want it refused", err)
	}
	if _, err := s.Update(base.Id, RuleTemplateInput{Name: "base", Content: "MATCH,DIRECT"}); err == nil ||
		!strings.Contains(err.Error(), "YAML document") {
		t.Errorf("saving a base as rule lines: err = %v, want it refused", err)
	}
	if got := templateRow(t, base.Id); got.Content != baseRules || got.BaseId != 0 {
		t.Fatalf("a refused save changed the base: %+v", got)
	}
}

// Restoring a variant's earlier save brings back its differences, still on its base.
func TestVariantRestoreStaysAVariant(t *testing.T) {
	setupConflictDB(t)
	base := mustCreateTemplate(t, "base", baseRules)
	variant := mustCreateVariant(t, "mine", base.Id, "mode: global\n")
	s := &RuleTemplateService{}
	if _, err := s.Update(variant.Id, RuleTemplateInput{Name: "mine", Content: "mode: direct\n", BaseId: base.Id}); err != nil {
		t.Fatalf("update: %v", err)
	}
	versions, err := s.Versions(variant.Id)
	if err != nil || len(versions) != 2 {
		t.Fatalf("versions = %v (err %v), want two", versions, err)
	}
	if _, err := s.Restore(versions[1].Id); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := templateRow(t, variant.Id); got.BaseId != base.Id || got.Content != "mode: global\n" {
		t.Fatalf("restored variant = base %d, content %q", got.BaseId, got.Content)
	}
}

func TestVariantCanMoveToAnotherBase(t *testing.T) {
	setupConflictDB(t)
	first := mustCreateTemplate(t, "first", baseRules)
	second := mustCreateTemplate(t, "second", baseRules)
	variant := mustCreateVariant(t, "mine", first.Id, "mode: global\n")
	if _, err := (&RuleTemplateService{}).Update(variant.Id, RuleTemplateInput{
		Name: "mine", Content: "mode: global\n", BaseId: second.Id,
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got := templateRow(t, variant.Id); got.BaseId != second.Id {
		t.Fatalf("variant base = %d, want %d", got.BaseId, second.Id)
	}
}

func TestListShowsWhatEachVariantChanges(t *testing.T) {
	setupConflictDB(t)
	base := mustCreateTemplate(t, "base", baseRules)
	mustCreateVariant(t, "mine", base.Id, "mode: global\nprepend-rules:\n  - DOMAIN,a.example,PROXY\n  - DOMAIN,b.example,PROXY\n")
	list, err := (&RuleTemplateService{}).List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	byName := map[string]RuleTemplateSummary{}
	for _, tpl := range list {
		byName[tpl.Name] = tpl
	}
	if got := byName["base"]; got.BaseId != 0 || len(got.Changes) != 0 {
		t.Errorf("base summary = %+v, want no base and no changes", got)
	}
	want := []RuleTemplateChange{{Key: "mode", Replaced: true}, {Key: "rules", Added: 2}}
	if got := byName["mine"]; got.BaseId != base.Id || !reflect.DeepEqual(got.Changes, want) {
		t.Errorf("variant summary = %+v, want base %d and changes %+v", got, base.Id, want)
	}
}

// A copy of the base written another way is the base: its plans move to it, its
// default star too, and the copy goes.
func TestConvertFoldsACopyOfTheBaseIntoIt(t *testing.T) {
	setupConflictDB(t)
	base := mustCreateTemplate(t, "base", baseRules)
	copyContent := "rules: ['DOMAIN-SUFFIX,a.example,DIRECT', 'GEOIP,CN,DIRECT', 'MATCH,PROXY']\n" +
		"proxy-groups:\n  - {name: PROXY, type: select, proxies: [__PROXY_NODES__]}\n"
	twin := mustCreateTemplate(t, "twin", copyContent)
	db := database.GetDB()
	if err := db.Create(&model.Plan{Name: "Monthly", TemplateId: twin.Id}).Error; err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	s := &RuleTemplateService{}
	if err := s.SetDefault(twin.Id); err != nil {
		t.Fatalf("set default: %v", err)
	}

	got, err := s.ConvertToVariant(twin.Id, base.Id, false, true)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if !got.Identical || got.PlanCount != 1 {
		t.Fatalf("conversion = %+v, want identical with one plan", got)
	}
	var plan model.Plan
	db.Where("name = ?", "Monthly").First(&plan)
	if plan.TemplateId != base.Id {
		t.Errorf("the plan uses template %d, want the base %d", plan.TemplateId, base.Id)
	}
	if !templateRow(t, base.Id).IsDefault {
		t.Error("the default star did not move to the base")
	}
	if _, err := s.Get(twin.Id); err == nil {
		t.Error("the copy of the base was kept")
	}
}

func TestConvertKeepsOnlyTheDifferences(t *testing.T) {
	setupConflictDB(t)
	base := mustCreateTemplate(t, "base", baseRules)
	full := mustCreateTemplate(t, "full", "mode: global\n"+strings.Replace(baseRules, "rules:\n",
		"rules:\n  - DOMAIN,mine.example,PROXY\n", 1))
	s := &RuleTemplateService{}

	dry, err := s.ConvertToVariant(full.Id, base.Id, false, false)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if dry.Identical || dry.Moved != 0 || len(dry.Changes) != 2 {
		t.Fatalf("dry run = %+v, want two changes and nothing moved", dry)
	}
	if got := templateRow(t, full.Id); got.BaseId != 0 {
		t.Fatal("the dry run converted the template")
	}

	if _, err := s.ConvertToVariant(full.Id, base.Id, false, true); err != nil {
		t.Fatalf("convert: %v", err)
	}
	got := templateRow(t, full.Id)
	want := map[string]any{"mode": "global", "prepend-rules": []any{"DOMAIN,mine.example,PROXY"}}
	if got.BaseId != base.Id || !reflect.DeepEqual(mustParseMap(t, got.Content), want) {
		t.Fatalf("converted = base %d, content %q", got.BaseId, got.Content)
	}
	if versions, _ := s.Versions(full.Id); len(versions) != 2 {
		t.Errorf("versions = %d, want the conversion kept as a save that can be undone", len(versions))
	}
}

// Rules between the base's own can only move to the front, which may change what
// they match: the conversion says how many, and needs a yes.
func TestConvertMovesRulesFromInsideOnlyWhenAllowed(t *testing.T) {
	setupConflictDB(t)
	base := mustCreateTemplate(t, "base", baseRules)
	full := mustCreateTemplate(t, "full", strings.Replace(baseRules, "  - GEOIP,CN,DIRECT\n",
		"  - DOMAIN,mine.example,PROXY\n  - GEOIP,CN,DIRECT\n", 1))
	s := &RuleTemplateService{}

	dry, err := s.ConvertToVariant(full.Id, base.Id, false, false)
	if err != nil || dry.Moved != 1 {
		t.Fatalf("dry run = %+v (err %v), want one rule moved", dry, err)
	}
	if _, err := s.ConvertToVariant(full.Id, base.Id, false, true); err == nil || !strings.Contains(err.Error(), "1 rule") {
		t.Fatalf("convert without consent: err = %v, want it refused naming the count", err)
	}
	if got := templateRow(t, full.Id); got.BaseId != 0 {
		t.Fatal("a refused conversion changed the template")
	}
	if _, err := s.ConvertToVariant(full.Id, base.Id, true, true); err != nil {
		t.Fatalf("convert with consent: %v", err)
	}
	want := map[string]any{"prepend-rules": []any{"DOMAIN,mine.example,PROXY"}}
	if got := templateRow(t, full.Id); !reflect.DeepEqual(mustParseMap(t, got.Content), want) {
		t.Fatalf("converted content = %q", got.Content)
	}
}

// A secret of ".inf" is a string; written out bare it would read back as infinity.
func TestConvertKeepsStringsThatLookLikeOtherTypes(t *testing.T) {
	setupConflictDB(t)
	base := mustCreateTemplate(t, "base", baseRules)
	full := mustCreateTemplate(t, "full", "secret: '.inf'\n"+baseRules)
	if _, err := (&RuleTemplateService{}).ConvertToVariant(full.Id, base.Id, false, true); err != nil {
		t.Fatalf("convert: %v", err)
	}
	if got := mustParseMap(t, templateRow(t, full.Id).Content); got["secret"] != ".inf" {
		t.Fatalf("converted secret = %#v, want the string .inf", got["secret"])
	}
}

// What YAML cannot carry through a write and a read (here a tab in a string) would
// change what people get, so the conversion is refused instead.
func TestConvertRefusesADifferenceThatWouldNotReadBackTheSame(t *testing.T) {
	setupConflictDB(t)
	base := mustCreateTemplate(t, "base", baseRules)
	full := mustCreateTemplate(t, "full", "secret: \"tab\\there\"\n"+baseRules)
	_, err := (&RuleTemplateService{}).ConvertToVariant(full.Id, base.Id, false, false)
	if err == nil || !strings.Contains(err.Error(), "would not render as the template") {
		t.Fatalf("err = %v, want the conversion refused", err)
	}
}

func TestConvertRefusesWhatCannotBeAVariant(t *testing.T) {
	setupConflictDB(t)
	base := mustCreateTemplate(t, "base", baseRules)
	parent := mustCreateTemplate(t, "parent", baseRules)
	mustCreateVariant(t, "child", parent.Id, "mode: global\n")
	variant := mustCreateVariant(t, "mine", base.Id, "mode: global\n")
	noRules := mustCreateTemplate(t, "norules", "proxy-groups:\n  - name: PROXY\n    type: select\n    proxies: [__PROXY_NODES__]\n")
	lines := mustCreateTemplate(t, "lines", "MATCH,DIRECT")
	for _, tc := range []struct {
		name         string
		id, baseId   int
		wantContains string
	}{
		{"itself", base.Id, base.Id, "itself"},
		{"a base", parent.Id, base.Id, "1 variant"},
		{"already a variant", variant.Id, base.Id, "already a variant"},
		{"onto a variant", noRules.Id, variant.Id, "not another variant"},
		{"drops rules", noRules.Id, base.Id, `"rules"`},
		{"rule lines", lines.Id, base.Id, "YAML"},
	} {
		if _, err := (&RuleTemplateService{}).ConvertToVariant(tc.id, tc.baseId, true, false); err == nil ||
			!strings.Contains(err.Error(), tc.wantContains) {
			t.Errorf("%s: err = %v, want it refused with %q", tc.name, err, tc.wantContains)
		}
	}
}

// A variant being edited previews as its merge, so a merge that loses the nodes is refused.
func TestPreviewChecksAVariantAsItsMerge(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	plan := &model.Plan{Name: "Monthly"}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	if err := db.Create(&model.ClientRecord{Email: "amy", SubID: "sub-amy", PlanId: plan.Id, Enable: true}).Error; err != nil {
		t.Fatalf("seed client: %v", err)
	}
	base := mustCreateTemplate(t, "base", baseRules)
	s := &RuleTemplateService{}
	subId, baseContent, err := s.PreviewMember(plan.Id, "mode: global\n", base.Id)
	if err != nil || subId == nil || subId.SubID != "sub-amy" || subId.Email != "amy" || baseContent != baseRules {
		t.Fatalf("preview = %+v, base %q, err %v", subId, baseContent, err)
	}
	if _, _, err := s.PreviewMember(plan.Id, "proxy-groups:\n  - name: P\n    type: select\n    proxies: [DIRECT]\n", base.Id); err == nil {
		t.Error("previewing a variant whose merge loses the nodes was accepted")
	}
}
