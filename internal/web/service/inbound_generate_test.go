package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/agentproto"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/runtime"
)

// generateFixture holds a plan whose member should get a generated node and one
// whose member should not; both plans start on one local inbound.
type generateFixture struct {
	agent      *model.Node
	mgr        *runtime.Manager
	chosenPlan int
	otherPlan  int
	lastSort   int
}

func setupGenerate(t *testing.T) generateFixture {
	t.Helper()
	setupConflictDB(t)
	startSerializedWriter(t)
	mgr := useTestRuntimeManager(t)
	agent := seedAgentNodeRow(t, "edge-hk")
	seedInboundConflict(t, "local-a", "0.0.0.0", 47101, model.VLESS, `{"network":"tcp"}`, `{"clients":[]}`)
	const lastSort = 7
	var local model.Inbound
	if err := database.GetDB().Where("tag = ?", "local-a").First(&local).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Model(&local).UpdateColumn("sub_sort_index", lastSort).Error; err != nil {
		t.Fatal(err)
	}
	plans := map[string]int{}
	for _, name := range []string{"alice", "bob"} {
		createPlanClient(t, name+"@gen", []int{local.Id}, 0)
		plan, err := (&PlanService{}).Create(PlanInput{Name: name, InboundIds: []int{local.Id}})
		if err != nil {
			t.Fatalf("create plan %s: %v", name, err)
		}
		putOnPlan(t, plan.Id, name+"@gen")
		plans[name] = plan.Id
	}
	return generateFixture{agent: agent, mgr: mgr, chosenPlan: plans["alice"], otherPlan: plans["bob"], lastSort: lastSort}
}

func nodeRequest(nodeID *int, port, publicPort int, planIds ...int) *GenerateNodeRequest {
	return &GenerateNodeRequest{
		Inbound: model.Inbound{
			Remark: "香港-Edge-2", Enable: true, Port: port, SharePort: publicPort, Protocol: model.VLESS, NodeID: nodeID,
			Settings:       `{"clients":[],"decryption":"none"}`,
			StreamSettings: `{"network":"tcp","security":"none"}`,
			Sniffing:       `{}`,
		},
		PlanIds: planIds,
	}
}

func storedInbound(t *testing.T, id int) model.Inbound {
	t.Helper()
	var ib model.Inbound
	if err := database.GetDB().First(&ib, id).Error; err != nil {
		t.Fatalf("read inbound %d: %v", id, err)
	}
	return ib
}

