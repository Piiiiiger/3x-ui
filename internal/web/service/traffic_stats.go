package service

import (
	"math"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// TrafficStatsService keeps a per-day history of client traffic and sums up
// quotas, usage and the clients that need attention for the overview page.
type TrafficStatsService struct{}

// TrafficDay is the traffic all clients used on one day of the panel's time zone.
type TrafficDay struct {
	Day  string `json:"day" example:"2026-10-01"`
	Up   int64  `json:"up" example:"1048576"`
	Down int64  `json:"down" example:"4194304"`
}

// AttentionClient is a client that ran out, or soon will. Status is expiring,
// usedUp or expired.
type AttentionClient struct {
	Email      string `json:"email" example:"alice"`
	PlanId     int    `json:"planId" example:"1"`
	Status     string `json:"status" example:"expiring"`
	ExpiryTime int64  `json:"expiryTime" example:"1735689600000"`
	TotalGB    int64  `json:"totalGB" example:"107374182400"`
	Used       int64  `json:"used" example:"53687091200"`
	Enable     bool   `json:"enable" example:"true"`
}

// TrafficOverview sums every client. Quota and remaining cover clients with a
// quota only, so their ratio is the share of sold traffic already consumed.
type TrafficOverview struct {
	QuotaBytes     int64             `json:"quotaBytes" example:"1099511627776"`
	RemainingBytes int64             `json:"remainingBytes" example:"884763262976"`
	UsedBytes      int64             `json:"usedBytes" example:"322122547200"`
	Clients        int               `json:"clients" example:"12"`
	Unlimited      int               `json:"unlimited" example:"2"`
	Active         int               `json:"active" example:"9"`
	Expiring       int               `json:"expiring" example:"2"`
	UsedUp         int               `json:"usedUp" example:"1"`
	Expired        int               `json:"expired" example:"1"`
	Disabled       int               `json:"disabled" example:"1"`
	Attention      []AttentionClient `json:"attention"`
	Daily          []TrafficDay      `json:"daily"`
}

const (
	trafficDailyKeepDays = 90
	attentionWindowMs    = 7 * 24 * 60 * 60 * 1000
	attentionListCap     = 100
)

func dayNumber(t time.Time) int {
	return t.Year()*10000 + int(t.Month())*100 + t.Day()
}

// counterGrowth reads a drop as a reset: all that is on the counter now is new.
func counterGrowth(prev, cur int64) int64 {
	if cur < prev {
		return cur
	}
	return cur - prev
}

// RecordDaily adds what each client used since the previous run to the day of
// now. A client's first sighting only sets its mark: older usage has no day.
func (s *TrafficStatsService) RecordDaily(now time.Time) error {
	day := dayNumber(now)
	return runSerializedTx(func(tx *gorm.DB) error {
		q := newClientQuery(tx, now.UnixMilli(), 0, 0)
		var counters []model.ClientTrafficMark
		if err := q.from().
			Select("c.email AS email, " + q.upExpr + " AS up, " + q.downExpr + " AS down").
			Scan(&counters).Error; err != nil {
			return err
		}
		var marks []model.ClientTrafficMark
		if err := tx.Find(&marks).Error; err != nil {
			return err
		}
		lastSeen := make(map[string]model.ClientTrafficMark, len(marks))
		for _, m := range marks {
			lastSeen[m.Email] = m
		}

		var growth []model.ClientDailyTraffic
		var moved []model.ClientTrafficMark
		for _, cur := range counters {
			prev, seen := lastSeen[cur.Email]
			if seen && prev.Up == cur.Up && prev.Down == cur.Down {
				continue
			}
			moved = append(moved, cur)
			if !seen {
				continue
			}
			up, down := counterGrowth(prev.Up, cur.Up), counterGrowth(prev.Down, cur.Down)
			if up > 0 || down > 0 {
				growth = append(growth, model.ClientDailyTraffic{Email: cur.Email, Day: day, Up: up, Down: down})
			}
		}

		if len(growth) > 0 {
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "email"}, {Name: "day"}},
				DoUpdates: clause.Assignments(map[string]any{
					"up":   gorm.Expr("client_daily_traffics.up + excluded.up"),
					"down": gorm.Expr("client_daily_traffics.down + excluded.down"),
				}),
			}).CreateInBatches(growth, 200).Error; err != nil {
				return err
			}
		}
		if len(moved) > 0 {
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "email"}},
				DoUpdates: clause.AssignmentColumns([]string{"up", "down"}),
			}).CreateInBatches(moved, 200).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec("DELETE FROM client_traffic_marks WHERE email NOT IN (SELECT email FROM clients)").Error; err != nil {
			return err
		}
		cutoff := dayNumber(now.AddDate(0, 0, -trafficDailyKeepDays))
		return tx.Where("day < ?", cutoff).Delete(&model.ClientDailyTraffic{}).Error
	})
}

