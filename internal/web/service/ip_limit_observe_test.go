package service

import (
	"net/netip"
	"slices"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/agentproto"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestAgentObservationsReadOnlyFreshReports(t *testing.T) {
	setupIpLimitTest(t)
	hub := useAgentHub(t)
	n := seedAgentNodeRow(t, "edge")
	agent := connectFakeAgent(t, hub, n.Id, agentproto.Hello{AgentVersion: "v-test"})
	status := agentproto.Status{IPs: map[string][]agentproto.IPEntry{"nia": {{IP: "198.51.100.100", Timestamp: 1_799_999_990}}}}
	agent.Send(agentproto.Message{Type: agentproto.TypeStatus, Status: &status})

	var got []IpObservation
	deadline := time.Now().Add(3 * time.Second)
	for len(got) == 0 && time.Now().Before(deadline) {
		got = (&IpLimitService{}).AgentObservations(time.Now())
		time.Sleep(5 * time.Millisecond)
	}
	want := []IpObservation{{Email: "nia", IP: "198.51.100.100", LastSeen: 1_799_999_990, Server: n.Id}}
	if !slices.Equal(got, want) {
		t.Fatalf("observations = %+v, want %+v", got, want)
	}
	if late := (&IpLimitService{}).AgentObservations(time.Now().Add(time.Minute)); len(late) != 0 {
		t.Fatalf("a report a minute old still counted: %+v", late)
	}
}

func TestRecordLiveFoldsRelayIdentitiesIntoTheirOwner(t *testing.T) {
	setupIpLimitTest(t)
	relayNode, exitNode := seedAgentNodeRow(t, "relay"), seedAgentNodeRow(t, "exit")
	user := model.Client{Email: "oli", ID: uuidFor("oli"), LimitIP: 1, Enable: true}
	relay := seedNodeInbound(t, &relayNode.Id, "relay-in", 81, model.VLESS, true, []model.Client{user})
	target := seedNodeInbound(t, &exitNode.Id, "exit-in", 443, model.VLESS, true, []model.Client{user})
	chain := model.ProxyChain{TargetInboundId: target.Id, RelayInboundId: relay.Id, Enabled: true}
	if err := database.GetDB().Create(&chain).Error; err != nil {
		t.Fatal(err)
	}
	_, alias := model.ChainTransitCredential(user.ID, chain.Id)
	now := time.Unix(1_800_000_000, 0)

	observed := []IpObservation{
		{Email: alias, IP: "::ffff:198.51.100.110", LastSeen: now.Unix() - 7200},
		{Email: "oli", IP: "198.51.100.111", LastSeen: now.Unix() - 5},
	}
	if err := (&IpLimitService{}).recordLive("node:7", observed, now); err != nil {
		t.Fatal(err)
	}
	var rows []model.InboundClientIps
	if err := database.GetDB().Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ClientEmail != "oli" ||
		rows[0].Ips != `[{"ip":"198.51.100.110","timestamp":1800000000},{"ip":"198.51.100.111","timestamp":1800000000}]` &&
			rows[0].Ips != `[{"ip":"198.51.100.111","timestamp":1800000000},{"ip":"198.51.100.110","timestamp":1800000000}]` {
		t.Fatalf("IP log rows = %+v; want the relayed address under oli, live now", rows)
	}
	var attributed []model.NodeClientIp
	if err := database.GetDB().Find(&attributed).Error; err != nil {
		t.Fatal(err)
	}
	if len(attributed) != 1 || attributed[0].NodeGuid != "node:7" || attributed[0].Email != "oli" {
		t.Fatalf("attribution rows = %+v; want one row for oli under node:7", attributed)
	}
}

func TestNoteAgentRemoteIPKeepsTheLatestPublicAddress(t *testing.T) {
	setupIpLimitTest(t)
	n := seedAgentNodeRow(t, "edge")
	svc := &NodeService{}
	for _, step := range []struct{ from, want string }{
		{"198.51.100.130", "198.51.100.130"},
		{"10.0.0.2", "198.51.100.130"},
		{"::ffff:198.51.100.131", "198.51.100.131"},
		{"not-an-ip", "198.51.100.131"},
	} {
		if err := svc.NoteAgentRemoteIP(n.Id, step.from); err != nil {
			t.Fatal(err)
		}
		var stored model.Node
		if err := database.GetDB().First(&stored, n.Id).Error; err != nil {
			t.Fatal(err)
		}
		if stored.AgentRemoteIP != step.want {
			t.Fatalf("after %q: remote ip %q, want %q", step.from, stored.AgentRemoteIP, step.want)
		}
	}
}

func TestIpLimitExemptHostsListsEveryServer(t *testing.T) {
	setupIpLimitTest(t)
	a := seedAgentNodeRow(t, "alpha")
	b := seedAgentNodeRow(t, "beta")
	if err := database.GetDB().Model(b).Updates(map[string]any{"address": "2001:db8:b::5", "agent_remote_ip": "198.51.100.150"}).Error; err != nil {
		t.Fatal(err)
	}
	ipLimitLocalAddrs = func() []netip.Addr { return []netip.Addr{netip.MustParseAddr("192.0.2.200")} }

	hosts, err := (&IpLimitService{}).ExemptHosts()
	if err != nil {
		t.Fatal(err)
	}
	want := []IpLimitExemptHost{
		{Name: "", Addresses: []string{"192.0.2.200"}},
		{Name: a.Name, Addresses: []string{"203.0.113.7"}},
		{Name: b.Name, Addresses: []string{"2001:db8:b::/64", "198.51.100.150"}},
	}
	if len(hosts) != len(want) {
		t.Fatalf("hosts = %+v, want %+v", hosts, want)
	}
	for i := range want {
		if hosts[i].Name != want[i].Name || !slices.Equal(hosts[i].Addresses, want[i].Addresses) {
			t.Fatalf("hosts = %+v, want %+v", hosts, want)
		}
	}
}
