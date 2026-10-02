package sub

import (
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

const (
	defaultClashRule = "DOMAIN-SUFFIX,default.example,DIRECT"
	planClashRule    = "DOMAIN-SUFFIX,plan.example,DIRECT"
)

// seedPlanSub gives subscription s1 a client on an inbound of the protocol.
func seedPlanSub(t *testing.T, protocol model.Protocol, settings, stream string) {
	t.Helper()
	seedSubDB(t)
	ib := seedTunnelSubInbound(t, protocol, "pr", "s1", "pr@e", settings, 4471)
	if err := database.GetDB().Model(ib).Update("stream_settings", stream).Error; err != nil {
		t.Fatalf("set stream: %v", err)
	}
}

// seedRuleTemplate saves a rule template, as the default one when asked.
func seedRuleTemplate(t *testing.T, name, content string, isDefault bool) int {
	t.Helper()
	tpl := &model.RuleTemplate{Name: name, Content: content, IsDefault: isDefault}
	if err := database.GetDB().Create(tpl).Error; err != nil {
		t.Fatalf("create template %s: %v", name, err)
	}
	return tpl.Id
}

// putS1OnPlan puts subscription s1's clients on a plan using the template; 0 is the default.
func putS1OnPlan(t *testing.T, templateId int) {
	t.Helper()
	db := database.GetDB()
	plan := &model.Plan{Name: "Plan", TemplateId: templateId}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if err := db.Model(&model.ClientRecord{}).Where("sub_id = ?", "s1").Update("plan_id", plan.Id).Error; err != nil {
		t.Fatalf("assign plan: %v", err)
	}
}

const tcpStream = `{"network":"tcp","security":"none"}`

const vlessPlanSettings = `{"clients":[{"id":"11111111-2222-4333-8444-000000004471","email":"pr@e","subId":"s1","enable":true}],"decryption":"none"}`

func clashFor(t *testing.T) string {
	t.Helper()
	out, _, err := NewSubClashService(NewSubService("")).GetClash("s1", "req.example.com")
	if err != nil {
		t.Fatalf("GetClash: %v", err)
	}
	return out
}

func TestClashUsesThePlansTemplateOverTheDefault(t *testing.T) {
	seedPlanSub(t, model.VLESS, vlessPlanSettings, tcpStream)
	seedRuleTemplate(t, "default", defaultClashRule, true)
	putS1OnPlan(t, seedRuleTemplate(t, "plan", planClashRule, false))
	if out := clashFor(t); !strings.Contains(out, "plan.example") || strings.Contains(out, "default.example") {
		t.Fatalf("want only the plan template's rule:\n%s", out)
	}
}

func TestClashUsesTheDefaultTemplateForAPlanWithoutOne(t *testing.T) {
	seedPlanSub(t, model.VLESS, vlessPlanSettings, tcpStream)
	seedRuleTemplate(t, "plan", planClashRule, false)
	seedRuleTemplate(t, "default", defaultClashRule, true)
	putS1OnPlan(t, 0)
	if out := clashFor(t); !strings.Contains(out, "default.example") || strings.Contains(out, "plan.example") {
		t.Fatalf("a plan without a template should get the default one:\n%s", out)
	}
}

func TestClashUsesTheDefaultTemplateForAClientWithoutAPlan(t *testing.T) {
	seedPlanSub(t, model.VLESS, vlessPlanSettings, tcpStream)
	seedRuleTemplate(t, "default", defaultClashRule, true)
	if out := clashFor(t); !strings.Contains(out, "default.example") {
		t.Fatalf("a client without a plan should get the default template:\n%s", out)
	}
}

func TestClashWithoutADefaultTemplateAddsNoRules(t *testing.T) {
	seedPlanSub(t, model.VLESS, vlessPlanSettings, tcpStream)
	seedRuleTemplate(t, "plan", planClashRule, false)
	putS1OnPlan(t, 0)
	if out := clashFor(t); strings.Contains(out, "plan.example") || !strings.Contains(out, "MATCH,PROXY") {
		t.Fatalf("with no default template the plan should get the plain proxy rule:\n%s", out)
	}
}

func TestLegacyClashLeavesThePlanTemplateOut(t *testing.T) {
	// Clash for Windows only takes Trojan over TLS.
	seedPlanSub(t, model.Trojan, `{"clients":[{"password":"pw-legacy","email":"pr@e","subId":"s1","enable":true}]}`,
		`{"network":"tcp","security":"tls","tlsSettings":{"serverName":"t.example"}}`)
	putS1OnPlan(t, seedRuleTemplate(t, "plan", planClashRule, false))
	out, _, err := NewSubClashService(NewSubService("")).GetClashLegacy("s1", "req.example.com")
	if err != nil {
		t.Fatalf("GetClashLegacy: %v", err)
	}
	if !strings.Contains(out, "type: trojan") || strings.Contains(out, "plan.example") {
		t.Fatalf("legacy Clash must carry the proxy but no custom rules:\n%s", out)
	}
}

// The templates page previews a template on a plan's member before anyone gets it.
func TestPreviewClashRendersTheGivenRulesInsteadOfTheTemplate(t *testing.T) {
	seedPlanSub(t, model.VLESS, vlessPlanSettings, tcpStream)
	putS1OnPlan(t, seedRuleTemplate(t, "plan", planClashRule, false))
	out, err := PreviewClash("s1", "req.example.com", "", "DOMAIN-SUFFIX,preview.example,DIRECT")
	if err != nil {
		t.Fatalf("PreviewClash: %v", err)
	}
	if !strings.Contains(out, "preview.example") || strings.Contains(out, "plan.example") || !strings.Contains(out, "type: vless") {
		t.Fatalf("want the previewed rules on the member's proxies:\n%s", out)
	}
}

func TestRefreshRemoteRoutingSourcesWarmsEveryClashSource(t *testing.T) {
	oldResolver := routingSourceResolver
	t.Cleanup(func() { routingSourceResolver = oldResolver })
	var mu sync.Mutex
	fetched := map[string]bool{}
	routingSourceResolver = newRemoteRoutingResolver(remoteRoutingTestClient(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		fetched[req.URL.String()] = true
		mu.Unlock()
		return remoteRoutingResponse(http.StatusOK, "rules:\n  - MATCH,PROXY\n"), nil
	}), false)

	sources := []string{"https://rules.example/global.yaml", "https://rules.example/plan.yaml"}
	RefreshRemoteRoutingSources("", append(sources, planClashRule), "")
	waitRemoteRoutingIdle(t, routingSourceResolver)

	mu.Lock()
	defer mu.Unlock()
	for _, source := range sources {
		if !fetched[source] {
			t.Fatalf("fetched %v, want every remote Clash source warmed", fetched)
		}
	}
}
