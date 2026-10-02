package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

const (
	groupTemplate  = "proxy-groups:\n  - name: PROXY\n    type: select\n    proxies: [__PROXY_NODES__]\nrules:\n  - MATCH,PROXY\n"
	filterTemplate = "proxy-groups:\n  - name: HK\n    type: url-test\n    filter: 香港\nrules:\n  - MATCH,HK\n"
)

func mustCreateTemplate(t *testing.T, name, content string) *model.RuleTemplate {
	t.Helper()
	tpl, err := (&RuleTemplateService{}).Create(RuleTemplateInput{Name: name, Content: content})
	if err != nil {
		t.Fatalf("create template %s: %v", name, err)
	}
	return tpl
}

func TestRuleTemplateTakesRulesYAMLAndHTTPSURLs(t *testing.T) {
	setupConflictDB(t)
	for name, content := range map[string]string{
		"lines":  "DOMAIN-SUFFIX,example.com,DIRECT\nMATCH,PROXY",
		"groups": groupTemplate,
		"filter": filterTemplate,
		"remote": "https://rules.example/clash.yaml",
	} {
		if _, err := (&RuleTemplateService{}).Create(RuleTemplateInput{Name: name, Content: content}); err != nil {
			t.Errorf("template %s refused: %v", name, err)
		}
	}
}

// What would render a subscription with no way to its nodes, or not parse at all,
// is refused on save rather than found out by the people using it.
func TestRuleTemplateRefusesContentThatCannotWork(t *testing.T) {
	setupConflictDB(t)
	for _, tc := range []struct{ name, content, want string }{
		{"empty", "  \n", "content is required"},
		{"credentials", "https://user:pw@rules.example/clash.yaml", "must not contain URL credentials"},
		{"no nodes", "proxy-groups:\n  - name: PROXY\n    type: select\n    proxies: [DIRECT]\n", "__PROXY_NODES__"},
		{"broken yaml", "proxy-groups:\n  - name: [PROXY\n", "not valid YAML"},
	} {
		_, err := (&RuleTemplateService{}).Create(RuleTemplateInput{Name: tc.name, Content: tc.content})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it refused with %q", tc.name, err, tc.want)
		}
	}
}

func TestRuleTemplateNamesAreRequiredAndUnique(t *testing.T) {
	setupConflictDB(t)
	mustCreateTemplate(t, "alpha_v3", groupTemplate)
	s := &RuleTemplateService{}
	if _, err := s.Create(RuleTemplateInput{Name: " alpha_v3 ", Content: groupTemplate}); err == nil {
		t.Error("a second template named alpha_v3 was accepted")
	}
	if _, err := s.Create(RuleTemplateInput{Name: "  ", Content: groupTemplate}); err == nil {
		t.Error("a template without a name was accepted")
	}
}

// Every save that changes the content is kept, newest first, up to the cap.
func TestRuleTemplateKeepsTheLatestVersions(t *testing.T) {
	setupConflictDB(t)
	tpl := mustCreateTemplate(t, "v", "MATCH,DIRECT")
	s := &RuleTemplateService{}
	for i := range ruleTemplateVersionsKept + 5 {
		if _, err := s.Update(tpl.Id, RuleTemplateInput{Name: "v", Content: fmt.Sprintf("DOMAIN,%d.example,DIRECT", i)}); err != nil {
			t.Fatalf("update %d: %v", i, err)
		}
	}
	if _, err := s.Update(tpl.Id, RuleTemplateInput{Name: "v", Content: fmt.Sprintf("DOMAIN,%d.example,DIRECT", ruleTemplateVersionsKept+4)}); err != nil {
		t.Fatalf("unchanged update: %v", err)
	}
	versions, err := s.Versions(tpl.Id)
	if err != nil {
		t.Fatalf("versions: %v", err)
	}
	if len(versions) != ruleTemplateVersionsKept {
		t.Fatalf("kept %d versions, want %d", len(versions), ruleTemplateVersionsKept)
	}
	newest, _ := s.version(versions[0].Id)
	if newest.Content != fmt.Sprintf("DOMAIN,%d.example,DIRECT", ruleTemplateVersionsKept+4) {
		t.Errorf("newest version holds %q, want the last save", newest.Content)
	}
}

// Renaming, or saving without a change, keeps the history for the saves that changed it.
func TestRuleTemplateSaveWithoutAChangeAddsNoVersion(t *testing.T) {
	setupConflictDB(t)
	tpl := mustCreateTemplate(t, "same", "MATCH,DIRECT")
	s := &RuleTemplateService{}
	if _, err := s.Update(tpl.Id, RuleTemplateInput{Name: "renamed", Content: "MATCH,DIRECT"}); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if versions, _ := s.Versions(tpl.Id); len(versions) != 1 {
		t.Fatalf("versions after a rename: %d, want only the first save", len(versions))
	}
}

