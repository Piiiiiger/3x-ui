package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func seedTrafficClient(t *testing.T, email string, quota, up, down int64) {
	t.Helper()
	db := database.GetDB()
	if err := db.Create(&model.ClientRecord{Email: email, SubID: "sub-" + email, TotalGB: quota, Enable: true}).Error; err != nil {
		t.Fatalf("seed client %s: %v", email, err)
	}
	if err := db.Create(&xray.ClientTraffic{Email: email, Up: up, Down: down, Total: quota, Enable: true}).Error; err != nil {
		t.Fatalf("seed traffic %s: %v", email, err)
	}
}

func setTrafficCounters(t *testing.T, email string, up, down int64) {
	t.Helper()
	if err := database.GetDB().Model(&xray.ClientTraffic{}).Where("email = ?", email).
		Updates(map[string]any{"up": up, "down": down}).Error; err != nil {
		t.Fatalf("set counters %s: %v", email, err)
	}
}

func dailyRow(t *testing.T, email string, day int) (model.ClientDailyTraffic, bool) {
	t.Helper()
	var rows []model.ClientDailyTraffic
	if err := database.GetDB().Where("email = ? AND day = ?", email, day).Find(&rows).Error; err != nil {
		t.Fatalf("read daily row: %v", err)
	}
	if len(rows) == 0 {
		return model.ClientDailyTraffic{}, false
	}
	return rows[0], true
}

var statsDay = time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)

func TestRecordDailyFirstSightIsOnlyABaseline(t *testing.T) {
	setupConflictDB(t)
	seedTrafficClient(t, "old@stats", 0, 5*planGiB, 7*planGiB)
	if err := (&TrafficStatsService{}).RecordDaily(statsDay); err != nil {
		t.Fatalf("record: %v", err)
	}
	// Usage from before the history existed must not land on the first day.
	if row, ok := dailyRow(t, "old@stats", 20261001); ok {
		t.Fatalf("first run recorded %d/%d for today, want nothing", row.Up, row.Down)
	}
}

func TestRecordDailyAddsGrowthSinceTheLastRunToToday(t *testing.T) {
	setupConflictDB(t)
	seedTrafficClient(t, "grow@stats", 0, 5*planGiB, 7*planGiB)
	s := &TrafficStatsService{}
	if err := s.RecordDaily(statsDay); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	setTrafficCounters(t, "grow@stats", 6*planGiB, 9*planGiB)
	if err := s.RecordDaily(statsDay.Add(10 * time.Minute)); err != nil {
		t.Fatalf("record: %v", err)
	}
	setTrafficCounters(t, "grow@stats", 7*planGiB, 9*planGiB)
	if err := s.RecordDaily(statsDay.Add(20 * time.Minute)); err != nil {
		t.Fatalf("record: %v", err)
	}
	row, ok := dailyRow(t, "grow@stats", 20261001)
	if !ok || row.Up != 2*planGiB || row.Down != 2*planGiB {
		t.Fatalf("today = %+v (found %v), want 2 GiB up and 2 GiB down", row, ok)
	}
}

func TestRecordDailyTreatsACounterDropAsAReset(t *testing.T) {
	setupConflictDB(t)
	seedTrafficClient(t, "reset@stats", 0, 5*planGiB, 5*planGiB)
	s := &TrafficStatsService{}
	if err := s.RecordDaily(statsDay); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	setTrafficCounters(t, "reset@stats", 1*planGiB, 0)
	if err := s.RecordDaily(statsDay.Add(10 * time.Minute)); err != nil {
		t.Fatalf("record: %v", err)
	}
	row, ok := dailyRow(t, "reset@stats", 20261001)
	if !ok || row.Up != 1*planGiB || row.Down != 0 {
		t.Fatalf("today = %+v (found %v), want the 1 GiB used since the reset", row, ok)
	}
}

func TestRecordDailyStartsANewRowEachDay(t *testing.T) {
	setupConflictDB(t)
	seedTrafficClient(t, "days@stats", 0, 0, 0)
	s := &TrafficStatsService{}
	lateEvening := time.Date(2026, 10, 1, 23, 50, 0, 0, time.UTC)
	if err := s.RecordDaily(lateEvening); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	setTrafficCounters(t, "days@stats", 3*planGiB, 0)
	if err := s.RecordDaily(lateEvening.Add(5 * time.Minute)); err != nil {
		t.Fatalf("record day one: %v", err)
	}
	setTrafficCounters(t, "days@stats", 4*planGiB, 0)
	if err := s.RecordDaily(lateEvening.Add(15 * time.Minute)); err != nil {
		t.Fatalf("record day two: %v", err)
	}
	first, _ := dailyRow(t, "days@stats", 20261001)
	second, _ := dailyRow(t, "days@stats", 20261002)
	if first.Up != 3*planGiB || second.Up != 1*planGiB {
		t.Fatalf("day one %d, day two %d; want 3 GiB then 1 GiB", first.Up, second.Up)
	}
}

