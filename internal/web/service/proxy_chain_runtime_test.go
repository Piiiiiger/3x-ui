package service

import (
	"encoding/json"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestChainRelaySniffingSurvivesEditsAndBothRuntimePaths(t *testing.T) {
	setupBulkDB(t)
	db := database.GetDB()
	ib := mkInbound(t, 30401, model.VLESS, `{"clients":[]}`)
	ib.Sniffing = `{"enabled":true,"destOverride":["tls","http"],"routeOnly":false,"metadataOnly":false}`
	chain := model.ProxyChain{TargetInboundId: ib.Id + 1, RelayInboundId: ib.Id, Enabled: true}
	if err := db.Create(&chain).Error; err != nil {
		t.Fatal(err)
	}
	assertSafe := func(raw string) {
		t.Helper()
		var sniff map[string]any
		if err := json.Unmarshal([]byte(raw), &sniff); err != nil {
			t.Fatal(err)
		}
		if sniff["routeOnly"] != true || sniff["enabled"] != true || sniff["metadataOnly"] != false {
			t.Fatalf("unexpected sniffing: %s", raw)
		}
	}
	svc := InboundService{}
	for _, build := range []func() (*model.Inbound, error){func() (*model.Inbound, error) { return svc.buildInboundForNodePush(db, ib) }, func() (*model.Inbound, error) { return svc.buildInboundForLocalRuntime(db, ib) }} {
		out, err := build()
		if err != nil {
			t.Fatal(err)
		}
		assertSafe(out.Sniffing)
	}
	out, err := (&XrayService{}).buildInboundConfig(ib)
	if err != nil {
		t.Fatal(err)
	}
	assertSafe(string(out.Sniffing))
	var original map[string]any
	_ = json.Unmarshal([]byte(ib.Sniffing), &original)
	if original["routeOnly"] != false {
		t.Fatal("mutated stored/user config")
	}
	if err := db.Model(&chain).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	built, err := svc.buildInboundForNodePush(db, ib)
	if err != nil {
		t.Fatal(err)
	}
	if built.Sniffing != ib.Sniffing {
		t.Fatal("inactive chain should not override sniffing")
	}
}
