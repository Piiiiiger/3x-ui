package service

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func seedAgentNodeRow(t *testing.T, name string) *model.Node {
	t.Helper()
	n := &model.Node{Name: name, Kind: model.NodeKindAgent, Address: "203.0.113.7", Enable: true, Status: "online"}
	if err := database.GetDB().Create(n).Error; err != nil {
		t.Fatalf("seed agent node %s: %v", name, err)
	}
	return n
}

func seedNodeInbound(t *testing.T, nodeID *int, tag string, port int, protocol model.Protocol, enable bool, clients []model.Client) *model.Inbound {
	t.Helper()
	in := &model.Inbound{
		Tag:      tag,
		Enable:   enable,
		Port:     port,
		Protocol: protocol,
		Settings: `{"clients":[],"decryption":"none"}`,
		NodeID:   nodeID,
	}
	if err := database.GetDB().Create(in).Error; err != nil {
		t.Fatalf("seed inbound %s: %v", tag, err)
	}
	if len(clients) > 0 {
		if err := (&ClientService{}).SyncInbound(nil, in.Id, clients); err != nil {
			t.Fatalf("attach clients to %s: %v", tag, err)
		}
	}
	return in
}

func inboundClientEmails(t *testing.T, cfg *xray.Config, tag string) []string {
	t.Helper()
	for i := range cfg.InboundConfigs {
		if cfg.InboundConfigs[i].Tag != tag {
			continue
		}
		var settings struct {
			Clients []struct {
				Email string `json:"email"`
			} `json:"clients"`
		}
		if err := json.Unmarshal(cfg.InboundConfigs[i].Settings, &settings); err != nil {
			t.Fatalf("settings of %s: %v", tag, err)
		}
		emails := make([]string, 0, len(settings.Clients))
		for _, c := range settings.Clients {
			emails = append(emails, c.Email)
		}
		slices.Sort(emails)
		return emails
	}
	t.Fatalf("inbound %s missing from the config", tag)
	return nil
}

func configInboundTags(cfg *xray.Config) []string {
	tags := make([]string, 0, len(cfg.InboundConfigs))
	for i := range cfg.InboundConfigs {
		tags = append(tags, cfg.InboundConfigs[i].Tag)
	}
	slices.Sort(tags)
	return tags
}

func TestGetAgentXrayConfig_HoldsOnlyThatAgentsEnabledInbounds(t *testing.T) {
	setupSettingTestDB(t)
	a := seedAgentNodeRow(t, "edge-hk")
	b := seedAgentNodeRow(t, "edge-us")
	alice := model.Client{Email: "alice", ID: "11111111-1111-1111-1111-111111111111", Enable: true}
	bob := model.Client{Email: "bob", ID: "22222222-2222-2222-2222-222222222222", Enable: true}

	seedNodeInbound(t, nil, "in-443-tcp", 443, model.VLESS, true, []model.Client{alice})
	seedNodeInbound(t, &a.Id, "n1-in-81-tcp", 81, model.VLESS, true, []model.Client{alice, bob})
	seedNodeInbound(t, &a.Id, "n1-in-82-tcp", 82, model.VLESS, false, []model.Client{alice})
	seedNodeInbound(t, &a.Id, "n1-mtproto-8443", 8443, model.MTProto, true, nil)
	seedNodeInbound(t, &b.Id, "n2-in-81-tcp", 81, model.VLESS, true, []model.Client{bob})
	disableClients(t, "bob")

	cfg, err := (&XrayService{}).GetAgentXrayConfig(a.Id)
	if err != nil {
		t.Fatalf("GetAgentXrayConfig: %v", err)
	}
	if got, want := configInboundTags(cfg), []string{"api", "n1-in-81-tcp"}; !slices.Equal(got, want) {
		t.Fatalf("inbounds = %v, want %v (the API inbound and this agent's enabled Xray inbounds only)", got, want)
	}
	if got := inboundClientEmails(t, cfg, "n1-in-81-tcp"); !slices.Equal(got, []string{"alice"}) {
		t.Fatalf("clients = %v, want only the enabled alice", got)
	}
	if len(cfg.Stats) == 0 || len(cfg.API) == 0 {
		t.Fatalf("the agent reports traffic through the core's stats API, got stats=%s api=%s", cfg.Stats, cfg.API)
	}
}

// client_traffics has one row per email, created on whichever inbound came first;
// a quota hit on the panel's own inbound must still cut the user off an agent.
func TestGetAgentXrayConfig_DropsAClientDepletedElsewhere(t *testing.T) {
	setupSettingTestDB(t)
	a := seedAgentNodeRow(t, "edge-hk")
	carol := model.Client{Email: "carol", ID: "33333333-3333-3333-3333-333333333333", Enable: true}
	local := seedNodeInbound(t, nil, "in-443-tcp", 443, model.VLESS, true, []model.Client{carol})
	seedNodeInbound(t, &a.Id, "n1-in-81-tcp", 81, model.VLESS, true, []model.Client{carol})
	row := &xray.ClientTraffic{InboundId: local.Id, Email: "carol", Enable: false, Total: 100, Up: 60, Down: 60}
	if err := database.GetDB().Create(row).Error; err != nil {
		t.Fatal(err)
	}

	cfg, err := (&XrayService{}).GetAgentXrayConfig(a.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got := inboundClientEmails(t, cfg, "n1-in-81-tcp"); len(got) != 0 {
		t.Fatalf("clients = %v; carol is depleted and must not reach the agent", got)
	}
}

// The panel's own egress bridge opens a SOCKS inbound; on an agent it would only
// be an open port nobody uses.
func TestGetAgentXrayConfig_LeavesOutThePanelEgress(t *testing.T) {
	setupSettingTestDB(t)
	a := seedAgentNodeRow(t, "edge-hk")
	seedNodeInbound(t, &a.Id, "n1-in-81-tcp", 81, model.VLESS, true, nil)
	if err := (&SettingService{}).SetPanelOutbound("direct"); err != nil {
		t.Fatal(err)
	}

	panelCfg, err := (&XrayService{}).GetXrayConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(configInboundTags(panelCfg), PanelEgressInboundTag) {
		t.Fatal("control: the panel's own config should carry the egress bridge")
	}
	cfg, err := (&XrayService{}).GetAgentXrayConfig(a.Id)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(configInboundTags(cfg), PanelEgressInboundTag) {
		t.Fatalf("agent config carries the panel egress bridge: %v", configInboundTags(cfg))
	}
}
