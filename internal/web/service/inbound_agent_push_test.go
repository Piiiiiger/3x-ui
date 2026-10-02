package service

import (
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/runtime"
)

// expectAgentNudge waits for nodeID's nudge, the signal that has the sync loop
// push that agent its config now instead of on its next tick.
func expectAgentNudge(t *testing.T, hub *runtime.AgentHub, nodeID int, after string) {
	t.Helper()
	select {
	case got := <-hub.Nudges():
		if got != nodeID {
			t.Fatalf("%s nudged node %d, want %d", after, got, nodeID)
		}
	case <-time.After(time.Second):
		t.Fatalf("%s never nudged agent node %d, so the agent waits for the next sync tick", after, nodeID)
	}
}

func setupAgentPush(t *testing.T) (*runtime.AgentHub, *model.Node) {
	t.Helper()
	setupConflictDB(t)
	useTestRuntimeManager(t)
	hub := useAgentHub(t)
	return hub, seedAgentNodeRow(t, "edge-hk")
}

func vlessInboundOnNode(nodeID int, port int) *model.Inbound {
	return &model.Inbound{
		Enable:         true,
		Port:           port,
		Protocol:       model.VLESS,
		NodeID:         &nodeID,
		Settings:       `{"clients":[],"decryption":"none"}`,
		StreamSettings: `{"network":"tcp"}`,
	}
}

// An inbound added on an agent reached it only on the next sync tick, up to five
// seconds later, while a delete was pushed at once.
func TestAddInbound_NudgesTheAgentNode(t *testing.T) {
	hub, agent := setupAgentPush(t)
	if _, _, err := (&InboundService{}).AddInbound(vlessInboundOnNode(agent.Id, 8443)); err != nil {
		t.Fatalf("AddInbound: %v", err)
	}
	expectAgentNudge(t, hub, agent.Id, "adding an inbound on the agent")
}

// An offline agent has no runtime to take the push; the add must still be saved
// and leave the node dirty, so the agent gets it once it is back.
func TestAddInbound_OfflineAgentGetsItOnceBack(t *testing.T) {
	_, agent := setupAgentPush(t)
	if err := database.GetDB().Model(agent).Update("status", "offline").Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := (&InboundService{}).AddInbound(vlessInboundOnNode(agent.Id, 8443)); err != nil {
		t.Fatalf("AddInbound on an offline agent: %v", err)
	}
	if _, _, dirty, _, err := (&NodeService{}).NodeSyncState(agent.Id); err != nil || !dirty {
		t.Fatalf("the offline agent must stay dirty for its next sync; dirty=%v err=%v", dirty, err)
	}
}

// An edit on an agent waited for the next sync tick the same way.
func TestUpdateInbound_NudgesTheAgentNode(t *testing.T) {
	hub, agent := setupAgentPush(t)
	seeded := seedNodeInbound(t, &agent.Id, "n1-in-81-tcp", 81, model.VLESS, true, nil)
	edit, err := (&InboundService{}).GetInbound(seeded.Id)
	if err != nil {
		t.Fatal(err)
	}
	edit.Remark = "edge-hk-2"
	if _, _, err := (&InboundService{}).UpdateInbound(edit); err != nil {
		t.Fatalf("UpdateInbound: %v", err)
	}
	expectAgentNudge(t, hub, agent.Id, "editing an inbound on the agent")
}

// Enabling and disabling already went through the node's runtime; an agent must
// keep getting either at once, or a disabled inbound stays served until the tick.
func TestSetInboundEnable_NudgesTheAgentNode(t *testing.T) {
	for _, tc := range []struct {
		action      string
		seededOn    bool
		requestedOn bool
	}{
		{"enabling", false, true},
		{"disabling", true, false},
	} {
		t.Run(tc.action, func(t *testing.T) {
			hub, agent := setupAgentPush(t)
			seeded := seedNodeInbound(t, &agent.Id, "n1-in-81-tcp", 81, model.VLESS, tc.seededOn, nil)
			if _, err := (&InboundService{}).SetInboundEnable(seeded.Id, tc.requestedOn); err != nil {
				t.Fatalf("SetInboundEnable(%v): %v", tc.requestedOn, err)
			}
			expectAgentNudge(t, hub, agent.Id, tc.action+" an inbound on the agent")
		})
	}
}

// The reorder goes to a node through a narrow runtime call, and the agent runtime
// had none, so reordering an agent's inbound failed after it was saved.
func TestSetInboundSubSortIndex_NudgesTheAgentNode(t *testing.T) {
	hub, agent := setupAgentPush(t)
	seeded := seedNodeInbound(t, &agent.Id, "n1-in-81-tcp", 81, model.VLESS, true, nil)
	if err := (&InboundService{}).SetInboundSubSortIndex(seeded.Id, 7); err != nil {
		t.Fatalf("SetInboundSubSortIndex: %v", err)
	}
	expectAgentNudge(t, hub, agent.Id, "reordering an inbound on the agent")
}

// A panel node gets adds and edits from the dirty-flag reconcile; pushing them live
// would call the remote panel on every save.
func TestAddAndUpdateInbound_PanelNodeWaitsForTheReconcile(t *testing.T) {
	setupConflictDB(t)
	nodeID, fake := setupNodeRuntime(t)
	created, _, err := (&InboundService{}).AddInbound(vlessInboundOnNode(nodeID, 8443))
	if err != nil {
		t.Fatalf("AddInbound: %v", err)
	}
	created.Remark = "edited"
	if _, _, err := (&InboundService{}).UpdateInbound(created); err != nil {
		t.Fatalf("UpdateInbound: %v", err)
	}
	if adds, updates := fake.addInbound.Load(), fake.updateInbound.Load(); adds != 0 || updates != 0 {
		t.Fatalf("the panel node got %d adds and %d updates pushed live, want both left to the reconcile", adds, updates)
	}
}
