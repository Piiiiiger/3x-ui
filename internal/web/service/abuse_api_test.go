package service

import (
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/abuse"
	"github.com/mhsanaei/3x-ui/v3/internal/agentproto"
)

// The overview puts the panel's core first, then each agent with its mode and
// whether it can detect: an agent that never said it can has no effect yet.
func TestAbuseOverviewSaysWhichServersCanDetect(t *testing.T) {
	setupIpLimitTest(t)
	hub := useAgentHub(t)
	fresh, old, offline := seedAgentNodeRow(t, "b-fresh"), seedAgentNodeRow(t, "c-old"), seedAgentNodeRow(t, "a-offline")
	s := &AbuseService{}
	for _, n := range []int{fresh.Id, old.Id, offline.Id} {
		if err := s.SetMode(n, AbuseModeObserve); err != nil {
			t.Fatal(err)
		}
	}
	connectFakeAgent(t, hub, fresh.Id, agentproto.Hello{AgentVersion: "fresh", Capabilities: []string{abuse.Capability}})
	connectFakeAgent(t, hub, old.Id, agentproto.Hello{AgentVersion: "old"})

	overview, err := s.Overview(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		name    string
		capable bool
	}
	var got []row
	for _, srv := range overview.Servers {
		got = append(got, row{srv.Name, srv.Capable})
	}
	want := []row{{"面板本机", true}, {"a-offline", false}, {"b-fresh", true}, {"c-old", false}}
	if len(got) != len(want) {
		t.Fatalf("servers = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("servers = %+v, want %+v", got, want)
		}
	}
}

// Each hit is said in words, and a network's sign-ups come from the user page.
func TestAbuseOverviewSaysWhereAndWhatEachHitWas(t *testing.T) {
	s := setupAbuse(t, AbuseModeObserve)
	now := time.Now()
	if _, err := s.HandleSignals(0, []abuse.Signal{strike(abuse.RuleScan)}, now); err != nil {
		t.Fatal(err)
	}
	for i, ip := range []string{"203.0.113.5", "203.0.113.6", "203.0.113.7"} {
		if err := signUp(t, s, ip, []string{"u1", "u2", "u3"}[i], now); err != nil {
			t.Fatal(err)
		}
	}

	overview, err := s.Overview(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Events) != 2 {
		t.Fatalf("events = %+v, want the scan and the sign-ups", overview.Events)
	}
	signups, scan := overview.Events[0], overview.Events[1]
	if signups.Server != "用户页面" || signups.Label != "批量注册" || signups.Evidence != "网络 203.0.113.0/24 在 24 小时内注册了 3 个账号" {
		t.Errorf("sign-up hit = %+v", signups)
	}
	if scan.Server != "面板本机" || scan.Label != "端口扫描" || scan.Evidence != "5 分钟内连接同一 IP 的 52 个端口" {
		t.Errorf("scan hit = %+v", scan)
	}
}
