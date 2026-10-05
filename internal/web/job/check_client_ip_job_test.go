package job

import (
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/agentproto"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// One scan must count what the agents report, ban what is over the limit and
// tell the local core to pick the new config up.
func TestCheckClientIpJobBansFromAgentReportsAndFlagsTheLocalCore(t *testing.T) {
	hub := setupAgentJobTest(t)
	t.Setenv("XUI_LOG_FOLDER", t.TempDir())
	n := seedAgent(t, "edge")
	in := &model.Inbound{Tag: "in-job", Enable: true, Protocol: model.VLESS, Port: 443, Settings: `{"clients":[],"decryption":"none"}`, NodeID: &n.Id}
	if err := database.GetDB().Create(in).Error; err != nil {
		t.Fatal(err)
	}
	client := model.Client{Email: "pat", ID: "44444444-4444-4444-8444-444444444444", LimitIP: 3, Enable: true}
	if err := (&service.ClientService{}).SyncInbound(nil, in.Id, []model.Client{client}); err != nil {
		t.Fatal(err)
	}
	agent := connectAgent(t, hub, n)
	now := time.Now().Unix()
	status := agentproto.Status{IPs: map[string][]agentproto.IPEntry{"pat": {
		{IP: "198.51.100.1", Timestamp: now - 400},
		{IP: "198.51.100.2", Timestamp: now - 30},
		{IP: "198.51.100.3", Timestamp: now - 20},
		{IP: "198.51.100.4", Timestamp: now - 10},
	}}}
	agent.Send(agentproto.Message{Type: agentproto.TypeStatus, Status: &status})
	deadline := time.Now().Add(3 * time.Second)
	for {
		if st, ok := hub.Session(n.Id); ok && !st.StatusAt.IsZero() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the agent's status never arrived")
		}
		time.Sleep(5 * time.Millisecond)
	}
	xrayService := service.XrayService{}
	xrayService.IsNeedRestartAndSetFalse()

	NewCheckClientIpJob().Run()

	var bans []model.ClientIpBan
	if err := database.GetDB().Find(&bans).Error; err != nil {
		t.Fatal(err)
	}
	if len(bans) != 1 || bans[0].Email != "pat" || bans[0].Network != "198.51.100.1" {
		t.Fatalf("bans = %+v; want pat's stalest network banned", bans)
	}
	if !xrayService.IsNeedRestartAndSetFalse() {
		t.Fatal("the local core was not told to pick the ban up")
	}
}
