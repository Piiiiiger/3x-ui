package runtime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/agentproto"
	"github.com/mhsanaei/3x-ui/v3/internal/agentproto/agenttest"

	"github.com/gorilla/websocket"
)

// serveHub exposes hub on a real WebSocket endpoint, binding every connection
// to nodeID the way the panel's agent route does.
func serveHub(t *testing.T, hub *AgentHub, nodeID int) string {
	t.Helper()
	return agenttest.Server(t, func(conn *websocket.Conn) { hub.Attach(nodeID, conn) })
}

func dialFakeAgent(t *testing.T, url string, hello agentproto.Hello) *agenttest.Agent {
	t.Helper()
	return agenttest.Dial(t, url, hello)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func helloReceived(hub *AgentHub, nodeID int, version string) func() bool {
	return func() bool {
		st, ok := hub.Session(nodeID)
		return ok && st.Hello.AgentVersion == version
	}
}

func TestAgentHub_ApplyWaitsForTheAgentsResult(t *testing.T) {
	hub := NewAgentHub()
	agent := dialFakeAgent(t, serveHub(t, hub, 7), agentproto.Hello{AgentVersion: "v1", ConfigHash: "running"})
	waitFor(t, "hello", helloReceived(hub, 7, "v1"))
	if st, _ := hub.Session(7); st.AppliedHash != "running" {
		t.Fatalf("applied hash = %q, want the hash from hello so an unchanged config is not re-sent", st.AppliedHash)
	}

	type outcome struct {
		res *agentproto.Result
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		res, err := hub.Apply(context.Background(), 7, agentproto.Apply{Config: []byte(`{"inbounds":[]}`), Hash: "next"})
		done <- outcome{res, err}
	}()

	msg, err := agent.Recv(2 * time.Second)
	if err != nil {
		t.Fatalf("agent never got the apply: %v", err)
	}
	if msg.Type != agentproto.TypeApply || msg.Apply == nil || msg.Apply.Hash != "next" || string(msg.Apply.Config) != `{"inbounds":[]}` {
		t.Fatalf("agent got %+v, want the apply with hash next", msg)
	}
	select {
	case o := <-done:
		t.Fatalf("Apply returned before the agent answered: %+v", o)
	case <-time.After(50 * time.Millisecond):
	}
	agent.Send(agentproto.Message{Type: agentproto.TypeResult, ID: msg.ID, Result: &agentproto.Result{OK: true, ConfigHash: "next"}})

	o := <-done
	if o.err != nil || o.res == nil || !o.res.OK {
		t.Fatalf("Apply = %+v, %v; want OK", o.res, o.err)
	}
	if st, _ := hub.Session(7); st.AppliedHash != "next" {
		t.Fatalf("applied hash = %q, want next after an accepted apply", st.AppliedHash)
	}
}

func TestAgentHub_RejectedApplyKeepsTheOldHash(t *testing.T) {
	hub := NewAgentHub()
	agent := dialFakeAgent(t, serveHub(t, hub, 7), agentproto.Hello{AgentVersion: "v1", ConfigHash: "running"})
	waitFor(t, "hello", helloReceived(hub, 7, "v1"))
	go func() {
		msg, err := agent.Recv(2 * time.Second)
		if err == nil {
			agent.Conn.WriteJSON(agentproto.Message{Type: agentproto.TypeResult, ID: msg.ID, Result: &agentproto.Result{Error: "port taken"}})
		}
	}()
	res, err := hub.Apply(context.Background(), 7, agentproto.Apply{Config: []byte(`{}`), Hash: "bad"})
	if err != nil || res.OK || res.Error != "port taken" {
		t.Fatalf("Apply = %+v, %v; want the agent's refusal", res, err)
	}
	if st, _ := hub.Session(7); st.AppliedHash != "running" {
		t.Fatalf("applied hash = %q, want running: a refused config is not what the agent runs", st.AppliedHash)
	}
}

func TestAgentHub_TrafficIsAckedOnlyAfterTheHandlerAccepts(t *testing.T) {
	hub := NewAgentHub()
	var mu sync.Mutex
	var calls []int64
	fail := true
	hub.SetHandlers(AgentHandlers{Traffic: func(nodeID int, tr *agentproto.Traffic) error {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, tr.Seq)
		if fail {
			fail = false
			return errors.New("database busy")
		}
		return nil
	}})
	agent := dialFakeAgent(t, serveHub(t, hub, 3), agentproto.Hello{AgentVersion: "v1"})
	report := agentproto.Message{Type: agentproto.TypeTraffic, Traffic: &agentproto.Traffic{
		Instance: "i1", Seq: 9, Clients: []agentproto.Counter{{Name: "alice", Up: 1, Down: 2}},
	}}

	agent.Send(report)
	if msg, err := agent.Recv(300 * time.Millisecond); err == nil {
		t.Fatalf("got %+v; a report the panel failed to store must not be acked", msg)
	}
	agent.Conn.Close()

	agent = dialFakeAgent(t, serveHub(t, hub, 3), agentproto.Hello{AgentVersion: "v1"})
	agent.Send(report)
	msg, err := agent.Recv(2 * time.Second)
	if err != nil {
		t.Fatalf("no ack after the panel stored the report: %v", err)
	}
	if msg.Type != agentproto.TypeAck || msg.Ack == nil || msg.Ack.Instance != "i1" || msg.Ack.Seq != 9 {
		t.Fatalf("got %+v, want ack i1/9", msg)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 2 {
		t.Fatalf("handler saw %v, want the report twice", calls)
	}
}

func TestAgentHub_NewConnectionReplacesTheOld(t *testing.T) {
	hub := NewAgentHub()
	var gone []int
	var mu sync.Mutex
	hub.SetHandlers(AgentHandlers{Gone: func(nodeID int) {
		mu.Lock()
		gone = append(gone, nodeID)
		mu.Unlock()
	}})
	url := serveHub(t, hub, 5)
	first := dialFakeAgent(t, url, agentproto.Hello{AgentVersion: "first"})
	waitFor(t, "first hello", helloReceived(hub, 5, "first"))
	second := dialFakeAgent(t, url, agentproto.Hello{AgentVersion: "second"})
	waitFor(t, "second hello", helloReceived(hub, 5, "second"))

	first.ExpectClosed("the replaced connection must be closed")
	go func() {
		msg, err := second.Recv(2 * time.Second)
		if err == nil {
			second.Conn.WriteJSON(agentproto.Message{Type: agentproto.TypeResult, ID: msg.ID, Result: &agentproto.Result{OK: true}})
		}
	}()
	if err := hub.Restart(context.Background(), 5); err != nil {
		t.Fatalf("Restart must reach the newer connection: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(gone) != 0 {
		t.Fatalf("Gone(%v) fired for a node that is still connected", gone)
	}
}

func TestAgentHub_RequestFailsWhenTheAgentDrops(t *testing.T) {
	hub := NewAgentHub()
	agent := dialFakeAgent(t, serveHub(t, hub, 2), agentproto.Hello{AgentVersion: "v1"})
	waitFor(t, "hello", helloReceived(hub, 2, "v1"))
	go func() {
		if _, err := agent.Recv(2 * time.Second); err == nil {
			agent.Conn.Close()
		}
	}()
	start := time.Now()
	_, err := hub.Apply(context.Background(), 2, agentproto.Apply{Config: []byte(`{}`), Hash: "h"})
	if !errors.Is(err, ErrAgentNotConnected) {
		t.Fatalf("Apply after the agent dropped = %v, want ErrAgentNotConnected", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("Apply waited for a timeout instead of noticing the drop")
	}
}

func TestAgentHub_ApplyWithoutAgent(t *testing.T) {
	if _, err := NewAgentHub().Apply(context.Background(), 1, agentproto.Apply{}); !errors.Is(err, ErrAgentNotConnected) {
		t.Fatalf("Apply with no session = %v, want ErrAgentNotConnected", err)
	}
}

func TestAgentHub_StatusDisconnectAndGone(t *testing.T) {
	hub := NewAgentHub()
	var mu sync.Mutex
	var statuses []agentproto.Status
	goneCh := make(chan int, 1)
	hub.SetHandlers(AgentHandlers{
		Status: func(nodeID int, s *agentproto.Status) {
			mu.Lock()
			statuses = append(statuses, *s)
			mu.Unlock()
		},
		Gone: func(nodeID int) { goneCh <- nodeID },
	})
	agent := dialFakeAgent(t, serveHub(t, hub, 4), agentproto.Hello{AgentVersion: "v1"})
	agent.Send(agentproto.Message{Type: agentproto.TypeStatus, Status: &agentproto.Status{CpuPct: 12, Online: []string{"alice"}}})
	waitFor(t, "status", func() bool {
		st, ok := hub.Session(4)
		return ok && st.Status.CpuPct == 12 && !st.StatusAt.IsZero()
	})
	mu.Lock()
	if len(statuses) != 1 || statuses[0].Online[0] != "alice" {
		t.Fatalf("status handler saw %+v", statuses)
	}
	mu.Unlock()

	hub.Disconnect(4)
	agent.ExpectClosed("Disconnect must close the agent's connection")
	select {
	case id := <-goneCh:
		if id != 4 {
			t.Fatalf("Gone(%d), want 4", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Gone never fired after the session ended")
	}
	if _, ok := hub.Session(4); ok {
		t.Fatal("a closed session must not be reported")
	}
}

func TestAgentHubProbeUsesRelayAndPropagatesFailure(t *testing.T) {
	hub := NewAgentHub()
	agent := dialFakeAgent(t, serveHub(t, hub, 7), agentproto.Hello{AgentVersion: "probe", Capabilities: []string{agentproto.ProbeCapability}})
	waitFor(t, "hello", helloReceived(hub, 7, "probe"))
	done := make(chan error, 1)
	go func() { done <- hub.Probe(context.Background(), 7, agentproto.Probe{Host: "2001:db8::1", Port: 19336}) }()
	msg, err := agent.Recv(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Type != agentproto.TypeProbe || msg.Probe == nil || msg.Probe.Host != "2001:db8::1" || msg.Probe.Port != 19336 {
		t.Fatalf("bad probe: %+v", msg)
	}
	agent.Send(agentproto.Message{Type: agentproto.TypeResult, ID: msg.ID, Result: &agentproto.Result{Error: "network is unreachable"}})
	if err := <-done; err == nil || err.Error() != "network is unreachable" {
		t.Fatalf("lost agent failure: %v", err)
	}
}

func TestAgentHubProbeRejectsLegacyAgent(t *testing.T) {
	hub := NewAgentHub()
	dialFakeAgent(t, serveHub(t, hub, 8), agentproto.Hello{AgentVersion: "old"})
	waitFor(t, "hello", helloReceived(hub, 8, "old"))
	if err := hub.Probe(context.Background(), 8, agentproto.Probe{Host: "example.com", Port: 443}); err == nil {
		t.Fatal("legacy agent was treated as checked")
	}
}
