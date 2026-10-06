package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/agentproto"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/snell"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// snellHost seeds an agent host with one Snell inbound the given users can reach.
func snellHost(t *testing.T, emails ...string) (*model.Node, *model.Inbound) {
	t.Helper()
	setupSettingTestDB(t)
	n := seedAgentNodeRow(t, "snell-host")
	clients := make([]model.Client, 0, len(emails))
	for _, e := range emails {
		clients = append(clients, model.Client{Email: e, Enable: true})
	}
	ib := seedNodeInbound(t, &n.Id, "n9-in-26163-tcpudp", 26163, model.Snell, true, clients)
	db := database.GetDB()
	if err := db.Model(ib).Update("settings", snellTestSettings).Error; err != nil {
		t.Fatal(err)
	}
	for _, e := range emails {
		if err := db.Create(&xray.ClientTraffic{InboundId: ib.Id, Email: e, Enable: true}).Error; err != nil {
			t.Fatal(err)
		}
	}
	return n, ib
}

func nodeUsageOf(t *testing.T, nodeID int, email string) [2]int64 {
	t.Helper()
	var row struct{ Up, Down int64 }
	database.GetDB().Raw("SELECT up, down FROM node_client_traffics WHERE node_id = ? AND email = ?", nodeID, email).Scan(&row)
	return [2]int64{row.Up, row.Down}
}

// A Snell inbound only one user can reach carries that user's traffic: it counts
// toward their usage and quota, beside what they moved through Xray, once.
func TestSnellTrafficIsItsOnlyUsersUsage(t *testing.T) {
	n, ib := snellHost(t, "solo")
	report := &agentproto.Traffic{
		Instance: "i1", Seq: 1,
		Inbounds: []agentproto.Counter{{Name: ib.Tag, Up: 100, Down: 900}},
		Clients:  []agentproto.Counter{{Name: "solo", Up: 5, Down: 7}},
	}
	for range 2 {
		if err := (&AgentService{}).HandleTraffic(n.Id, report); err != nil {
			t.Fatal(err)
		}
	}

	if up, down := inboundUsage(t, ib.Tag); up != 100 || down != 900 {
		t.Errorf("Snell inbound = %d/%d, want 100/900", up, down)
	}
	if ct := clientUsage(t, "solo"); ct.Up != 105 || ct.Down != 907 {
		t.Errorf("solo's usage = %d/%d, want the Snell 100/900 on top of the Xray 5/7", ct.Up, ct.Down)
	}
	if got := nodeUsageOf(t, n.Id, "solo"); got != [2]int64{105, 907} {
		t.Errorf("solo's usage on the host = %v, want 105/907", got)
	}

	snellOnly := &agentproto.Traffic{Instance: "i1", Seq: 2, Inbounds: []agentproto.Counter{{Name: ib.Tag, Up: 10, Down: 20}}}
	if err := (&AgentService{}).HandleTraffic(n.Id, snellOnly); err != nil {
		t.Fatal(err)
	}
	if ct := clientUsage(t, "solo"); ct.Up != 115 || ct.Down != 927 {
		t.Errorf("after a report with Snell traffic only, solo's usage = %d/%d, want 115/927", ct.Up, ct.Down)
	}
}

// A Snell inbound several users share cannot say whose bytes they were: they
// count for the server, for none of the users.
func TestSharedSnellTrafficCountsForTheServerOnly(t *testing.T) {
	n, ib := snellHost(t, "ann", "ben")
	report := &agentproto.Traffic{Instance: "i1", Seq: 1, Inbounds: []agentproto.Counter{{Name: ib.Tag, Up: 100, Down: 900}}}
	if err := (&AgentService{}).HandleTraffic(n.Id, report); err != nil {
		t.Fatal(err)
	}
	if up, down := inboundUsage(t, ib.Tag); up != 100 || down != 900 {
		t.Errorf("Snell inbound = %d/%d, want 100/900", up, down)
	}
	for _, e := range []string{"ann", "ben"} {
		if got := usageOf(t, e); got != 0 {
			t.Errorf("%s was charged %d bytes for a shared inbound", e, got)
		}
	}
}

func snellServed(t *testing.T, nodeID int) bool {
	t.Helper()
	raw, _, err := (&AgentService{}).AgentConfig(nodeID)
	if err != nil {
		t.Fatal(err)
	}
	_, instances, err := snell.Split(raw)
	if err != nil {
		t.Fatal(err)
	}
	return len(instances) == 1
}

// A Snell inbound with one user serves only while that user may: out of traffic,
// expired or disabled, it stops; one that is shared keeps serving.
func TestSnellServesOnlyWhileItsOnlyUserMay(t *testing.T) {
	n, ib := snellHost(t, "solo")
	db := database.GetDB()
	if !snellServed(t, n.Id) {
		t.Fatal("an active user's Snell is not served")
	}
	db.Model(&xray.ClientTraffic{}).Where("email = ?", "solo").Update("enable", false)
	if snellServed(t, n.Id) {
		t.Fatal("served after its user ran out of traffic or time")
	}
	db.Model(&xray.ClientTraffic{}).Where("email = ?", "solo").Update("enable", true)
	db.Model(&model.ClientRecord{}).Where("email = ?", "solo").Update("enable", false)
	if snellServed(t, n.Id) {
		t.Fatal("served after its user was disabled")
	}

	if err := (&ClientService{}).SyncInbound(nil, ib.Id, []model.Client{{Email: "solo", Enable: false}, {Email: "mate", Enable: true}}); err != nil {
		t.Fatal(err)
	}
	if !snellServed(t, n.Id) {
		t.Fatal("a shared Snell stopped because one of its users is out")
	}
}

// The inbound list says a Snell inbound is suspended for its only user.
func TestSnellStatusSaysItIsSuspendedForItsUser(t *testing.T) {
	_, ib := snellHost(t, "solo")
	database.GetDB().Model(&xray.ClientTraffic{}).Where("email = ?", "solo").Update("enable", false)
	var row model.Inbound
	if err := database.GetDB().First(&row, ib.Id).Error; err != nil {
		t.Fatal(err)
	}
	(&InboundService{}).annotateSnellStatus([]*model.Inbound{&row})
	if row.RuntimeState != "suspended" {
		t.Fatalf("state = %q, want suspended", row.RuntimeState)
	}
}
