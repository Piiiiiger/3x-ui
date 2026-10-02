package service

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/agentproto"
	"github.com/mhsanaei/3x-ui/v3/internal/agentproto/agenttest"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/runtime"

	"github.com/gorilla/websocket"
)

func useAgentHub(t *testing.T) *runtime.AgentHub {
	t.Helper()
	hub := runtime.NewAgentHub()
	prev := runtime.GetAgentHub()
	runtime.SetAgentHub(hub)
	t.Cleanup(func() { runtime.SetAgentHub(prev) })
	return hub
}

// connectFakeAgent dials hub as nodeID's agent and waits until it is registered.
func connectFakeAgent(t *testing.T, hub *runtime.AgentHub, nodeID int, hello agentproto.Hello) *agenttest.Agent {
	t.Helper()
	url := agenttest.Server(t, func(conn *websocket.Conn) { hub.Attach(nodeID, conn) })
	agent := agenttest.Dial(t, url, hello)
	deadline := time.Now().Add(3 * time.Second)
	for {
		if st, ok := hub.Session(nodeID); ok && st.Hello.AgentVersion == hello.AgentVersion {
			return agent
		}
		if time.Now().After(deadline) {
			t.Fatal("hub never registered the agent")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func applyCarries(t *testing.T, a agentproto.Apply, tag string, emails ...string) {
	t.Helper()
	var cfg struct {
		Inbounds []struct {
			Tag      string `json:"tag"`
			Settings struct {
				Clients []struct {
					Email string `json:"email"`
				} `json:"clients"`
			} `json:"settings"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(a.Config, &cfg); err != nil {
		t.Fatalf("config is not JSON: %v", err)
	}
	for _, ib := range cfg.Inbounds {
		if ib.Tag != tag {
			continue
		}
		var got []string
		for _, c := range ib.Settings.Clients {
			got = append(got, c.Email)
		}
		slices.Sort(got)
		if !slices.Equal(got, emails) {
			t.Fatalf("%s clients = %v, want %v", tag, got, emails)
		}
		return
	}
	t.Fatalf("pushed config has no inbound %s", tag)
}

func expectApply(t *testing.T, applies <-chan agentproto.Apply) agentproto.Apply {
	t.Helper()
	select {
	case a := <-applies:
		return a
	case <-time.After(3 * time.Second):
		t.Fatal("the agent never received a config")
	}
	return agentproto.Apply{}
}

func expectNoApply(t *testing.T, applies <-chan agentproto.Apply) {
	t.Helper()
	select {
	case a := <-applies:
		t.Fatalf("an unchanged config was pushed again (hash %s)", a.Hash)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestSyncAgent_PushesTheConfigOnlyWhenItChanges(t *testing.T) {
	setupSettingTestDB(t)
	hub := useAgentHub(t)
	n := seedAgentNodeRow(t, "edge-hk")
	alice := model.Client{Email: "alice", ID: "11111111-1111-1111-1111-111111111111", Enable: true}
	ib := seedNodeInbound(t, &n.Id, "n1-in-81-tcp", 81, model.VLESS, true, []model.Client{alice})
	applies := connectFakeAgent(t, hub, n.Id, agentproto.Hello{AgentVersion: "v1"}).AnswerApplies(true)
	svc := &AgentService{}
	ctx := context.Background()

	if err := svc.SyncAgent(ctx, n); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	first := expectApply(t, applies)
	applyCarries(t, first, "n1-in-81-tcp", "alice")
	if _, hash, err := svc.AgentConfig(n.Id); err != nil || hash != first.Hash {
		t.Fatalf("pushed hash %s, AgentConfig hash %s (%v): the agent must get exactly the built config", first.Hash, hash, err)
	}

	if err := svc.SyncAgent(ctx, n); err != nil {
		t.Fatal(err)
	}
	expectNoApply(t, applies)

	bob := model.Client{Email: "bob", ID: "22222222-2222-2222-2222-222222222222", Enable: true}
	if err := (&ClientService{}).SyncInbound(nil, ib.Id, []model.Client{alice, bob}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SyncAgent(ctx, n); err != nil {
		t.Fatal(err)
	}
	applyCarries(t, expectApply(t, applies), "n1-in-81-tcp", "alice", "bob")
}

// A caller waiting for an agent to run a change must not read a missing agent as
// one that took it; the node would be reported live while no host runs it.
func TestSyncAgent_ReportsAnAgentThatIsNotConnected(t *testing.T) {
	setupSettingTestDB(t)
	useAgentHub(t)
	n := seedAgentNodeRow(t, "edge-hk")

	if err := (&AgentService{}).SyncAgent(context.Background(), n); !errors.Is(err, runtime.ErrAgentNotConnected) {
		t.Fatalf("SyncAgent with no agent connected: %v, want runtime.ErrAgentNotConnected", err)
	}
}

// A change made while the agent was away marks the node dirty; only an applied
// config may clear that, or the change would be forgotten.
func TestSyncAgent_ClearsDirtyOnlyWhenTheAgentAccepts(t *testing.T) {
	setupSettingTestDB(t)
	nodeSvc := &NodeService{}
	for _, accept := range []bool{false, true} {
		hub := useAgentHub(t)
		n := seedAgentNodeRow(t, map[bool]string{false: "refuses", true: "accepts"}[accept])
		seedNodeInbound(t, &n.Id, map[bool]string{false: "n-a-81", true: "n-b-81"}[accept], 81, model.VLESS, true, nil)
		if err := nodeSvc.MarkNodeDirty(n.Id); err != nil {
			t.Fatal(err)
		}
		applies := connectFakeAgent(t, hub, n.Id, agentproto.Hello{AgentVersion: "v1"}).AnswerApplies(accept)

		err := (&AgentService{}).SyncAgent(context.Background(), n)
		expectApply(t, applies)
		_, _, dirty, _, stateErr := nodeSvc.NodeSyncState(n.Id)
		if stateErr != nil {
			t.Fatal(stateErr)
		}
		if accept && (err != nil || dirty) {
			t.Fatalf("accepted push: err=%v dirty=%v, want nil and clean", err, dirty)
		}
		if !accept && (err == nil || !dirty) {
			t.Fatalf("refused push: err=%v dirty=%v, want an error and still dirty", err, dirty)
		}
	}
}

func TestAgentStatus_ShowsUpUnderTheInboundsGuid(t *testing.T) {
	setupSettingTestDB(t)
	useOnlineTestProcess(t)
	converted := seedAgentNodeRow(t, "edge-hk")
	if err := database.GetDB().Model(&model.Node{}).Where("id = ?", converted.Id).Update("guid", "f9713b88").Error; err != nil {
		t.Fatal(err)
	}
	fresh := seedAgentNodeRow(t, "edge-us")
	svc := &AgentService{}
	inbounds := InboundService{}

	svc.HandleStatus(converted.Id, &agentproto.Status{
		Online: []string{"alice"}, ActiveInbounds: []string{"n1-in-81-tcp"},
		IPs: map[string][]agentproto.IPEntry{"alice": {{IP: "198.51.100.4", Timestamp: time.Now().Unix()}}},
	})
	svc.HandleStatus(fresh.Id, &agentproto.Status{Online: []string{"bob"}, ActiveInbounds: []string{"n2-in-81-tcp"}})

	online := inbounds.GetOnlineClientsByGuid()
	if !slices.Contains(online["f9713b88"], "alice") {
		t.Fatalf("online by guid = %v; a converted node's inbounds keep its old GUID", online)
	}
	if key := "node:" + strconv.Itoa(fresh.Id); !slices.Contains(online[key], "bob") {
		t.Fatalf("online by guid = %v; a new agent is keyed %s like its inbounds", online, key)
	}
	if active := inbounds.GetActiveInboundsByGuid(); !slices.Contains(active["f9713b88"], "n1-in-81-tcp") {
		t.Fatalf("active inbounds by guid = %v", active)
	}
	ips, err := inbounds.GetClientIpsWithNodes("alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(ips) != 1 || ips[0].IP != "198.51.100.4" || ips[0].Node != "edge-hk" {
		t.Fatalf("alice's IPs = %+v, want 198.51.100.4 labelled edge-hk", ips)
	}

	svc.HandleGone(converted.Id)
	if online := inbounds.GetOnlineClientsByGuid(); len(online["f9713b88"]) != 0 {
		t.Fatalf("a disconnected agent's clients still show online: %v", online)
	}
}

func TestProbeAgentNode(t *testing.T) {
	setupSettingTestDB(t)
	hub := useAgentHub(t)
	n := seedAgentNodeRow(t, "edge-hk")
	nodeSvc := &NodeService{}

	if _, err := nodeSvc.Probe(context.Background(), n); err == nil {
		t.Fatal("an agent that is not connected must probe offline")
	}

	agent := connectFakeAgent(t, hub, n.Id, agentproto.Hello{AgentVersion: "v0.1.0", XrayVersion: "26.9.9"})
	if _, err := nodeSvc.Probe(context.Background(), n); err == nil {
		t.Fatal("an agent that has not reported its status yet must not count as online")
	}
	status := agentproto.Status{CpuPct: 7, MemPct: 30, UptimeSecs: 99, XrayVersion: "26.9.9", XrayState: "running"}
	agent.Send(agentproto.Message{Type: agentproto.TypeStatus, Status: &status})
	var patch HeartbeatPatch
	var err error
	deadline := time.Now().Add(3 * time.Second)
	for {
		if patch, err = nodeSvc.Probe(context.Background(), n); err == nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("probe with a fresh status: %v", err)
	}
	if patch.CpuPct != 7 || patch.UptimeSecs != 99 || patch.XrayVersion != "26.9.9" || patch.XrayState != "running" || patch.PanelVersion != "v0.1.0" {
		t.Fatalf("patch = %+v, want the agent's reported status", patch)
	}

	prev := agentStatusStaleAfter
	agentStatusStaleAfter = 50 * time.Millisecond
	t.Cleanup(func() { agentStatusStaleAfter = prev })
	time.Sleep(100 * time.Millisecond)
	if _, err := nodeSvc.Probe(context.Background(), n); err == nil {
		t.Fatal("an agent whose status went stale must probe offline")
	}
}
