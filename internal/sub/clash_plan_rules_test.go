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
	globalClashRule = "DOMAIN-SUFFIX,global.example,DIRECT"
	planClashRule   = "DOMAIN-SUFFIX,plan.example,DIRECT"
)

// seedPlanSub puts the clients of subscription s1 on a plan with the given rules.
func seedPlanSub(t *testing.T, protocol model.Protocol, settings, stream, planRules string) {
	t.Helper()
	seedSubDB(t)
	ib := seedTunnelSubInbound(t, protocol, "pr", "s1", "pr@e", settings, 4471)
	db := database.GetDB()
	if err := db.Model(ib).Update("stream_settings", stream).Error; err != nil {
		t.Fatalf("set stream: %v", err)
	}
	plan := &model.Plan{Name: "Plan", ClashRules: planRules}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if err := db.Model(&model.ClientRecord{}).Where("sub_id = ?", "s1").Update("plan_id", plan.Id).Error; err != nil {
		t.Fatalf("assign plan: %v", err)
	}
}

const tcpStream = `{"network":"tcp","security":"none"}`

const vlessPlanSettings = `{"clients":[{"id":"11111111-2222-4333-8444-000000004471","email":"pr@e","subId":"s1","enable":true}],"decryption":"none"}`

func TestClashUsesThePlanRulesInsteadOfTheGlobalOnes(t *testing.T) {
	seedPlanSub(t, model.VLESS, vlessPlanSettings, tcpStream, planClashRule)
	// The global switch only governs the global rules, so a plan's own rules apply either way.
	for _, globalRouting := range []bool{true, false} {
		out, _, err := NewSubClashService(globalRouting, globalClashRule, NewSubService("")).GetClash("s1", "req.example.com")
		if err != nil {
			t.Fatalf("GetClash: %v", err)
		}
		if !strings.Contains(out, "plan.example") || strings.Contains(out, "global.example") {
			t.Fatalf("global routing %v: want only the plan rule:\n%s", globalRouting, out)
		}
	}
}

func TestClashKeepsTheGlobalRulesForAPlanWithoutItsOwn(t *testing.T) {
	seedPlanSub(t, model.VLESS, vlessPlanSettings, tcpStream, "")
	out, _, err := NewSubClashService(true, globalClashRule, NewSubService("")).GetClash("s1", "req.example.com")
	if err != nil {
		t.Fatalf("GetClash: %v", err)
	}
	if !strings.Contains(out, "global.example") {
		t.Fatalf("a plan without rules should inherit the global ones:\n%s", out)
	}
}

func TestLegacyClashLeavesThePlanRulesOut(t *testing.T) {
	// Clash for Windows only takes Trojan over TLS.
	seedPlanSub(t, model.Trojan, `{"clients":[{"password":"pw-legacy","email":"pr@e","subId":"s1","enable":true}]}`,
		`{"network":"tcp","security":"tls","tlsSettings":{"serverName":"t.example"}}`, planClashRule)
	out, _, err := NewSubClashService(false, "", NewSubService("")).GetClashLegacy("s1", "req.example.com")
	if err != nil {
		t.Fatalf("GetClashLegacy: %v", err)
	}
	if !strings.Contains(out, "type: trojan") || strings.Contains(out, "plan.example") {
		t.Fatalf("legacy Clash must carry the proxy but no custom rules:\n%s", out)
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
