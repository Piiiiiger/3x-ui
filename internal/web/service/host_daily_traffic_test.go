package service

import (
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// seedTrafficInbound adds an inbound with its counters; nodeID 0 runs it on the panel itself.
func seedTrafficInbound(t *testing.T, tag string, port, nodeID int, up, down int64) *model.Inbound {
	t.Helper()
	ib := &model.Inbound{
		UserId: 1, Tag: tag, Remark: tag, Enable: true, Port: port, Protocol: model.VLESS,
		Settings: `{"clients":[]}`, StreamSettings: `{}`, Up: up, Down: down,
	}
	if nodeID > 0 {
		ib.NodeID = &nodeID
	}
	if err := database.GetDB().Create(ib).Error; err != nil {
		t.Fatalf("seed inbound %s: %v", tag, err)
	}
	return ib
}

func setInboundCounters(t *testing.T, ib *model.Inbound, up, down int64) {
	t.Helper()
	if err := database.GetDB().Model(&model.Inbound{}).Where("id = ?", ib.Id).
		Updates(map[string]any{"up": up, "down": down}).Error; err != nil {
		t.Fatalf("set counters of %s: %v", ib.Tag, err)
	}
}

func hostDailyRow(t *testing.T, nodeID, day int) (model.HostDailyTraffic, bool) {
	t.Helper()
	var rows []model.HostDailyTraffic
	if err := database.GetDB().Where("node_id = ? AND day = ?", nodeID, day).Find(&rows).Error; err != nil {
		t.Fatalf("read host day: %v", err)
	}
	if len(rows) == 0 {
		return model.HostDailyTraffic{}, false
	}
	return rows[0], true
}

func TestRecordDailyFirstSightOfAnInboundIsOnlyABaseline(t *testing.T) {
	setupConflictDB(t)
	seedTrafficInbound(t, "old-in", 20001, 0, 5*planGiB, 7*planGiB)
	if err := (&TrafficStatsService{}).RecordDaily(statsDay); err != nil {
		t.Fatalf("record: %v", err)
	}
	if row, ok := hostDailyRow(t, 0, 20261001); ok {
		t.Fatalf("first run credited %d/%d to the panel's host, want nothing", row.Up, row.Down)
	}
}

func TestRecordDailyCreditsEachInboundsGrowthToItsHost(t *testing.T) {
	setupConflictDB(t)
	localA := seedTrafficInbound(t, "local-a", 20001, 0, 1*planGiB, 1*planGiB)
	localB := seedTrafficInbound(t, "local-b", 20002, 0, 0, 0)
	edge := seedTrafficInbound(t, "edge", 20003, 3, 2*planGiB, 2*planGiB)
	s := &TrafficStatsService{}
	if err := s.RecordDaily(statsDay); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	setInboundCounters(t, localA, 2*planGiB, 1*planGiB)
	setInboundCounters(t, localB, 0, 2*planGiB)
	setInboundCounters(t, edge, 2*planGiB, 5*planGiB)
	if err := s.RecordDaily(statsDay.Add(10 * time.Minute)); err != nil {
		t.Fatalf("record: %v", err)
	}
	if row, ok := hostDailyRow(t, 0, 20261001); !ok || row.Up != 1*planGiB || row.Down != 2*planGiB {
		t.Errorf("panel's host today = %+v (found %v), want both its inbounds: 1 GiB up, 2 GiB down", row, ok)
	}
	if row, ok := hostDailyRow(t, 3, 20261001); !ok || row.Up != 0 || row.Down != 3*planGiB {
		t.Errorf("node 3 today = %+v (found %v), want 3 GiB down", row, ok)
	}
}

func TestRecordDailyTreatsAnInboundCounterDropAsAReset(t *testing.T) {
	setupConflictDB(t)
	ib := seedTrafficInbound(t, "reset-in", 20001, 2, 5*planGiB, 5*planGiB)
	s := &TrafficStatsService{}
	if err := s.RecordDaily(statsDay); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	setInboundCounters(t, ib, 1*planGiB, 0)
	if err := s.RecordDaily(statsDay.Add(10 * time.Minute)); err != nil {
		t.Fatalf("record: %v", err)
	}
	if row, ok := hostDailyRow(t, 2, 20261001); !ok || row.Up != 1*planGiB || row.Down != 0 {
		t.Fatalf("node 2 today = %+v (found %v), want the 1 GiB used since the reset", row, ok)
	}
}

// A deleted inbound takes its mark along, so it neither counts again nor lowers
// what its host's other inbounds used.
func TestRecordDailyForgetsADeletedInbound(t *testing.T) {
	setupConflictDB(t)
	kept := seedTrafficInbound(t, "kept", 20001, 4, 1*planGiB, 0)
	gone := seedTrafficInbound(t, "gone", 20002, 4, 9*planGiB, 0)
	s := &TrafficStatsService{}
	if err := s.RecordDaily(statsDay); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if err := database.GetDB().Delete(&model.Inbound{}, gone.Id).Error; err != nil {
		t.Fatalf("delete inbound: %v", err)
	}
	setInboundCounters(t, kept, 2*planGiB, 0)
	if err := s.RecordDaily(statsDay.Add(10 * time.Minute)); err != nil {
		t.Fatalf("record: %v", err)
	}
	if row, ok := hostDailyRow(t, 4, 20261001); !ok || row.Up != 1*planGiB {
		t.Errorf("node 4 today = %+v (found %v), want only the kept inbound's 1 GiB", row, ok)
	}
	var marks int64
	if err := database.GetDB().Model(&model.InboundTrafficMark{}).Where("inbound_id = ?", gone.Id).Count(&marks).Error; err != nil || marks != 0 {
		t.Errorf("the deleted inbound's mark: %d rows (err %v), want none", marks, err)
	}
}

func TestRecordDailyPrunesHostDaysPastTheWindow(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	for _, day := range []int{20260601, 20260920} {
		if err := db.Create(&model.HostDailyTraffic{NodeId: 5, Day: day, Up: 1}).Error; err != nil {
			t.Fatalf("seed day %d: %v", day, err)
		}
	}
	if err := (&TrafficStatsService{}).RecordDaily(statsDay); err != nil {
		t.Fatalf("record: %v", err)
	}
	if _, ok := hostDailyRow(t, 5, 20260601); ok {
		t.Error("a host day 122 days old survived the 90-day window")
	}
	if _, ok := hostDailyRow(t, 5, 20260920); !ok {
		t.Error("a host day 11 days old was pruned")
	}
}