func TestRecordDailyPrunesDaysPastTheWindow(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	for _, day := range []int{20260601, 20260920} {
		if err := db.Create(&model.ClientDailyTraffic{Email: "gone@stats", Day: day, Up: 1}).Error; err != nil {
			t.Fatalf("seed day %d: %v", day, err)
		}
	}
	if err := (&TrafficStatsService{}).RecordDaily(statsDay); err != nil {
		t.Fatalf("record: %v", err)
	}
	if _, ok := dailyRow(t, "gone@stats", 20260601); ok {
		t.Fatal("a day 122 days old survived the 90-day window")
	}
	if _, ok := dailyRow(t, "gone@stats", 20260920); !ok {
		t.Fatal("a day 11 days old was pruned")
	}
}

func TestOverviewTotalsLimitedClientsAndFillsMissingDays(t *testing.T) {
	setupConflictDB(t)
	seedTrafficClient(t, "a@stats", 100*planGiB, 10*planGiB, 20*planGiB)
	seedTrafficClient(t, "b@stats", 50*planGiB, 30*planGiB, 30*planGiB)
	seedTrafficClient(t, "c@stats", 0, 4*planGiB, 6*planGiB)
	db := database.GetDB()
	for _, d := range []model.ClientDailyTraffic{
		{Email: "a@stats", Day: 20261001, Up: 1, Down: 2},
		{Email: "c@stats", Day: 20261001, Up: 10, Down: 20},
		{Email: "a@stats", Day: 20260929, Up: 5, Down: 5},
	} {
		if err := db.Create(&d).Error; err != nil {
			t.Fatalf("seed daily: %v", err)
		}
	}

	ov, err := (&TrafficStatsService{}).Overview(statsDay, 7)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	// b is over its 50 GiB, so it adds nothing to what is left; c has no quota.
	if ov.QuotaBytes != 150*planGiB || ov.RemainingBytes != 70*planGiB || ov.UsedBytes != 100*planGiB {
		t.Fatalf("totals = %+v, want quota 150, remaining 70, used 100 GiB", ov)
	}
	if len(ov.Daily) != 7 || ov.Daily[0].Day != "2026-09-25" || ov.Daily[6].Day != "2026-10-01" {
		t.Fatalf("daily = %+v, want 7 days from 2026-09-25 to 2026-10-01", ov.Daily)
	}
	if ov.Daily[6].Up != 11 || ov.Daily[6].Down != 22 || ov.Daily[4].Up != 5 || ov.Daily[5].Up != 0 {
		t.Fatalf("daily = %+v, want today summed across clients, 09-29 kept and 09-30 zero", ov.Daily)
	}
}

func TestOverviewListsWhoNeedsAttentionMostUrgentFirst(t *testing.T) {
	setupConflictDB(t)
	now := statsDay.UnixMilli()
	day := int64(24 * time.Hour / time.Millisecond)
	clients := []struct {
		email           string
		quota, used     int64
		expiry          int64
		enable          bool
		wantInAttention bool
	}{
		{"fine@stats", 100 * planGiB, 10 * planGiB, now + 60*day, true, false},
		{"gonelong@stats", 0, 0, now - 30*day, true, true},
		{"low@stats", 100 * planGiB, 95 * planGiB, 0, true, true},
		{"later@stats", 0, 0, now + 5*day, true, true},
		{"off@stats", 0, 0, 0, false, false},
		{"out@stats", 50 * planGiB, 60 * planGiB, 0, true, true},
		{"firstuse@stats", 0, 0, -30 * day, true, false},
		{"gone@stats", 0, 0, now - day, true, true},
		{"soon@stats", 0, 0, now + 2*day, true, true},
	}
	db := database.GetDB()
	for _, c := range clients {
		seedTrafficClient(t, c.email, c.quota, c.used, 0)
		if err := db.Model(&model.ClientRecord{}).Where("email = ?", c.email).
			Updates(map[string]any{"expiry_time": c.expiry, "enable": c.enable}).Error; err != nil {
			t.Fatalf("set %s: %v", c.email, err)
		}
	}

	ov, err := (&TrafficStatsService{}).Overview(statsDay, 1)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	var got []string
	for _, a := range ov.Attention {
		got = append(got, a.Email+":"+a.Status)
	}
	// Dated endings come before the client that only runs low on traffic.
	want := []string{
		"soon@stats:expiring", "later@stats:expiring", "low@stats:expiring",
		"out@stats:usedUp", "gone@stats:expired", "gonelong@stats:expired",
	}
	if len(got) != len(want) {
		t.Fatalf("attention = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("attention = %v, want %v", got, want)
		}
	}
	counts := []int{ov.Clients, ov.Active, ov.Expiring, ov.UsedUp, ov.Expired, ov.Disabled, ov.Unlimited}
	wantCounts := []int{9, 5, 3, 1, 2, 1, 6}
	for i := range counts {
		if counts[i] != wantCounts[i] {
			t.Fatalf("clients/active/expiring/usedUp/expired/disabled/unlimited = %v, want %v", counts, wantCounts)
		}
	}
}

// The home page's schema rejects null, so an empty panel must still send lists.
func TestOverviewOfAnEmptyPanelSendsEmptyLists(t *testing.T) {
	setupConflictDB(t)
	ov, err := (&TrafficStatsService{}).Overview(statsDay, 3)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	raw, err := json.Marshal(ov)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{`"attention":[]`, `"day":"2026-09-29","up":0,"down":0`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("overview JSON %s lacks %s", raw, want)
		}
	}
}
