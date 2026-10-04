package sub

import (
	"encoding/json"
	"strings"
	"testing"

	yaml "github.com/goccy/go-yaml"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestPlanNodeVariantsRenderOnlySelectedVersions(t *testing.T) {
	seedSubDB(t)
	db := database.GetDB()
	relay := seedSubInbound(t, "s1", "奶爸", 4501, 1, tcpStream)
	target := seedSubInbound(t, "s1", "新加坡-家宽", 4502, 2, tcpStream)
	chain := model.ProxyChain{TargetInboundId: target.Id, RelayInboundId: relay.Id, Enabled: true}
	if err := db.Create(&chain).Error; err != nil {
		t.Fatal(err)
	}
	tpl := seedRuleTemplate(t, "variants", "proxy-groups:\n  - name: 🤖 AI 服务\n    type: select\n    proxies: [__PROXY_NODES__]\nrules: [MATCH,🤖 AI 服务]\n", false)
	putS1OnPlan(t, tpl)
	directKey, relayKey := model.PlanNodeKey(target.Id, false), model.PlanChainKey(target.Id, chain.Id)
	for _, tt := range []struct {
		name string
		keys []string
		want []string
	}{
		{"direct", []string{directKey}, []string{"新加坡-家宽"}},
		{"relay", []string{relayKey}, []string{"新加坡-家宽（中转·奶爸）"}},
		{"both", []string{directKey, relayKey}, []string{"新加坡-家宽", "新加坡-家宽（中转·奶爸）"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			keys, _ := json.Marshal(append([]string{model.PlanNodeKey(relay.Id, false)}, tt.keys...))
			groups, _ := json.Marshal([]map[string]any{{"name": "🤖 AI 服务", "inboundIds": []int{target.Id}, "nodeKeys": tt.keys}})
			if err := db.Model(&model.Plan{}).Where("name = ?", "Plan").Updates(map[string]any{"node_keys": string(keys), "proxy_groups": string(groups)}).Error; err != nil {
				t.Fatal(err)
			}
			out, _, err := NewSubClashService(NewSubService("{{INBOUND}}")).GetClash("s1", "example.com")
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out, "__xui_") {
				t.Fatal("internal selection metadata leaked into subscription")
			}
			var config struct {
				Proxies []struct {
					Name   string `yaml:"name"`
					Dialer string `yaml:"dialer-proxy"`
				} `yaml:"proxies"`
				Groups []struct {
					Name    string   `yaml:"name"`
					Proxies []string `yaml:"proxies"`
				} `yaml:"proxy-groups"`
			}
			if err := yaml.Unmarshal([]byte(out), &config); err != nil {
				t.Fatal(err)
			}
			extra := 0
			for _, key := range tt.keys {
				if key == relayKey {
					extra++
				}
			}
			if len(config.Proxies) != len(tt.want)+1+extra {
				t.Fatalf("wrong proxies: %s", out)
			}
			for _, p := range config.Proxies {
				if strings.Contains(p.Name, "（中转") && p.Dialer != "中转入口-1-奶爸" {
					t.Fatalf("bad relay reference: %+v", p)
				}
			}
			if len(config.Groups) != 1 {
				t.Fatalf("unexpected groups: %s", out)
			}
			assertStrings(t, "AI variants", config.Groups[0].Proxies, tt.want)
		})
	}
}

func TestPlanOrderPreservesTemplateStrategyPositions(t *testing.T) {
	seedSubDB(t)
	a := seedSubInbound(t, "s1", "洛杉矶", 4701, 1, tcpStream)
	b := seedSubInbound(t, "s1", "新加坡-家宽", 4702, 2, tcpStream)
	tpl := seedRuleTemplate(t, "ordered", `proxy-groups:
  - name: 全球直连
    type: select
    proxies: [DIRECT]
  - name: 节点选择
    type: select
    proxies: [__PROXY_NODES__]
  - name: 🤖 AI 服务
    type: select
    proxies: [__PROXY_NODES__, 全球直连]
  - name: 国内媒体
    type: select
    proxies: [全球直连, 节点选择, __PROXY_NODES__]
rules: ["MATCH,节点选择"]
`, false)
	putS1OnPlan(t, tpl)
	ka, kb := model.PlanNodeKey(a.Id, false), model.PlanNodeKey(b.Id, false)
	keys, _ := json.Marshal([]string{kb, ka})
	for _, order := range [][]string{{ka, kb}, {kb, ka}} {
		groups, _ := json.Marshal([]map[string]any{{"name": "🤖 AI 服务", "nodeKeys": order}, {"name": "国内媒体", "nodeKeys": order}})
		if err := database.GetDB().Model(&model.Plan{}).Where("name = ?", "Plan").Updates(map[string]any{"node_keys": string(keys), "proxy_groups": string(groups)}).Error; err != nil {
			t.Fatal(err)
		}
		out, _, err := NewSubClashService(NewSubService("{{INBOUND}}")).GetClash("s1", "example.com")
		if err != nil {
			t.Fatal(err)
		}
		var cfg struct {
			Proxies []struct {
				Name string `yaml:"name"`
			} `yaml:"proxies"`
			Groups []struct {
				Name    string   `yaml:"name"`
				Proxies []string `yaml:"proxies"`
			} `yaml:"proxy-groups"`
			Rules []string `yaml:"rules"`
		}
		if err := yaml.Unmarshal([]byte(out), &cfg); err != nil {
			t.Fatal(err)
		}
		names := []string{}
		for _, p := range cfg.Proxies {
			names = append(names, p.Name)
		}
		assertStrings(t, "subscription order", names, []string{"新加坡-家宽", "洛杉矶"})
		wanted := []string{"洛杉矶", "新加坡-家宽"}
		if order[0] == kb {
			wanted = []string{"新加坡-家宽", "洛杉矶"}
		}
		for _, g := range cfg.Groups {
			switch g.Name {
			case "🤖 AI 服务":
				assertStrings(t, "AI explicit order", g.Proxies, append(append([]string{}, wanted...), "全球直连"))
			case "国内媒体":
				assertStrings(t, "template fixed priority", g.Proxies, append([]string{"全球直连", "节点选择"}, wanted...))
			}
		}
		assertStrings(t, "rules unchanged", cfg.Rules, []string{"MATCH,节点选择"})
	}
}
