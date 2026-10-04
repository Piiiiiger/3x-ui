package sub

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	yaml "github.com/goccy/go-yaml"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// A cut-down 妙妙屋X template: proxies: null, groups built on __PROXY_NODES__,
// a filter-only url-test group and its own MATCH.
const mmwxStyleTemplate = `port: 7890
mode: rule
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
  - DOMAIN-SUFFIX,cn,🎯 全球直连
  - MATCH,🔰 节点选择
`

func TestPlanTemplateRendersLikeMMWX(t *testing.T) {
	seedSubDB(t)
	vless := func(email string) string {
		return `{"clients":[{"id":"11111111-2222-4333-8444-0000000044` + email[:2] + `","email":"` + email + `","subId":"s1","enable":true}],"decryption":"none"}`
	}
	db := database.GetDB()
	for i, tag := range []string{"洛杉矶-Core", "新加坡-Edge"} {
		email := []string{"81@e", "82@e"}[i]
		ib := seedTunnelSubInbound(t, model.VLESS, tag, "s1", email, vless(email), 4481+i)
		if err := db.Model(ib).Update("stream_settings", tcpStream).Error; err != nil {
			t.Fatalf("set stream: %v", err)
		}
	}
	putS1OnPlan(t, seedRuleTemplate(t, "Template", mmwxStyleTemplate, false))

	out, _, err := NewSubClashService(NewSubService("{{INBOUND}}")).GetClash("s1", "req.example.com")
	if err != nil {
		t.Fatalf("GetClash: %v", err)
	}
	var got struct {
		Port    int `yaml:"port"`
		Proxies []struct {
			Name string `yaml:"name"`
		} `yaml:"proxies"`
		Groups []struct {
			Name    string   `yaml:"name"`
			Proxies []string `yaml:"proxies"`
			Filter  string   `yaml:"filter"`
		} `yaml:"proxy-groups"`
		Rules []string `yaml:"rules"`
	}
	if err := yaml.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse %s: %v", out, err)
	}

	var names []string
	for _, p := range got.Proxies {
		names = append(names, p.Name)
	}
	assertStrings(t, "proxies", names, []string{"洛杉矶-Core", "新加坡-Edge"})
	if got.Port != 7890 {
		t.Fatalf("port = %d, want the template's 7890", got.Port)
	}
	want := map[string][]string{
		"🔰 节点选择":          {"洛杉矶-Core", "新加坡-Edge", "🎯 全球直连", "🇸🇬 新加坡（自动）-Edge"},
		"🇸🇬 新加坡（自动）-Edge": {"新加坡-Edge"},
		"🎯 全球直连":          {"DIRECT", "洛杉矶-Core", "新加坡-Edge"},
	}
	if len(got.Groups) != len(want) {
		t.Fatalf("groups = %+v, want exactly the template's three", got.Groups)
	}
	for _, g := range got.Groups {
		assertStrings(t, "group "+g.Name, g.Proxies, want[g.Name])
		if g.Filter != "" {
			t.Fatalf("group %s still carries filter %q after being filled", g.Name, g.Filter)
		}
	}
	assertStrings(t, "rules", got.Rules, []string{"DOMAIN-SUFFIX,cn,🎯 全球直连", "MATCH,🔰 节点选择"})
}

// The usage info node is a placeholder socks5 proxy: groups skip it next to real
// proxies, but keep it when it is all an expired subscription has.
func TestTemplateGroupsSkipTheInfoNodeUnlessItIsAlone(t *testing.T) {
	info := map[string]any{"name": "⏳ Expired", "type": "socks5", "server": "127.0.0.1", "port": 1080}
	node := map[string]any{"name": "洛杉矶-Core", "type": "vless", "server": "203.0.113.5", "port": 443}
	assertStrings(t, "with a node", clashProxyNamesForGroups([]map[string]any{info, node}), []string{"洛杉矶-Core"})
	assertStrings(t, "alone", clashProxyNamesForGroups([]map[string]any{info}), []string{"⏳ Expired"})
}

func TestTemplateGroupsUsePlanNodeAssignments(t *testing.T) {
	config := map[string]any{
		"proxies": []map[string]any{
			{"name": "香港", clashPlanInboundIDKey: 1},
			{"name": "新加坡", clashPlanInboundIDKey: 2},
		},
		clashPlanGroupsKey: map[string]map[int]struct{}{
			"节点选择": {2: {}},
		},
		"proxy-groups": []map[string]any{{
			"name":    "节点选择",
			"type":    "select",
			"proxies": []any{clashProxyNodesPlaceholder},
		}},
	}
	expandClashTemplateGroups(config)
	groups, ok := config["proxy-groups"].([]map[string]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("groups = %#v", config["proxy-groups"])
	}
	members, _ := groups[0]["proxies"].([]any)
	got := make([]string, 0, len(members))
	for _, member := range members {
		got = append(got, member.(string))
	}
	assertStrings(t, "assigned group", got, []string{"新加坡"})
}

func TestPlanNodeAssignmentsOverrideIncludeAllGroups(t *testing.T) {
	config := map[string]any{
		"proxies": []map[string]any{
			{"name": "香港", clashPlanInboundIDKey: 1},
			{"name": "新加坡", clashPlanInboundIDKey: 2},
		},
		clashPlanGroupsKey: map[string]map[int]struct{}{
			"海外自动": {},
		},
		"proxy-groups": []map[string]any{{
			"name":                "海外自动",
			"type":                "url-test",
			"include-all-proxies": true,
			"filter":              "新加坡",
		}},
	}
	expandClashTemplateGroups(config)
	group := config["proxy-groups"].([]map[string]any)[0]
	if _, ok := group["include-all-proxies"]; ok {
		t.Fatal("include-all-proxies should not bypass a plan assignment")
	}
	members, _ := group["proxies"].([]any)
	got := make([]string, 0, len(members))
	for _, member := range members {
		got = append(got, member.(string))
	}
	assertStrings(t, "assigned include-all group", got, []string{})
}

func assertStrings(t *testing.T, what string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %q, want %q", what, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s = %q, want %q", what, got, want)
		}
	}
}

func TestSubscriptionRootRedirectsToThePortal(t *testing.T) {
	seedSubDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewSUBController(router.Group("/"), WithSUBPath("/x/"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/x/portal" {
		t.Fatalf("GET / = %d to %q, want 302 to /x/portal", rec.Code, rec.Header().Get("Location"))
	}
}
