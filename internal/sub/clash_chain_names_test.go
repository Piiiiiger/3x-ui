package sub

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	yaml "github.com/goccy/go-yaml"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestNamedChainVariantsPreserveRoutingAndAIDefault(t *testing.T) {
	for _, withPlan := range []bool{false, true} {
		t.Run(fmt.Sprintf("plan=%t", withPlan), func(t *testing.T) {
			seedSubDB(t)
			db := database.GetDB()
			relay := seedSubInbound(t, "s1", "奶爸", 4501, 1, tcpStream)
			target := seedSubInbound(t, "s1", "新加坡-家宽", 4502, 2, tcpStream)
			chain := model.ProxyChain{TargetInboundId: target.Id, RelayInboundId: relay.Id, Enabled: true, DirectName: "Home A", RelayName: "Home B"}
			if err := db.Create(&chain).Error; err != nil {
				t.Fatal(err)
			}
			tpl := seedRuleTemplate(t, "aliases", "proxy-groups:\n  - name: 🤖 AI 服务\n    type: select\n    proxies: [__PROXY_NODES__]\nrules: [MATCH,🤖 AI 服务]\n", !withPlan)
			if withPlan {
				putS1OnPlan(t, tpl)
				keys, _ := json.Marshal([]string{model.PlanNodeKey(relay.Id, false), model.PlanNodeKey(target.Id, false), model.PlanChainKey(target.Id, chain.Id)})
				if err := db.Model(&model.Plan{}).Where("name = ?", "Plan").Update("node_keys", string(keys)).Error; err != nil {
					t.Fatal(err)
				}
			}
			for _, renamed := range []bool{true, false} {
				if !renamed {
					if err := db.Model(&chain).Updates(map[string]any{"direct_name": "", "relay_name": ""}).Error; err != nil {
						t.Fatal(err)
					}
				}
				out, _, err := NewSubClashService(NewSubService("{{INBOUND}}")).GetClash("s1", "example.com")
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(out, "__xui_") {
					t.Fatal("metadata leaked")
				}
				var config struct {
					Proxies []struct {
						Name   string `yaml:"name"`
						Dialer string `yaml:"dialer-proxy"`
					} `yaml:"proxies"`
					Groups []struct {
						Proxies []string `yaml:"proxies"`
					} `yaml:"proxy-groups"`
				}
				if err := yaml.Unmarshal([]byte(out), &config); err != nil {
					t.Fatal(err)
				}
				direct, via := "新加坡-家宽", "Home B"
				if !renamed {
					direct, via = "新加坡-家宽", "新加坡-家宽（中转·奶爸）"
				}
				names := map[string]string{}
				for _, p := range config.Proxies {
					names[p.Name] = p.Dialer
				}
				if len(names) != 4 || names[via] != "中转入口-1-奶爸" {
					t.Fatalf("bad renamed route: %s", out)
				}
				if dialer, ok := names[direct]; !ok || dialer != "" {
					t.Fatalf("bad direct variant: %s", out)
				}
				if len(config.Groups) != 1 || len(config.Groups[0].Proxies) == 0 || config.Groups[0].Proxies[0] != via {
					t.Fatalf("AI default lost identity after rename: %s", out)
				}
			}
		})
	}
}

func TestMultipleChainsShareOneDirectAndSelectIndependentRoute(t *testing.T) {
	seedSubDB(t)
	db := database.GetDB()
	relayA := seedSubInbound(t, "s1", "Relay A", 4701, 1, tcpStream)
	relayB := seedSubInbound(t, "s1", "Relay B", 4702, 2, tcpStream)
	target := seedSubInbound(t, "s1", "Singapore", 4703, 3, tcpStream)
	a := model.ProxyChain{TargetInboundId: target.Id, RelayInboundId: relayA.Id, RelayName: "Via A", Enabled: true}
	b := model.ProxyChain{TargetInboundId: target.Id, RelayInboundId: relayB.Id, RelayName: "Via B", Enabled: true}
	for _, c := range []*model.ProxyChain{&a, &b} {
		if err := db.Create(c).Error; err != nil {
			t.Fatal(err)
		}
	}
	tpl := seedRuleTemplate(t, "multi", "proxy-groups:\n  - name: Choice\n    type: select\n    proxies: [__PROXY_NODES__]\nrules: [MATCH,Choice]\n", true)
	svc := NewSubClashService(NewSubService("{{INBOUND}}"))
	check := func(want map[string]string, groupNames []string) {
		t.Helper()
		out, _, err := svc.GetClash("s1", "example.com")
		if err != nil {
			t.Fatal(err)
		}
		var cfg struct {
			Proxies []struct {
				Name   string `yaml:"name"`
				Dialer string `yaml:"dialer-proxy"`
			} `yaml:"proxies"`
			Groups []struct {
				Proxies []string `yaml:"proxies"`
			} `yaml:"proxy-groups"`
		}
		if err := yaml.Unmarshal([]byte(out), &cfg); err != nil {
			t.Fatal(err)
		}
		extra := 0
		for _, v := range want {
			if v != "" {
				extra++
			}
		}
		if len(cfg.Proxies) != len(want)+extra {
			t.Fatalf("proxy count = %d, want %d", len(cfg.Proxies), len(want))
		}
		for _, p := range cfg.Proxies {
			if strings.HasPrefix(p.Name, "中转入口-") {
				continue
			}
			d, ok := want[p.Name]
			if d != "" {
				id := a.Id
				if d == "Relay B" {
					id = b.Id
				}
				d = fmt.Sprintf("中转入口-%d-%s", id, d)
			}
			if !ok || d != p.Dialer {
				t.Fatalf("unexpected route %s via %s", p.Name, p.Dialer)
			}
		}
		if groupNames != nil {
			assertStrings(t, "selected routes", cfg.Groups[0].Proxies, groupNames)
		}
	}
	check(map[string]string{"Relay A": "", "Relay B": "", "Singapore": "", "Via A": "Relay A", "Via B": "Relay B"}, nil)
	putS1OnPlan(t, tpl)
	key := model.PlanChainKey(target.Id, b.Id)
	keys, _ := json.Marshal([]string{model.PlanNodeKey(relayA.Id, false), model.PlanNodeKey(relayB.Id, false), key})
	groups, _ := json.Marshal([]map[string]any{{"name": "Choice", "inboundIds": []int{target.Id}, "nodeKeys": []string{key}}})
	if err := db.Model(&model.Plan{}).Where("name = ?", "Plan").Updates(map[string]any{"node_keys": string(keys), "proxy_groups": string(groups)}).Error; err != nil {
		t.Fatal(err)
	}
	check(map[string]string{"Relay A": "", "Relay B": "", "Via B": "Relay B"}, []string{"Via B"})
}

func TestChainDialerUsesFinalDeduplicatedName(t *testing.T) {
	proxies := []map[string]any{
		{"name": "Same", clashPlanNodeKey: "1:direct"},
		{"name": "Same", clashPlanNodeKey: "2:direct"},
		{"name": "Custom route", clashPlanNodeKey: "3:relay", clashRelayInboundKey: 2, "dialer-proxy": "old name"},
	}
	ensureUniqueProxyNames(proxies)
	if err := resolveChainProxyNames(proxies); err != nil {
		t.Fatal(err)
	}
	if proxies[2]["dialer-proxy"] != "Same-2" {
		t.Fatalf("wrong relay: %+v", proxies)
	}
}