func TestRuleTemplateRestoreBringsBackAVersionAsANewSave(t *testing.T) {
	setupConflictDB(t)
	tpl := mustCreateTemplate(t, "r", "MATCH,DIRECT")
	s := &RuleTemplateService{}
	if _, err := s.Update(tpl.Id, RuleTemplateInput{Name: "r", Content: "MATCH,PROXY"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	versions, _ := s.Versions(tpl.Id)
	restored, err := s.Restore(versions[len(versions)-1].Id)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restored.Content != "MATCH,DIRECT" {
		t.Errorf("restored content %q, want the first save", restored.Content)
	}
	if after, _ := s.Versions(tpl.Id); len(after) != 3 {
		t.Errorf("versions after restoring: %d, want the restore kept as a third save", len(after))
	}
}

func TestRuleTemplateDefaultMovesAndCannotBeDeleted(t *testing.T) {
	setupConflictDB(t)
	a := mustCreateTemplate(t, "a", "MATCH,DIRECT")
	b := mustCreateTemplate(t, "b", "MATCH,PROXY")
	s := &RuleTemplateService{}
	if err := s.SetDefault(a.Id); err != nil {
		t.Fatalf("set default a: %v", err)
	}
	if err := s.SetDefault(b.Id); err != nil {
		t.Fatalf("set default b: %v", err)
	}
	var defaults []string
	database.GetDB().Model(&model.RuleTemplate{}).Where("is_default = ?", true).Pluck("name", &defaults)
	if len(defaults) != 1 || defaults[0] != "b" {
		t.Fatalf("default templates %v, want only b", defaults)
	}
	if err := s.Delete(b.Id); err == nil || !strings.Contains(err.Error(), "default") {
		t.Errorf("deleting the default template: err = %v, want it refused", err)
	}
}

func TestRuleTemplateDeleteRefusesWhileAPlanUsesIt(t *testing.T) {
	setupConflictDB(t)
	used := mustCreateTemplate(t, "used", "MATCH,DIRECT")
	spare := mustCreateTemplate(t, "spare", "MATCH,PROXY")
	if err := database.GetDB().Create(&model.Plan{Name: "Monthly", TemplateId: used.Id}).Error; err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	s := &RuleTemplateService{}
	if err := s.Delete(used.Id); err == nil || !strings.Contains(err.Error(), "1 plan") {
		t.Errorf("deleting a template a plan uses: err = %v, want it refused naming the count", err)
	}
	if err := s.Delete(spare.Id); err != nil {
		t.Fatalf("delete spare: %v", err)
	}
	var versions int64
	database.GetDB().Model(&model.RuleTemplateVersion{}).Where("template_id = ?", spare.Id).Count(&versions)
	if versions != 0 {
		t.Errorf("the deleted template left %d versions behind", versions)
	}
}

// The default template also serves every plan that names none, so its count has them.
func TestRuleTemplateListCountsThePlansUsingEach(t *testing.T) {
	setupConflictDB(t)
	yamlTpl := mustCreateTemplate(t, "yaml", groupTemplate)
	remote := mustCreateTemplate(t, "remote", "https://rules.example/clash.yaml")
	mustCreateTemplate(t, "lines", "MATCH,DIRECT")
	mustCreateTemplate(t, "list", "- DOMAIN-SUFFIX,example.com,DIRECT\n- MATCH,PROXY\n")
	s := &RuleTemplateService{}
	if err := s.SetDefault(remote.Id); err != nil {
		t.Fatalf("set default: %v", err)
	}
	db := database.GetDB()
	for i, tplID := range []int{yamlTpl.Id, yamlTpl.Id, 0, remote.Id} {
		if err := db.Create(&model.Plan{Name: fmt.Sprintf("p%d", i), TemplateId: tplID}).Error; err != nil {
			t.Fatalf("seed plan: %v", err)
		}
	}
	list, err := s.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	got := map[string]string{}
	for _, tpl := range list {
		got[tpl.Name] = fmt.Sprintf("%s/%d/%v/%d", tpl.Kind, tpl.PlanCount, tpl.IsDefault, tpl.Size)
	}
	want := map[string]string{
		"yaml":   fmt.Sprintf("yaml/2/false/%d", len(groupTemplate)),
		"remote": fmt.Sprintf("remote/2/true/%d", len("https://rules.example/clash.yaml")),
		"lines":  "rules/0/false/12",
		"list":   "yaml/0/false/49",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("list = %v\nwant   %v", got, want)
	}
}

// A preview renders for the plan's first member, so it needs one and a template
// that would save.
func TestRuleTemplatePreviewPicksThePlansFirstMember(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	plan := &model.Plan{Name: "Monthly"}
	empty := &model.Plan{Name: "Empty"}
	for _, p := range []*model.Plan{plan, empty} {
		if err := db.Create(p).Error; err != nil {
			t.Fatalf("seed plan: %v", err)
		}
	}
	for _, c := range []model.ClientRecord{
		{Email: "zed", SubID: "sub-zed", PlanId: plan.Id, Enable: true},
		{Email: "amy", SubID: "sub-amy", PlanId: plan.Id, Enable: true},
	} {
		if err := db.Create(&c).Error; err != nil {
			t.Fatalf("seed client: %v", err)
		}
	}
	s := &RuleTemplateService{}
	if subId, _, err := s.PreviewSubId(plan.Id, "MATCH,DIRECT", 0); err != nil || subId != "sub-zed" {
		t.Errorf("preview member = %q (err %v), want the first one added, sub-zed", subId, err)
	}
	if _, _, err := s.PreviewSubId(empty.Id, "MATCH,DIRECT", 0); err == nil || !strings.Contains(err.Error(), "no users") {
		t.Errorf("preview on an empty plan: err = %v, want it refused", err)
	}
	if _, _, err := s.PreviewSubId(plan.Id, "proxy-groups:\n  - name: [PROXY\n", 0); err == nil {
		t.Error("preview of broken YAML was accepted")
	}
}
