package service

import (
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/runtime"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func seedClientTrafficRow(t *testing.T, inboundID int, email string, total int64) {
	t.Helper()
	row := &xray.ClientTraffic{InboundId: inboundID, Email: email, Enable: true, Total: total}
	if err := database.GetDB().Create(row).Error; err != nil {
		t.Fatalf("seed client traffic %s: %v", email, err)
	}
}

func inboundUsage(t *testing.T, tag string) (int64, int64) {
	t.Helper()
	var ib model.Inbound
	if err := database.GetDB().Where("tag = ?", tag).First(&ib).Error; err != nil {
		t.Fatal(err)
	}
	return ib.Up, ib.Down
}

func clientUsage(t *testing.T, email string) xray.ClientTraffic {
	t.Helper()
	var ct xray.ClientTraffic
	if err := database.GetDB().Where("email = ?", email).First(&ct).Error; err != nil {
		t.Fatal(err)
	}
	return ct
}

func agentReport(t *testing.T, nodeID int, instance string, seq int64, tag, email string, up, down int64) bool {
	t.Helper()
	applied, err := (&InboundService{}).AddAgentTraffic(nodeID, instance, seq,
		[]*xray.Traffic{{IsInbound: true, Tag: tag, Up: up, Down: down}},
		[]*xray.ClientTraffic{{Email: email, Up: up, Down: down}})
	if err != nil {
		t.Fatalf("AddAgentTraffic(%s/%d): %v", instance, seq, err)
	}
	return applied
}

// An agent resends a report whose ack was lost; counting it again would bill the
// user twice, and a reinstalled agent must start over instead of being ignored.
func TestAddAgentTraffic_AppliesEachReportOnce(t *testing.T) {
	setupSettingTestDB(t)
	a := seedAgentNodeRow(t, "edge-hk")
	alice := model.Client{Email: "alice", ID: "11111111-1111-1111-1111-111111111111", Enable: true}
	ib := seedNodeInbound(t, &a.Id, "n1-in-81-tcp", 81, model.VLESS, true, []model.Client{alice})
	seedClientTrafficRow(t, ib.Id, "alice", 0)

	if !agentReport(t, a.Id, "i1", 1, "n1-in-81-tcp", "alice", 10, 20) {
		t.Fatal("first report must apply")
	}
	if agentReport(t, a.Id, "i1", 1, "n1-in-81-tcp", "alice", 10, 20) {
		t.Fatal("a resent report must not apply again")
	}
	if !agentReport(t, a.Id, "i1", 2, "n1-in-81-tcp", "alice", 1, 2) {
		t.Fatal("the next report must apply")
	}
	if agentReport(t, a.Id, "i1", 1, "n1-in-81-tcp", "alice", 10, 20) {
		t.Fatal("an older report must not apply")
	}
	if !agentReport(t, a.Id, "i2", 1, "n1-in-81-tcp", "alice", 100, 200) {
		t.Fatal("a reinstalled agent restarts its numbering and must still be counted")
	}

	if up, down := inboundUsage(t, "n1-in-81-tcp"); up != 111 || down != 222 {
		t.Fatalf("inbound usage = %d/%d, want 111/222", up, down)
	}
	if ct := clientUsage(t, "alice"); ct.Up != 111 || ct.Down != 222 {
		t.Fatalf("client usage = %d/%d, want 111/222", ct.Up, ct.Down)
	}
	var perNode model.NodeClientTraffic
	if err := database.GetDB().Where("node_id = ? AND email = ?", a.Id, "alice").First(&perNode).Error; err != nil {
		t.Fatalf("per-server usage row: %v", err)
	}
	if perNode.Up != 111 || perNode.Down != 222 {
		t.Fatalf("per-server usage = %d/%d, want 111/222 for the breakdown", perNode.Up, perNode.Down)
	}
}

// Counters are matched by tag; an agent must not be able to add usage to an
// inbound that lives on the panel or on another server.
func TestAddAgentTraffic_IgnoresInboundsItDoesNotHost(t *testing.T) {
	setupSettingTestDB(t)
	a := seedAgentNodeRow(t, "edge-hk")
	b := seedAgentNodeRow(t, "edge-us")
	seedNodeInbound(t, nil, "in-443-tcp", 443, model.VLESS, true, nil)
	seedNodeInbound(t, &b.Id, "n2-in-81-tcp", 81, model.VLESS, true, nil)

	agentReport(t, a.Id, "i1", 1, "in-443-tcp", "nobody", 5, 5)
	agentReport(t, a.Id, "i1", 2, "n2-in-81-tcp", "nobody", 5, 5)

	if up, down := inboundUsage(t, "in-443-tcp"); up != 0 || down != 0 {
		t.Fatalf("panel inbound usage = %d/%d, want untouched", up, down)
	}
	if up, down := inboundUsage(t, "n2-in-81-tcp"); up != 0 || down != 0 {
		t.Fatalf("other agent's inbound usage = %d/%d, want untouched", up, down)
	}
}

func TestAddAgentTraffic_QuotaCutsTheClientOffTheAgent(t *testing.T) {
	setupSettingTestDB(t)
	useTestRuntimeManager(t)
	hub := runtime.NewAgentHub()
	prev := runtime.GetAgentHub()
	runtime.SetAgentHub(hub)
	t.Cleanup(func() { runtime.SetAgentHub(prev) })

	a := seedAgentNodeRow(t, "edge-hk")
	alice := model.Client{Email: "alice", ID: "11111111-1111-1111-1111-111111111111", Enable: true}
	ib := seedNodeInbound(t, &a.Id, "n1-in-81-tcp", 81, model.VLESS, true, []model.Client{alice})
	seedClientTrafficRow(t, ib.Id, "alice", 100)

	agentReport(t, a.Id, "i1", 1, "n1-in-81-tcp", "alice", 60, 60)

	if ct := clientUsage(t, "alice"); ct.Enable {
		t.Fatal("alice went over her quota and must be disabled")
	}
	select {
	case got := <-hub.Nudges():
		if got != a.Id {
			t.Fatalf("nudged node %d, want %d", got, a.Id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the agent was never asked to drop the depleted client")
	}
}
