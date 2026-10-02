package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func seedOverviewNode(t *testing.T, name, remark string) int {
	t.Helper()
	n := &model.Node{Name: name, Remark: remark, Kind: model.NodeKindAgent, Address: "203.0.113.7", Enable: true}
	if err := database.GetDB().Create(n).Error; err != nil {
		t.Fatalf("seed node %s: %v", name, err)
	}
	return n.Id
}

func seedHostDays(t *testing.T, rows ...model.HostDailyTraffic) {
	t.Helper()
	for _, r := range rows {
		if err := database.GetDB().Create(&r).Error; err != nil {
			t.Fatalf("seed host day %+v: %v", r, err)
		}
	}
}

func seedClientDays(t *testing.T, rows ...model.ClientDailyTraffic) {
	t.Helper()
	for _, r := range rows {
		if err := database.GetDB().Create(&r).Error; err != nil {
			t.Fatalf("seed client day %+v: %v", r, err)
		}
	}
}

func linkedServer(nodeID int, limit, used int64) ProbeServer {
	return ProbeServer{Linked: true, NodeId: nodeID, TrafficLimit: limit, TrafficUsed: used}
}

// The cards add up what Lite reports for the linked hosts: a host without a quota
// counts as unlimited, and one past its quota adds nothing to what is left.
func TestOverviewSumsTheLinkedHostsQuotasAndUse(t *testing.T) {
	setupConflictDB(t)
	over := seedOverviewNode(t, "over", "Over quota")
	free := seedOverviewNode(t, "free", "")
	loose := seedOverviewNode(t, "loose", "")
	probe := ProbeOverview{Configured: true, Servers: []ProbeServer{
		linkedServer(0, 1500*planGiB, 250*planGiB),
		linkedServer(over, 500*planGiB, 600*planGiB),
		linkedServer(free, 0, 1*planGiB),
		{Id: "monitor-only", TrafficLimit: 900 * planGiB, TrafficUsed: 5 * planGiB},
	}}

	ov, err := (&TrafficStatsService{}).Overview(statsDay, 7, TrafficMonth, probe)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	got := ov.Servers
	if got.QuotaBytes != 2000*planGiB || got.UsedBytes != 851*planGiB || got.RemainingBytes != 1250*planGiB ||
		got.Unlimited != 1 || got.Unlinked != 1 || !got.Configured {
		t.Errorf("servers = %+v, want quota 2000, used 851, remaining 1250 GiB, 1 unlimited, 1 unlinked", got)
	}
	want := []TrafficHost{
		{NodeId: 0, Name: "", Linked: true, QuotaBytes: 1500 * planGiB, UsedBytes: 250 * planGiB},
		{NodeId: over, Name: "Over quota", Linked: true, QuotaBytes: 500 * planGiB, UsedBytes: 600 * planGiB},
		{NodeId: free, Name: "free", Linked: true, QuotaBytes: 0, UsedBytes: 1 * planGiB},
		{NodeId: loose, Name: "loose"},
	}
	if fmt.Sprint(ov.Hosts) != fmt.Sprint(want) {
		t.Errorf("hosts = %+v\nwant  %+v", ov.Hosts, want)
	}
}

// A failed Lite fetch still lists every host, unlinked, and says why.
func TestOverviewWithoutLiteListsHostsUnlinked(t *testing.T) {
	setupConflictDB(t)
	seedOverviewNode(t, "edge", "")
	ov, err := (&TrafficStatsService{}).Overview(statsDay, 7, TrafficMonth, ProbeOverview{Configured: true, Error: "Lite did not answer"})
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if ov.Servers.Error != "Lite did not answer" || ov.Servers.Unlinked != 2 || len(ov.Hosts) != 2 || ov.Hosts[1].Linked {
		t.Fatalf("servers = %+v, hosts = %+v; want both hosts unlinked and Lite's error passed on", ov.Servers, ov.Hosts)
	}
}

func TestTrafficPeriodStartsAtTheDayWeekOrMonth(t *testing.T) {
	for _, tc := range []struct {
		now    time.Time
		period TrafficPeriod
		want   string
	}{
		{time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC), TrafficToday, "2026-10-15"},
		{time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC), TrafficWeek, "2026-10-12"},
		{time.Date(2026, 10, 18, 23, 0, 0, 0, time.UTC), TrafficWeek, "2026-10-12"},
		{time.Date(2026, 10, 19, 0, 30, 0, 0, time.UTC), TrafficWeek, "2026-10-19"},
		{time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC), TrafficMonth, "2026-10-01"},
	} {
		if got := tc.period.Start(tc.now).Format("2006-01-02"); got != tc.want {
			t.Errorf("%s of %s starts %s, want %s", tc.period, tc.now.Format(time.DateTime), got, tc.want)
		}
	}
}