func countRows(t *testing.T, table any, where string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := database.GetDB().Model(table).Where(where, args...).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

// The node must reach the chosen plan's members only, sort after every node
// people already have, and be enabled only once its agent runs it.
func TestGenerateNodeOnAnAgentGoesLiveOnceTheAgentRunsIt(t *testing.T) {
	f := setupGenerate(t)
	hub := useAgentHub(t)
	applies := connectFakeAgent(t, hub, f.agent.Id, agentproto.Hello{AgentVersion: "v1"}).AnswerApplies(true)

	created, err := (&InboundService{}).GenerateNode(nodeRequest(&f.agent.Id, 24567, 0, f.chosenPlan))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	stored := storedInbound(t, created.Id)
	if !stored.Enable || !created.Enable {
		t.Fatalf("stored enable %v, returned %v: an accepted node must be live", stored.Enable, created.Enable)
	}
	applyCarries(t, expectApply(t, applies), stored.Tag, "alice@gen")
	if st, _ := hub.Session(f.agent.Id); st.AppliedHash == "" {
		t.Fatal("the agent never recorded the config with the new node")
	}
	if !slices.Contains(planInboundIdsOf(t, "alice@gen"), created.Id) || slices.Contains(planInboundIdsOf(t, "bob@gen"), created.Id) {
		t.Fatalf("alice %v, bob %v: only the chosen plan's member gets node %d", planInboundIdsOf(t, "alice@gen"), planInboundIdsOf(t, "bob@gen"), created.Id)
	}
	if stored.SubSortIndex != f.lastSort+1 {
		t.Fatalf("sub sort index %d, want %d so the node lists after everyone's existing nodes", stored.SubSortIndex, f.lastSort+1)
	}
}

// The sync tick and a nudge push to an agent one at a time; generating a node on
// it must wait its turn too instead of pushing over a push in flight.
func TestGenerateNodeWaitsForAPushInFlightToTheSameAgent(t *testing.T) {
	f := setupGenerate(t)
	hub := useAgentHub(t)
	applies := connectFakeAgent(t, hub, f.agent.Id, agentproto.Hello{AgentVersion: "v1"}).AnswerApplies(true)

	unlock := (&AgentService{}).LockAgent(f.agent.Id)
	done := make(chan error, 1)
	go func() {
		_, err := (&InboundService{}).GenerateNode(nodeRequest(&f.agent.Id, 24567, 0, f.chosenPlan))
		done <- err
	}()
	select {
	case a := <-applies:
		t.Fatalf("generate pushed (hash %s) while another push to the agent held its lock", a.Hash)
	case <-time.After(200 * time.Millisecond):
	}
	unlock()
	expectApply(t, applies)
	if err := <-done; err != nil {
		t.Fatalf("generate after the push in flight finished: %v", err)
	}
}

// A refused node must not reach anyone's subscription, and the agent keeps
// running the config it had, so the node stays as a disabled row to fix.
func TestGenerateNodeAnAgentRefusesIsLeftDisabled(t *testing.T) {
	f := setupGenerate(t)
	hub := useAgentHub(t)
	applies := connectFakeAgent(t, hub, f.agent.Id, agentproto.Hello{AgentVersion: "v1"}).AnswerApplies(false)

	created, err := (&InboundService{}).GenerateNode(nodeRequest(&f.agent.Id, 24567, 0, f.chosenPlan))
	if want := "the node was left disabled: agent on edge-hk refused its config: refused"; err == nil || err.Error() != want {
		t.Fatalf("generate: error %v, want %q", err, want)
	}
	expectApply(t, applies)
	if created == nil {
		t.Fatal("a refused node must still be returned, so the panel can show the row it left")
	}
	if storedInbound(t, created.Id).Enable {
		t.Fatal("the refused node was left enabled, so every member's subscription lists a dead node")
	}
}

func TestGenerateNodeForAnUnknownPlanLeavesNothingBehind(t *testing.T) {
	f := setupGenerate(t)
	hub := useAgentHub(t)
	connectFakeAgent(t, hub, f.agent.Id, agentproto.Hello{AgentVersion: "v1"}).AnswerApplies(true)
	before := planInboundIdsOf(t, "alice@gen")

	_, err := (&InboundService{}).GenerateNode(nodeRequest(&f.agent.Id, 24567, 20443, f.chosenPlan, 9999))
	if err == nil || strings.TrimSpace(err.Error()) != "plan not found: [9999]" {
		t.Fatalf("generate: error %v, want plan not found: [9999]", err)
	}
	if n := countRows(t, &model.Inbound{}, "node_id = ?", f.agent.Id); n != 0 {
		t.Fatalf("%d inbounds left on the agent after a failed generate", n)
	}
	if got := planInboundIdsOf(t, "alice@gen"); !slices.Equal(got, before) {
		t.Fatalf("alice's inbounds %v, want them unchanged %v", got, before)
	}
}

// Only the local panel and a connected agent can confirm the node runs; anything
// else is refused before a row is written.
func TestGenerateNodeRefusesAHostThatCannotConfirmIt(t *testing.T) {
	for _, tc := range []struct {
		name string
		host func(t *testing.T, f generateFixture) *int
		want string
	}{
		{"panel node", func(t *testing.T, _ generateFixture) *int { return &seedPanelNodeRow(t, "remote").Id }, "a node can only be generated on the local panel or an agent host"},
		{"agent not connected", func(_ *testing.T, f generateFixture) *int { return &f.agent.Id }, "host edge-hk is not connected; start its agent first"},
		{"disabled agent", func(t *testing.T, f generateFixture) *int {
			if err := database.GetDB().Model(f.agent).UpdateColumn("enable", false).Error; err != nil {
				t.Fatal(err)
			}
			return &f.agent.Id
		}, "host edge-hk is disabled"},
		{"unknown node", func(*testing.T, generateFixture) *int { missing := 999; return &missing }, "node not found: 999"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setupGenerate(t)
			useAgentHub(t)
			host := tc.host(t, f)
			before := countRows(t, &model.Inbound{}, "1 = 1")

			_, err := (&InboundService{}).GenerateNode(nodeRequest(host, 24567, 0, f.chosenPlan))
			if err == nil || err.Error() != tc.want {
				t.Fatalf("generate: error %v, want %q", err, tc.want)
			}
			if after := countRows(t, &model.Inbound{}, "1 = 1"); after != before {
				t.Fatalf("%d inbounds before, %d after a refused generate", before, after)
			}
		})
	}
}

