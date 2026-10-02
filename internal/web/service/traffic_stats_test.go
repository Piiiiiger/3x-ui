package service

import (
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
