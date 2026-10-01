package job

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/op/go-logging"

	"github.com/mhsanaei/3x-ui/v3/internal/agentproto"
	"github.com/mhsanaei/3x-ui/v3/internal/agentproto/agenttest"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	xuilogger "github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/runtime"

	"github.com/gorilla/websocket"
)

func setupAgentJobTest(t *testing.T) *runtime.AgentHub {
	t.Helper()
	xuilogger.InitLogger(logging.ERROR)
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	runtime.SetManager(runtime.NewManager(runtime.LocalDeps{APIPort: func() int { return 0 }, SetNeedRestart: func() {}}))
	t.Cleanup(func() { runtime.SetManager(nil) })
	hub := runtime.NewAgentHub()
	runtime.SetAgentHub(hub)
	t.Cleanup(func() { runtime.SetAgentHub(nil) })
	return hub
}

func seedAgent(t *testing.T, name string) *model.Node {
	t.Helper()
	n := &model.Node{Name: name, Kind: model.NodeKindAgent, Address: "203.0.113.9", Enable: true, Status: "online"}
	if err := database.GetDB().Create(n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func connectAgent(t *testing.T, hub *runtime.AgentHub, n *model.Node) *agenttest.Agent {
	t.Helper()
	url := agenttest.Server(t, func(conn *websocket.Conn) { hub.Attach(n.Id, conn) })
	agent := agenttest.Dial(t, url, agentproto.Hello{AgentVersion: n.Name})
	deadline := time.Now().Add(3 * time.Second)
	for {
		if st, ok := hub.Session(n.Id); ok && st.Hello.AgentVersion == n.Name {
			return agent
		}
		if time.Now().After(deadline) {
			t.Fatalf("agent %s never registered", n.Name)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func expectApplied(t *testing.T, applies <-chan agentproto.Apply, who string) {
	t.Helper()
	select {
	case <-applies:
	case <-time.After(3 * time.Second):
		t.Fatalf("%s never received its config", who)
	}
}

func TestAgentSyncJob_PushesConnectedAgentsAndDropsRetiredOnes(t *testing.T) {
	hub := setupAgentJobTest(t)
	live := seedAgent(t, "live")
	disabled := seedAgent(t, "disabled")
	deleted := seedAgent(t, "deleted")
	demoted := seedAgent(t, "demoted")
	applies := connectAgent(t, hub, live).AnswerApplies(true)
	retired := []*agenttest.Agent{
		connectAgent(t, hub, disabled),
		connectAgent(t, hub, deleted),
		connectAgent(t, hub, demoted),
	}
	db := database.GetDB()
	if err := db.Model(&model.Node{}).Where("id = ?", disabled.Id).Update("enable", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&model.Node{}, deleted.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.Node{}).Where("id = ?", demoted.Id).Update("kind", model.NodeKindPanel).Error; err != nil {
		t.Fatal(err)
	}

	NewAgentSyncJob().Run()

	expectApplied(t, applies, "the live agent")
	for _, agent := range retired {
		agent.ExpectClosed("an agent whose node no longer allows it must be disconnected")
	}
}

// The panel already zeroed a reset client's counters; a reset left queued for an
// agent would keep that client's node verdict frozen.
func TestAgentSyncJob_ClearsQueuedResets(t *testing.T) {
	hub := setupAgentJobTest(t)
	n := seedAgent(t, "lazycat")
	connectAgent(t, hub, n).AnswerApplies(true)
	if err := database.GetDB().Create(&model.NodePendingReset{NodeId: n.Id, Email: "alice", QueuedAt: 1}).Error; err != nil {
		t.Fatal(err)
	}

	NewAgentSyncJob().Run()

	var left int64
	database.GetDB().Model(&model.NodePendingReset{}).Where("node_id = ?", n.Id).Count(&left)
	if left != 0 {
		t.Fatalf("%d resets still queued for the agent", left)
	}
}

func TestAgentSyncJob_NudgeSyncsWithoutWaitingForTheTick(t *testing.T) {
	hub := setupAgentJobTest(t)
	n := seedAgent(t, "lazycat")
	agent := connectAgent(t, hub, n)
	// The hello already nudged; drain it so only the nudge below can sync.
	for len(hub.Nudges()) > 0 {
		<-hub.Nudges()
	}
	applies := agent.AnswerApplies(true)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go NewAgentSyncJob().WatchNudges(ctx)

	hub.Nudge(n.Id)
	select {
	case <-applies:
	case <-time.After(time.Second):
		t.Fatal("a nudged agent was not synced within a second")
	}
}

// The panel-node traffic sync has nothing to fetch from an agent; looking one up
// anyway logged a warning for every agent every five seconds.
func TestNodeTrafficSyncLeavesAgentsAlone(t *testing.T) {
	setupAgentJobTest(t)
	seedAgent(t, "agent-quiet-check")

	NewNodeTrafficSyncJob().Run()

	for _, line := range xuilogger.GetLogs(500, "DEBUG") {
		if strings.Contains(line, "agent-quiet-check") {
			t.Fatalf("panel-node sync touched the agent: %s", line)
		}
	}
}