type refusingLocalRuntime struct{ fakeNodeRuntime }

func (*refusingLocalRuntime) AddInbound(context.Context, *model.Inbound) error {
	return errors.New("listen tcp :24567: bind: address already in use")
}

// The local panel's Xray takes a node through a live add; one it refuses would
// otherwise ask for a restart that cannot bring it up either.
func TestGenerateNodeOnTheLocalPanel(t *testing.T) {
	freePort := func(t *testing.T) int {
		l, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()
		return l.Addr().(*net.TCPAddr).Port
	}

	t.Run("live add accepted", func(t *testing.T) {
		f := setupGenerate(t)
		live := &fakeNodeRuntime{}
		f.mgr.SetLocalRuntimeOverride(live)
		created, err := (&InboundService{}).GenerateNode(nodeRequest(nil, freePort(t), 0, f.chosenPlan))
		if err != nil {
			t.Fatalf("generate: %v", err)
		}
		if !storedInbound(t, created.Id).Enable || live.addInbound.Load() == 0 {
			t.Fatalf("enabled %v, live adds %d: the accepted node must be enabled and added live", storedInbound(t, created.Id).Enable, live.addInbound.Load())
		}
	})

	t.Run("live add refused", func(t *testing.T) {
		f := setupGenerate(t)
		f.mgr.SetLocalRuntimeOverride(&refusingLocalRuntime{})
		created, err := (&InboundService{}).GenerateNode(nodeRequest(nil, freePort(t), 0, f.chosenPlan))
		if want := "the node was left disabled: the panel's Xray did not take it live; see the panel log"; err == nil || err.Error() != want {
			t.Fatalf("generate: error %v, want %q", err, want)
		}
		if storedInbound(t, created.Id).Enable {
			t.Fatal("a node the local Xray refused was left enabled")
		}
	})

	t.Run("port held by another program", func(t *testing.T) {
		f := setupGenerate(t)
		live := &fakeNodeRuntime{}
		f.mgr.SetLocalRuntimeOverride(live)
		held, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Fatal(err)
		}
		defer held.Close()
		port := held.Addr().(*net.TCPAddr).Port

		created, err := (&InboundService{}).GenerateNode(nodeRequest(nil, port, 0, f.chosenPlan))
		if want := fmt.Sprintf("the node was left disabled: port %d is already in use on this machine", port); err == nil || err.Error() != want {
			t.Fatalf("generate: error %v, want %q", err, want)
		}
		if storedInbound(t, created.Id).Enable || live.addInbound.Load() != 0 {
			t.Fatal("a node on a port another program holds must not be enabled or added live")
		}
	})
}

// The name is the proxy name in every subscription and the entry row's remark, so
// a nameless node is refused before anything is written.
func TestGenerateNodeNeedsAName(t *testing.T) {
	f := setupGenerate(t)
	hub := useAgentHub(t)
	connectFakeAgent(t, hub, f.agent.Id, agentproto.Hello{AgentVersion: "v1"}).AnswerApplies(true)
	req := nodeRequest(&f.agent.Id, 24567, 20443, f.chosenPlan)
	req.Inbound.Remark = "  "

	_, err := (&InboundService{}).GenerateNode(req)
	if want := "a node needs a name: it is the name people see in their subscription"; err == nil || err.Error() != want {
		t.Fatalf("generate: error %v, want %q", err, want)
	}
	if n := countRows(t, &model.Inbound{}, "node_id = ?", f.agent.Id); n != 0 {
		t.Fatalf("%d inbounds written for a nameless node", n)
	}
}
