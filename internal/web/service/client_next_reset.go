package service

import (
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func (s *ClientService) NextReset(rec model.ClientRecord, now time.Time) (int64, error) {
	var traffic xray.ClientTraffic
	if err := database.GetDB().Select("reset_count").Where("email = ?", rec.Email).Find(&traffic).Error; err != nil {
		return 0, err
	}
	return nextClientReset(rec, traffic.ResetCount, now), nil
}

// nextClientReset is when a client's usage next returns to zero: its reset cycle
// or an auto-renewal at expiry, whichever is first; 0 when neither is scheduled.
func nextClientReset(rec model.ClientRecord, resetCount int, now time.Time) int64 {
	next := nextCycleReset(rec.TrafficReset, rec.TrafficResetDay, now)
	if renewsAtExpiry(rec, resetCount) && rec.ExpiryTime > now.UnixMilli() {
		if next == 0 || rec.ExpiryTime < next {
			next = rec.ExpiryTime
		}
	}
	return next
}

func renewsAtExpiry(rec model.ClientRecord, resetCount int) bool {
	if rec.Reset <= 0 && rec.ResetDay <= 0 && rec.ResetWeekday <= 0 {
		return false
	}
	return rec.ResetMax == 0 || resetCount < rec.ResetMax
}

// nextCycleReset follows the cron specs web.go registers: @hourly, @daily,
// @weekly (Sunday midnight), and a daily run that fires on the monthly day.
func nextCycleReset(period string, day int, now time.Time) int64 {
	loc := now.Location()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	switch period {
	case "hourly":
		return time.Date(now.Year(), now.Month(), now.Day(), now.Hour()+1, 0, 0, 0, loc).UnixMilli()
	case "daily":
		return midnight.AddDate(0, 0, 1).UnixMilli()
	case "weekly":
		days := (7 - int(now.Weekday())) % 7
		if days == 0 {
			days = 7
		}
		return midnight.AddDate(0, 0, days).UnixMilli()
	case "monthly":
		day = max(day, 1)
		for ahead := range 2 {
			first := time.Date(now.Year(), now.Month()+time.Month(ahead), 1, 0, 0, 0, 0, loc)
			at := time.Date(first.Year(), first.Month(), min(day, first.AddDate(0, 1, -1).Day()), 0, 0, 0, 0, loc)
			if at.After(now) {
				return at.UnixMilli()
			}
		}
	}
	return 0
}