// Overview totals the clients and lists the last days of traffic, the final
// entry being the day of now.
func (s *TrafficStatsService) Overview(now time.Time, days int) (*TrafficOverview, error) {
	db := database.GetDB()
	q := newClientQuery(db, now.UnixMilli(), 0, 0)
	used := q.usedExpr
	expired := "(c.expiry_time > 0 AND c.expiry_time <= " + sqlInt(q.nowMs) + ")"
	usedUp := "(c.total_gb > 0 AND " + used + " >= c.total_gb)"
	runningLow := "(" + sqlClientEnabled + " AND NOT " + q.depletedExpr() + " AND (" +
		"(c.expiry_time > 0 AND c.expiry_time - " + sqlInt(q.nowMs) + " < " + sqlInt(attentionWindowMs) + ")" +
		" OR (c.total_gb > 0 AND (c.total_gb - " + used + ") * 10 < c.total_gb)))"
	sum := func(cond string) string {
		return "COALESCE(SUM(CASE WHEN " + cond + " THEN 1 ELSE 0 END), 0)"
	}

	var totals struct {
		Clients, Unlimited, Active, Expiring, UsedUp, Expired, Disabled int
		QuotaBytes, RemainingBytes, UsedBytes                           int64
	}
	if err := q.from().Select(
		"COUNT(*) AS clients," +
			" COALESCE(SUM(CASE WHEN c.total_gb > 0 THEN c.total_gb ELSE 0 END), 0) AS quota_bytes," +
			" COALESCE(SUM(CASE WHEN c.total_gb > " + used + " THEN c.total_gb - " + used + " ELSE 0 END), 0) AS remaining_bytes," +
			" COALESCE(SUM(" + used + "), 0) AS used_bytes," +
			" " + sum("c.total_gb <= 0") + " AS unlimited," +
			" " + sum("("+sqlClientEnabled+" AND NOT "+q.depletedExpr()+")") + " AS active," +
			" " + sum(runningLow) + " AS expiring," +
			" " + sum(expired) + " AS expired," +
			" " + sum("("+usedUp+" AND NOT "+expired+")") + " AS used_up," +
			" " + sum(q.deactiveExpr()) + " AS disabled",
	).Scan(&totals).Error; err != nil {
		return nil, err
	}
	ov := &TrafficOverview{
		QuotaBytes: totals.QuotaBytes, RemainingBytes: totals.RemainingBytes, UsedBytes: totals.UsedBytes,
		Clients: totals.Clients, Unlimited: totals.Unlimited, Active: totals.Active, Expiring: totals.Expiring,
		UsedUp: totals.UsedUp, Expired: totals.Expired, Disabled: totals.Disabled,
		Attention: []AttentionClient{},
	}

	// Running low first, soonest end first and undated last; then used up; then the
	// most recently expired.
	rank := "CASE WHEN " + expired + " THEN 2 WHEN " + usedUp + " THEN 1 ELSE 0 END"
	endsAt := "CASE WHEN " + expired + " THEN -c.expiry_time WHEN c.expiry_time > 0 THEN c.expiry_time ELSE " + sqlInt(math.MaxInt64) + " END"
	if err := q.from().
		Select("c.email AS email, COALESCE(c.plan_id, 0) AS plan_id, c.expiry_time AS expiry_time," +
			" c.total_gb AS total_gb, " + used + " AS used, " + sqlClientEnabled + " AS enable," +
			" CASE WHEN " + expired + " THEN 'expired' WHEN " + usedUp + " THEN 'usedUp' ELSE 'expiring' END AS status").
		Where(expired + " OR " + usedUp + " OR " + runningLow).
		Order(rank + ", " + endsAt + ", c.email").
		Limit(attentionListCap).
		Scan(&ov.Attention).Error; err != nil {
		return nil, err
	}

	daily, err := dailyTraffic(db, now, days)
	if err != nil {
		return nil, err
	}
	ov.Daily = daily
	return ov, nil
}

func dailyTraffic(db *gorm.DB, now time.Time, days int) ([]TrafficDay, error) {
	if days < 1 {
		days = 1
	}
	first := now.AddDate(0, 0, -(days - 1))
	var rows []model.ClientDailyTraffic
	if err := db.Model(&model.ClientDailyTraffic{}).
		Select("day, SUM(up) AS up, SUM(down) AS down").
		Where("day >= ? AND day <= ?", dayNumber(first), dayNumber(now)).
		Group("day").Scan(&rows).Error; err != nil {
		return nil, err
	}
	byDay := make(map[int]model.ClientDailyTraffic, len(rows))
	for _, r := range rows {
		byDay[r.Day] = r
	}
	out := make([]TrafficDay, 0, days)
	for i := range days {
		d := first.AddDate(0, 0, i)
		r := byDay[dayNumber(d)]
		out = append(out, TrafficDay{Day: d.Format("2006-01-02"), Up: r.Up, Down: r.Down})
	}
	return out, nil
}
