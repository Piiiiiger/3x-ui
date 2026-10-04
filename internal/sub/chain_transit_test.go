package sub

import (
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"testing"
)

func TestTransitRelayUsesSeparateCredentialWithoutChangingVisibleChoices(t *testing.T) {
	relay := map[string]any{"name": "香港", "type": "vless", "uuid": "original-secret", clashPlanNodeKey: "1:direct", clashPlanInboundIDKey: 1}
	target := map[string]any{"name": "家宽", "type": "vless", "uuid": "exit-secret", "dialer-proxy": "香港", "__xui_chain_id": 7, clashPlanNodeKey: "2:relay:7", clashPlanInboundIDKey: 2}
	proxies, err := attachChainTransitProxies([]map[string]any{relay, target})
	if err != nil {
		t.Fatal(err)
	}
	if len(proxies) != 3 {
		t.Fatal("missing isolated relay")
	}
	expected, _ := model.ChainTransitCredential("original-secret", 7)
	if proxies[2]["uuid"] != expected || target["dialer-proxy"] != proxies[2]["name"] {
		t.Fatal("client/server transit credential mismatch")
	}
	if relay["uuid"] != "original-secret" || target["uuid"] != "exit-secret" {
		t.Fatal("changed direct or billable exit credential")
	}
	assertStrings(t, "no internal relay in choices", clashProxyNamesForGroups(proxies), []string{"香港", "家宽"})
	cfg := map[string]any{"proxies": proxies}
	clearPlanMetadata(cfg)
	for _, p := range proxies {
		for _, k := range []string{"__xui_chain_id", "__xui_transit", clashPlanNodeKey} {
			if _, ok := p[k]; ok {
				t.Fatalf("metadata leaked: %s", k)
			}
		}
	}
}
