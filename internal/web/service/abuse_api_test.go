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
	v1 := seedAgentNodeRow(t, "d-v1")
	s := &AbuseService{}
	for _, n := range []int{fresh.Id, old.Id, offline.Id, v1.Id} {
		if err := s.SetMode(n, AbuseModeObserve); err != nil {
			t.Fatal(err)
		}
	}
	connectFakeAgent(t, hub, fresh.Id, agentproto.Hello{AgentVersion: "fresh", Capabilities: []string{abuse.Capability}})
	connectFakeAgent(t, hub, old.Id, agentproto.Hello{AgentVersion: "old"})
	connectFakeAgent(t, hub, v1.Id, agentproto.Hello{AgentVersion: "v1", Capabilities: []string{"abuse-v1"}})

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
	want := []row{{"面板本机", true}, {"a-offline", false}, {"b-fresh", true}, {"c-old", false}, {"d-v1", false}}
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

// A bulk sign-up hit names the platform and how long the account kept at it.
func TestAbuseOverviewSaysWhichPlatformABulkSignUpHit(t *testing.T) {
	s := setupAbuse(t, AbuseModeObserve)
	now := time.Now()
	hits := []abuse.Signal{
		{
			Email: "alice", Rule: abuse.RuleRegister, Level: abuse.LevelStrike, Measure: abuse.MeasureRegisterHour,
			Count: 11, Limit: 10, Window: 3600, Samples: []string{abuse.PlatformOpenAI},
		},
		{
			Email: "alice", Rule: abuse.RuleRegister, Level: abuse.LevelStrike, Measure: abuse.MeasureRegisterDay,
			Count: 21, Limit: 20, Window: 24 * 3600, Samples: []string{abuse.PlatformMicrosoft},
		},
	}
	if _, err := s.HandleSignals(0, hits, now); err != nil {
		t.Fatal(err)
	}
	overview, err := s.Overview(now)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"1 小时内有 11 分钟在连 OpenAI（ChatGPT）的登录/注册服务器": true,
		"24 小时内有 21 分钟在连微软的账号注册服务器":               true,
	}
	for _, e := range overview.Events {
		if e.Label != "批量注册 AI/Google/微软账号" || !want[e.Evidence] {
			t.Errorf("bulk sign-up hit = %q: %q", e.Label, e.Evidence)
		}
		delete(want, e.Evidence)
	}
	if len(want) != 0 {
		t.Errorf("missing hits %v in %+v", want, overview.Events)
	}
}