func TestParseTrafficPeriod(t *testing.T) {
	for in, want := range map[string]TrafficPeriod{"": TrafficMonth, "today": TrafficToday, "week": TrafficWeek, "month": TrafficMonth} {
		if got, err := ParseTrafficPeriod(in); err != nil || got != want {
			t.Errorf("ParseTrafficPeriod(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := ParseTrafficPeriod("year"); err == nil {
		t.Error("ParseTrafficPeriod(year) accepted a period the page does not offer")
	}
}

// Rankings sum the period's days only, list every host and person (idle ones last)
// and leave out a deleted host or person whose days are still kept.
func TestOverviewRanksHostsAndPeopleOverThePeriod(t *testing.T) {
	setupConflictDB(t)
	edge := seedOverviewNode(t, "edge", "Edge")
	idle := seedOverviewNode(t, "idle", "")
	seedHostDays(t,
		model.HostDailyTraffic{NodeId: 0, Day: 20261001, Up: 1 * planGiB},
		model.HostDailyTraffic{NodeId: 0, Day: 20261013, Down: 2 * planGiB},
		model.HostDailyTraffic{NodeId: 0, Day: 20261015, Up: 4 * planGiB},
		model.HostDailyTraffic{NodeId: edge, Day: 20260930, Down: 8 * planGiB},
		model.HostDailyTraffic{NodeId: edge, Day: 20261014, Down: 5 * planGiB},
		model.HostDailyTraffic{NodeId: 99, Day: 20261015, Up: 50 * planGiB},
	)
	for _, email := range []string{"amy@rank", "bob@rank", "cat@rank"} {
		seedTrafficClient(t, email, 0, 0, 0)
	}
	seedClientDays(t,
		model.ClientDailyTraffic{Email: "amy@rank", Day: 20261002, Up: 3 * planGiB},
		model.ClientDailyTraffic{Email: "bob@rank", Day: 20261015, Down: 1 * planGiB},
		model.ClientDailyTraffic{Email: "bob@rank", Day: 20260930, Down: 9 * planGiB},
		model.ClientDailyTraffic{Email: "left@rank", Day: 20261015, Up: 70 * planGiB},
	)
	now := time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)
	s := &TrafficStatsService{}

	rank := func(period TrafficPeriod) (string, string) {
		t.Helper()
		ov, err := s.Overview(now, 7, period, ProbeOverview{})
		if err != nil {
			t.Fatalf("overview %s: %v", period, err)
		}
		var hosts, users []string
		for _, h := range ov.HostRanking {
			hosts = append(hosts, fmt.Sprintf("%d:%d/%d", h.NodeId, h.Up/planGiB, h.Down/planGiB))
		}
		for _, u := range ov.UserRanking {
			users = append(users, fmt.Sprintf("%s:%d/%d", u.Email, u.Up/planGiB, u.Down/planGiB))
		}
		if ov.Users != 3 {
			t.Errorf("%s: users = %d, want 3", period, ov.Users)
		}
		return strings.Join(hosts, " "), strings.Join(users, " ")
	}
	for _, tc := range []struct {
		period            TrafficPeriod
		wantHosts, wantUs string
	}{
		{TrafficMonth, fmt.Sprintf("0:5/2 %d:0/5 %d:0/0", edge, idle), "amy@rank:3/0 bob@rank:0/1 cat@rank:0/0"},
		{TrafficWeek, fmt.Sprintf("0:4/2 %d:0/5 %d:0/0", edge, idle), "bob@rank:0/1 amy@rank:0/0 cat@rank:0/0"},
		{TrafficToday, fmt.Sprintf("0:4/0 %d:0/0 %d:0/0", edge, idle), "bob@rank:0/1 amy@rank:0/0 cat@rank:0/0"},
	} {
		hosts, users := rank(tc.period)
		if hosts != tc.wantHosts {
			t.Errorf("%s host ranking = %s, want %s", tc.period, hosts, tc.wantHosts)
		}
		if users != tc.wantUs {
			t.Errorf("%s user ranking = %s, want %s", tc.period, users, tc.wantUs)
		}
	}
}

// The page shows the top of the user ranking and a count, so a big panel sends a
// bounded list rather than every client it has.
func TestOverviewCapsTheUserRanking(t *testing.T) {
	setupConflictDB(t)
	for i := range userRankingCap + 1 {
		seedTrafficClient(t, fmt.Sprintf("u%03d@cap", i), 0, 0, 0)
	}
	ov, err := (&TrafficStatsService{}).Overview(statsDay, 1, TrafficMonth, ProbeOverview{})
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if len(ov.UserRanking) != userRankingCap || ov.Users != userRankingCap+1 {
		t.Fatalf("user ranking has %d of %d users, want %d of %d", len(ov.UserRanking), ov.Users, userRankingCap, userRankingCap+1)
	}
}

func TestOverviewFillsMissingDaysOfTheDailyChart(t *testing.T) {
	setupConflictDB(t)
	seedClientDays(t,
		model.ClientDailyTraffic{Email: "a@stats", Day: 20261001, Up: 1, Down: 2},
		model.ClientDailyTraffic{Email: "c@stats", Day: 20261001, Up: 10, Down: 20},
		model.ClientDailyTraffic{Email: "a@stats", Day: 20260929, Up: 5, Down: 5},
	)
	ov, err := (&TrafficStatsService{}).Overview(statsDay, 7, TrafficMonth, ProbeOverview{})
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if len(ov.Daily) != 7 || ov.Daily[0].Day != "2026-09-25" || ov.Daily[6].Day != "2026-10-01" {
		t.Fatalf("daily = %+v, want 7 days from 2026-09-25 to 2026-10-01", ov.Daily)
	}
	if ov.Daily[6].Up != 11 || ov.Daily[6].Down != 22 || ov.Daily[4].Up != 5 || ov.Daily[5].Up != 0 {
		t.Fatalf("daily = %+v, want today summed across clients, 09-29 kept and 09-30 zero", ov.Daily)
	}
}

// The home page's schema rejects null, so an empty panel must still send lists.
func TestOverviewOfAnEmptyPanelSendsEmptyLists(t *testing.T) {
	setupConflictDB(t)
	ov, err := (&TrafficStatsService{}).Overview(statsDay, 3, TrafficToday, ProbeOverview{})
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	raw, err := json.Marshal(ov)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{
		`"userRanking":[]`, `"hostRanking":[{"nodeId":0,"name":"","up":0,"down":0}]`,
		`"period":"today","periodStart":"2026-10-01"`, `"day":"2026-09-29","up":0,"down":0`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("overview JSON %s lacks %s", raw, want)
		}
	}
}
