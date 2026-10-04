package service

import (
	"encoding/json"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"testing"
)

func TestChainTransitCountsExitOnceAndDirectRelaySeparately(t *testing.T) {
	setupSettingTestDB(t)
	a, b := seedAgentNodeRow(t, "transit"), seedAgentNodeRow(t, "exit")
	user := model.Client{Email: "transit-test", ID: "11111111-1111-4111-8111-111111111111", Enable: true}
	relay := seedNodeInbound(t, &a.Id, "transit-in", 81, model.VLESS, true, []model.Client{user})
	target := seedNodeInbound(t, &b.Id, "exit-in", 443, model.VLESS, true, []model.Client{user})
	seedClientTrafficRow(t, target.Id, user.Email, 0)
	chain := model.ProxyChain{TargetInboundId: target.Id, RelayInboundId: relay.Id, Enabled: true}
	if err := database.GetDB().Create(&chain).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Model(&model.Inbound{}).Where("id = ?", target.Id).Updates(map[string]any{"share_addr_strategy": "custom", "share_addr": "192.0.2.1", "share_port": 2443}).Error; err != nil {
		t.Fatal(err)
	}
	cfg, err := (&XrayService{}).GetAgentXrayConfig(a.Id)
	if err != nil {
		t.Fatal(err)
	}
	credential, alias := model.ChainTransitCredential(user.ID, chain.Id)
	found := false
	for _, ib := range cfg.InboundConfigs {
		if ib.Tag != relay.Tag {
			continue
		}
		var s struct {
			Clients []struct {
				ID    string `json:"id"`
				Email string `json:"email"`
			}
		}
		if err := json.Unmarshal(ib.Settings, &s); err != nil {
			t.Fatal(err)
		}
		for _, c := range s.Clients {
			if c.Email == alias && c.ID == credential {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("relay identity not installed")
	}
	var routing struct{ Rules []map[string]any }
	if err := json.Unmarshal(cfg.RouterConfig, &routing); err != nil {
		t.Fatal(err)
	}
	if len(routing.Rules) < 2 || routing.Rules[0]["port"] != "2443" || routing.Rules[0]["outboundTag"] != "chain-transit-direct" || routing.Rules[1]["outboundTag"] != "chain-transit-block" {
		t.Fatalf("transit is not destination restricted: %s", cfg.RouterConfig)
	}
	agentReport(t, a.Id, "a", 1, relay.Tag, alias, 10, 100)
	agentReport(t, b.Id, "b", 1, target.Tag, user.Email, 10, 100)
	agentReport(t, a.Id, "a", 2, relay.Tag, user.Email, 3, 20)
	if ct := clientUsage(t, user.Email); ct.Up != 13 || ct.Down != 120 {
		t.Fatalf("double-billed transit or dropped direct use: %+v", ct)
	}
	if up, down := inboundUsage(t, relay.Tag); up != 13 || down != 120 {
		t.Fatalf("lost physical relay usage: %d %d", up, down)
	}
	// Repeated reports cannot double count, including transit reports.
	agentReport(t, a.Id, "a", 2, relay.Tag, user.Email, 3, 20)
	if ct := clientUsage(t, user.Email); ct.Up != 13 || ct.Down != 120 {
		t.Fatal("replayed report was counted")
	}
	// Removing access removes the internal identity too.
	if err := database.GetDB().Where("inbound_id = ?", target.Id).Delete(&model.ClientInbound{}).Error; err != nil {
		t.Fatal(err)
	}
	cfg, err = (&XrayService{}).GetAgentXrayConfig(a.Id)
	if err != nil {
		t.Fatal(err)
	}
	for _, ib := range cfg.InboundConfigs {
		var s struct {
			Clients []struct {
				Email string `json:"email"`
			}
		}
		_ = json.Unmarshal(ib.Settings, &s)
		for _, c := range s.Clients {
			if c.Email == alias {
				t.Fatal("revoked transit credential survived")
			}
		}
	}
}
