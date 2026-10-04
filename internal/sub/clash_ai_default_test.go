package sub

import "testing"

func TestAIGroupDefaultsToAvailableSingaporeResidential(t *testing.T) {
	const direct = "新加坡-家宽（直连）"
	const relay = "新加坡-家宽（中转·香港-Neburst）"
	for _, tt := range []struct {
		name, group, kind        string
		available, members, want []string
	}{
		{"chain preferred", "🤖 AI 服务", "select", []string{"香港", direct, relay}, []string{"香港", direct, relay}, []string{relay, "香港", direct}},
		{"direct only", "AI服务", "select", []string{"香港", direct}, []string{"香港", direct}, []string{direct, "香港"}},
		{"no residential", "🤖 AI 服务", "select", []string{"香港", "新加坡-Titan"}, []string{"香港", "新加坡-Titan"}, []string{"香港", "新加坡-Titan"}},
		{"missing proxy", "🤖 AI 服务", "select", []string{"香港"}, []string{"香港", direct}, []string{"香港", direct}},
		{"outside group assignment", "🤖 AI 服务", "select", []string{"香港", direct}, []string{"香港"}, []string{"香港"}},
		{"other group", "🔰 节点选择", "select", []string{"香港", direct}, []string{"香港", direct}, []string{"香港", direct}},
		{"automatic group", "🤖 AI 服务", "url-test", []string{"香港", direct}, []string{"香港", direct}, []string{"香港", direct}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			proxies := make([]map[string]any, 0, len(tt.available))
			for _, name := range tt.available {
				proxies = append(proxies, map[string]any{"name": name, "type": "vless"})
			}
			group := map[string]any{"name": tt.group, "type": tt.kind, "proxies": stringsToAny(tt.members)}
			config := map[string]any{"proxies": proxies, "proxy-groups": []map[string]any{group}}
			preferSingaporeResidentialForAI(config)
			members, _ := asAnySlice(group["proxies"])
			got := make([]string, len(members))
			for i, member := range members {
				got[i] = member.(string)
			}
			assertStrings(t, "AI default", got, tt.want)
		})
	}
}
