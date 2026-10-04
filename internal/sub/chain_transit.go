package sub

import (
	"fmt"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"maps"
)

// Each dialer has a separate relay credential. It stays in proxies solely as a
// dependency; direct nodes and selectable route names retain their normal identity.
func attachChainTransitProxies(proxies []map[string]any) ([]map[string]any, error) {
	byName := map[string]map[string]any{}
	used := map[string]bool{}
	for _, p := range proxies {
		n, _ := p["name"].(string)
		byName[n] = p
		used[n] = true
	}
	aliases := map[string]string{}
	out := append([]map[string]any{}, proxies...)
	for _, p := range proxies {
		id, ok := p["__xui_chain_id"].(int)
		if !ok {
			continue
		}
		relayName, _ := p["dialer-proxy"].(string)
		relay := byName[relayName]
		if relay == nil {
			return nil, fmt.Errorf("missing chain relay %s", relayName)
		}
		field := "uuid"
		switch relay["type"] {
		case "vless", "vmess":
		case "trojan":
			field = "password"
		default:
			return nil, fmt.Errorf("中转协议 %v 尚不支持独立中转计量", relay["type"])
		}
		secret, _ := relay[field].(string)
		if secret == "" {
			return nil, fmt.Errorf("missing relay credential")
		}
		credential, _ := model.ChainTransitCredential(secret, id)
		key := relayName + ":" + credential
		name := aliases[key]
		if name == "" {
			base := fmt.Sprintf("中转入口-%d-%s", id, relayName)
			name = base
			for i := 2; used[name]; i++ {
				name = fmt.Sprintf("%s-%d", base, i)
			}
			used[name] = true
			aliases[key] = name
			clone := maps.Clone(relay)
			clone["name"] = name
			clone[field] = credential
			clone["__xui_transit"] = true
			delete(clone, clashPlanNodeKey)
			delete(clone, clashPlanInboundIDKey)
			delete(clone, clashAIRankKey)
			out = append(out, clone)
		}
		p["dialer-proxy"] = name
	}
	return out, nil
}
