package runtime

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func installAgentHub(t *testing.T) *AgentHub {
	t.Helper()
	hub := NewAgentHub()
	prev := GetAgentHub()
	SetAgentHub(hub)
	t.Cleanup(func() { SetAgentHub(prev) })
	return hub
}

func seedAgentNode(t *testing.T) *model.Node {
	t.Helper()
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	n := &model.Node{Name: "edge-hk", Kind: model.NodeKindAgent, Address: "203.0.113.53", Enable: true}
	if err := database.GetDB().Create(n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func expectNudge(t *testing.T, hub *AgentHub, nodeID int, after string) {
	t.Helper()
	select {
	case got := <-hub.Nudges():
		if got != nodeID {
			t.Fatalf("%s nudged node %d, want %d", after, got, nodeID)
		}
	case <-time.After(time.Second):
		t.Fatalf("%s did not ask the sync loop to push the agent's config", after)
	}
}

// An agent node has no panel API: every change must reach it through a config
// push, never an HTTP call to its address.
func TestManagerRuntimeForAgentNode(t *testing.T) {
	hub := installAgentHub(t)
	n := seedAgentNode(t)
	m := NewManager(LocalDeps{})

	rt, err := m.RuntimeFor(&n.Id)
	if err != nil {
		t.Fatalf("RuntimeFor(agent): %v", err)
	}
	if _, ok := rt.(*AgentRuntime); !ok {
		t.Fatalf("RuntimeFor(agent) = %T, want *AgentRuntime", rt)
	}
	ib := &model.Inbound{Tag: "n1-in-81-tcp", NodeID: &n.Id}
	ctx := context.Background()

	if err := rt.AddClient(ctx, ib, model.Client{Email: "alice"}); err != nil {
		t.Fatalf("AddClient: %v", err)
	}
	expectNudge(t, hub, n.Id, "AddClient")
	if err := rt.DelInbound(ctx, ib); err != nil {
		t.Fatalf("DelInbound: %v", err)
	}
	expectNudge(t, hub, n.Id, "DelInbound")
	if err := rt.UpdateUser(ctx, ib, "alice", model.Client{Email: "alice"}); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	expectNudge(t, hub, n.Id, "UpdateUser")

	// The panel already zeroed its own counters; the agent keeps none.
	if err := rt.ResetClientTraffic(ctx, ib, "alice"); err != nil {
		t.Fatalf("ResetClientTraffic: %v", err)
	}
	if err := rt.RestartXray(ctx); !errors.Is(err, ErrAgentNotConnected) {
		t.Fatalf("RestartXray with no agent connected = %v, want ErrAgentNotConnected", err)
	}
}

func TestManagerRemoteForRefusesAgentNodes(t *testing.T) {
	m := NewManager(LocalDeps{})
	if _, err := m.RemoteFor(&model.Node{Id: 3, Name: "a", Kind: model.NodeKindAgent, Address: "1.2.3.4"}); err == nil {
		t.Fatal("RemoteFor must refuse an agent node: it has no panel API to call")
	}
}
