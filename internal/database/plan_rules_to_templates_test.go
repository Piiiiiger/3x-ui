package database

import (
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

const (
	legacyPlanRules   = "proxy-groups:\n  - name: PROXY\n    type: select\n    proxies: [__PROXY_NODES__]\nrules:\n  - MATCH,PROXY\n"
	legacyGlobalRules = "DOMAIN-SUFFIX,example.com,DIRECT"
)

// Plans kept their Clash rules in a column of their own, and clients without plan
// rules got the global rules setting. The upgrade turns both into rule templates
// that render the same subscriptions.
func TestInitDB_MovesPlanRulesIntoTemplates(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "x-ui.db")
	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	seedLegacyPlanRules(t, "true")
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}

	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB on the legacy database: %v", err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	assertPlanRulesMoved(t, true)
}

// Global rules that were switched off still become a template, so nothing typed in
// is lost, but not the default one: the subscriptions keep going without them.
func TestInitDB_KeepsSwitchedOffGlobalRulesAsAPlainTemplate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "x-ui.db")
	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	seedLegacyPlanRules(t, "false")
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}

	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB on the legacy database: %v", err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	assertPlanRulesMoved(t, false)
}

func TestInitDB_MovesPlanRulesIntoTemplates_Postgres(t *testing.T) {
	usePostgresSchema(t)
	if err := InitDB(""); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	seedLegacyPlanRules(t, "true")
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}

	if err := InitDB(""); err != nil {
		t.Fatalf("InitDB on the legacy database: %v", err)
	}
	assertPlanRulesMoved(t, true)
}

// seedLegacyPlanRules recreates the column plans kept their rules in: two plans with
// the same rules, one without, and the global rules with routing on or off.
func seedLegacyPlanRules(t *testing.T, routing string) {
	t.Helper()
	legacy := []string{
		"ALTER TABLE plans ADD COLUMN clash_rules text DEFAULT ''",
		"INSERT INTO plans (name, sort_index, clash_rules) VALUES ('Alpha 100G', 1, '" + legacyPlanRules + "')",
		"INSERT INTO plans (name, sort_index, clash_rules) VALUES ('Beta 200G', 2, '" + legacyPlanRules + "')",
		"INSERT INTO plans (name, sort_index, clash_rules) VALUES ('Gamma', 3, '')",
		"INSERT INTO settings (key, value) VALUES ('subClashEnableRouting', '" + routing + "')",
		"INSERT INTO settings (key, value) VALUES ('subClashRules', '" + legacyGlobalRules + "')",
	}
	for _, stmt := range legacy {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("seed legacy plan rules %q: %v", stmt, err)
		}
	}
}

func assertPlanRulesMoved(t *testing.T, globalIsDefault bool) {
	t.Helper()
	var templates []model.RuleTemplate
	if err := db.Order("id").Find(&templates).Error; err != nil {
		t.Fatal(err)
	}
	byName := map[string]model.RuleTemplate{}
	for _, tpl := range templates {
		byName[tpl.Name] = tpl
	}
	if len(templates) != 3 {
		t.Fatalf("templates = %+v, want one per plan with rules plus the global rules", templates)
	}
	for _, name := range []string{"Alpha 100G", "Beta 200G"} {
		if tpl, ok := byName[name]; !ok || tpl.Content != legacyPlanRules || tpl.IsDefault {
			t.Errorf("template %q = %+v (found %v), want the plan's own rules, not default", name, tpl, ok)
		}
	}
	if global, ok := byName["global"]; !ok || global.Content != legacyGlobalRules || global.IsDefault != globalIsDefault {
		t.Errorf("template global = %+v (found %v), want the global rules, default %v", global, ok, globalIsDefault)
	}

	planTemplate := func(name string) int {
		var plan model.Plan
		if err := db.Where("name = ?", name).First(&plan).Error; err != nil {
			t.Fatalf("plan %s: %v", name, err)
		}
		return plan.TemplateId
	}
	if got := planTemplate("Alpha 100G"); got != byName["Alpha 100G"].Id {
		t.Errorf("Alpha 100G uses template %d, want its own %d", got, byName["Alpha 100G"].Id)
	}
	if got := planTemplate("Beta 200G"); got != byName["Beta 200G"].Id {
		t.Errorf("Beta 200G uses template %d, want its own %d", got, byName["Beta 200G"].Id)
	}
	if got := planTemplate("Gamma"); got != 0 {
		t.Errorf("Gamma uses template %d, want 0 (the default)", got)
	}

	var versions int64
	if err := db.Model(&model.RuleTemplateVersion{}).Count(&versions).Error; err != nil || versions != 3 {
		t.Errorf("template versions = %d (err %v), want each template's moved content kept as its first version", versions, err)
	}
	if db.Migrator().HasColumn(&model.Plan{}, "clash_rules") {
		t.Error("plans.clash_rules survived the upgrade")
	}
	var leftover int64
	if err := db.Model(&model.Setting{}).Where("key IN ?", []string{"subClashEnableRouting", "subClashRules"}).Count(&leftover).Error; err != nil || leftover != 0 {
		t.Errorf("global rules settings left: %d (err %v), want none", leftover, err)
	}
}
