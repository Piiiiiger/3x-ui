package service

import (
	"cmp"
	"maps"
	"slices"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
)

// TrafficStatsService keeps a per-day history of client and host traffic and
// builds the traffic overview page from it and from the probe's quotas.
type TrafficStatsService struct{}

// TrafficDay is the traffic all clients used on one day of the panel's time zone.
type TrafficDay struct {
	Day  string `json:"day" example:"2026-10-01"`
	Up   int64  `json:"up" example:"1048576"`
	Down int64  `json:"down" example:"4194304"`
}

// TrafficServers adds up the quota and billing-cycle use Lite reports for the
// hosts linked to it. A host without a quota counts as unlimited.
type TrafficServers struct {
	Configured     bool   `json:"configured" example:"true"`
	Error          string `json:"error" example:""`
	QuotaBytes     int64  `json:"quotaBytes" example:"5529664757760"`
	UsedBytes      int64  `json:"usedBytes" example:"505833635840"`
	RemainingBytes int64  `json:"remainingBytes" example:"5023831121920"`
	Unlimited      int    `json:"unlimited" example:"2"`
	Unlinked       int    `json:"unlinked" example:"0"`
}

// TrafficHost is one host of the panel (node id 0 is the panel itself) with the
// quota and cycle use of its Lite server; a quota of 0 is unlimited.
type TrafficHost struct {
	NodeId     int    `json:"nodeId" example:"2"`
	Name       string `json:"name" example:"edge-hk"`
	Linked     bool   `json:"linked" example:"true"`
	QuotaBytes int64  `json:"quotaBytes" example:"1073741824000"`
	UsedBytes  int64  `json:"usedBytes" example:"44023414784"`
}

// HostTraffic is what one host's inbounds carried in the overview's period.
type HostTraffic struct {
	NodeId int    `json:"nodeId" example:"2"`
	Name   string `json:"name" example:"edge-hk"`
	Up     int64  `json:"up" example:"1048576"`
	Down   int64  `json:"down" example:"4194304"`
}

// UserTraffic is what one client used in the overview's period.
type UserTraffic struct {
	Email string `json:"email" example:"alice"`
	Up    int64  `json:"up" example:"1048576"`
	Down  int64  `json:"down" example:"4194304"`
}

// TrafficOverview is the home page: the servers' quotas, the daily chart and the
// period's rankings, busiest first. Users counts every client, ranked or not.
type TrafficOverview struct {
	Servers     TrafficServers `json:"servers"`
	Hosts       []TrafficHost  `json:"hosts"`
	Daily       []TrafficDay   `json:"daily"`
	Period      string         `json:"period" validate:"oneof=today week month" example:"month"`
	PeriodStart string         `json:"periodStart" example:"2026-10-01"`
	HostRanking []HostTraffic  `json:"hostRanking"`
	UserRanking []UserTraffic  `json:"userRanking"`
	Users       int            `json:"users" example:"10"`
}

// TrafficPeriod is the span the rankings cover: from the start of the day, the
// week (Monday) or the month, in the panel's time zone, until now.
type TrafficPeriod string

const (
	TrafficToday TrafficPeriod = "today"
	TrafficWeek  TrafficPeriod = "week"
	TrafficMonth TrafficPeriod = "month"
)

// ParseTrafficPeriod reads the page's period switch; empty means the month.
func ParseTrafficPeriod(s string) (TrafficPeriod, error) {
	switch p := TrafficPeriod(s); p {
	case "":
		return TrafficMonth, nil
	case TrafficToday, TrafficWeek, TrafficMonth:
		return p, nil
	}
	return "", common.NewErrorf("unknown traffic period %q: use today, week or month", s)
}

// Start is midnight of the period's first day in now's time zone.
func (p TrafficPeriod) Start(now time.Time) time.Time {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch p {
	case TrafficWeek:
		return day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
	case TrafficMonth:
		return day.AddDate(0, 0, 1-day.Day())
	}
	return day
}

const (
	trafficDailyKeepDays = 90
	// The page shows the top of the user ranking and its full list in a dialog.
	userRankingCap = 100
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

// RecordDaily adds what each client and each host used since the previous run to
// the day of now. A first sighting only sets a mark: older usage has no day.
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
		if err := recordHostDaily(tx, day); err != nil {
			return err
		}
		cutoff := dayNumber(now.AddDate(0, 0, -trafficDailyKeepDays))
		if err := tx.Where("day < ?", cutoff).Delete(&model.ClientDailyTraffic{}).Error; err != nil {
			return err
		}
		return tx.Where("day < ?", cutoff).Delete(&model.HostDailyTraffic{}).Error
	})
}

// recordHostDaily credits each inbound's growth since the last run to the host it
// runs on, node id 0 being the panel itself.
func recordHostDaily(tx *gorm.DB, day int) error {
	var counters []struct {
		Id       int
		NodeId   int
		Up, Down int64
	}
	if err := tx.Model(&model.Inbound{}).
		Select("id, COALESCE(node_id, 0) AS node_id, up, down").
		Scan(&counters).Error; err != nil {
		return err
	}
	var marks []model.InboundTrafficMark
	if err := tx.Find(&marks).Error; err != nil {
		return err
	}
	lastSeen := make(map[int]model.InboundTrafficMark, len(marks))
	for _, m := range marks {
		lastSeen[m.InboundId] = m
	}

	growth := map[int]*model.HostDailyTraffic{}
	var moved []model.InboundTrafficMark
	for _, cur := range counters {
		prev, seen := lastSeen[cur.Id]
		if seen && prev.Up == cur.Up && prev.Down == cur.Down {
			continue
		}
		moved = append(moved, model.InboundTrafficMark{InboundId: cur.Id, Up: cur.Up, Down: cur.Down})
		if !seen {
			continue
		}
		up, down := counterGrowth(prev.Up, cur.Up), counterGrowth(prev.Down, cur.Down)
		if up == 0 && down == 0 {
			continue
		}
		row := growth[cur.NodeId]
		if row == nil {
			row = &model.HostDailyTraffic{NodeId: cur.NodeId, Day: day}
			growth[cur.NodeId] = row
		}
		row.Up += up
		row.Down += down
	}

	if len(growth) > 0 {
		rows := make([]model.HostDailyTraffic, 0, len(growth))
		for _, nodeID := range slices.Sorted(maps.Keys(growth)) {
			rows = append(rows, *growth[nodeID])
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "node_id"}, {Name: "day"}},
			DoUpdates: clause.Assignments(map[string]any{
				"up":   gorm.Expr("host_daily_traffics.up + excluded.up"),
				"down": gorm.Expr("host_daily_traffics.down + excluded.down"),
			}),
		}).Create(&rows).Error; err != nil {
			return err
		}
	}
	if len(moved) > 0 {
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "inbound_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"up", "down"}),
		}).CreateInBatches(moved, 200).Error; err != nil {
			return err
		}
	}
	return tx.Exec("DELETE FROM inbound_traffic_marks WHERE inbound_id NOT IN (SELECT id FROM inbounds)").Error
}

// Overview builds the home page for now: every host with what the probe says of
// it, the last days of traffic and the period's rankings.
func (s *TrafficStatsService) Overview(now time.Time, days int, period TrafficPeriod, probe ProbeOverview) (*TrafficOverview, error) {
	db := database.GetDB()
	hosts, err := probeHosts(db)
	if err != nil {
		return nil, err
	}
	start := period.Start(now)
	ov := &TrafficOverview{
		Servers:     TrafficServers{Configured: probe.Configured, Error: probe.Error},
		Hosts:       make([]TrafficHost, 0, len(hosts)),
		Period:      string(period),
		PeriodStart: start.Format("2006-01-02"),
	}
	linked := make(map[int]ProbeServer, len(probe.Servers))
	for _, server := range probe.Servers {
		if server.Linked {
			linked[server.NodeId] = server
		}
	}
	for _, host := range hosts {
		row := TrafficHost{NodeId: host.nodeId, Name: host.name}
		if server, ok := linked[host.nodeId]; ok {
			row.Linked, row.QuotaBytes, row.UsedBytes = true, server.TrafficLimit, server.TrafficUsed
			ov.Servers.UsedBytes += server.TrafficUsed
			if server.TrafficLimit > 0 {
				ov.Servers.QuotaBytes += server.TrafficLimit
				ov.Servers.RemainingBytes += max(server.TrafficLimit-server.TrafficUsed, 0)
			} else {
				ov.Servers.Unlimited++
			}
		} else {
			ov.Servers.Unlinked++
		}
		ov.Hosts = append(ov.Hosts, row)
	}

	from, to := dayNumber(start), dayNumber(now)
	if ov.HostRanking, err = hostRanking(db, hosts, from, to); err != nil {
		return nil, err
	}
	if ov.UserRanking, ov.Users, err = userRanking(db, from, to); err != nil {
		return nil, err
	}
	if ov.Daily, err = dailyTraffic(db, now, days); err != nil {
		return nil, err
	}
	return ov, nil
}

// hostRanking sums each host's days from..to, busiest first; a tie keeps the
// hosts' own order, the panel first. Days of a deleted node are left out.
func hostRanking(db *gorm.DB, hosts []probeHost, from, to int) ([]HostTraffic, error) {
	var sums []model.HostDailyTraffic
	if err := db.Model(&model.HostDailyTraffic{}).
		Select("node_id, SUM(up) AS up, SUM(down) AS down").
		Where("day >= ? AND day <= ?", from, to).
		Group("node_id").Scan(&sums).Error; err != nil {
		return nil, err
	}
	byNode := make(map[int]model.HostDailyTraffic, len(sums))
	for _, r := range sums {
		byNode[r.NodeId] = r
	}
	out := make([]HostTraffic, 0, len(hosts))
	for _, host := range hosts {
		r := byNode[host.nodeId]
		out = append(out, HostTraffic{NodeId: host.nodeId, Name: host.name, Up: r.Up, Down: r.Down})
	}
	slices.SortStableFunc(out, func(a, b HostTraffic) int { return cmp.Compare(b.Up+b.Down, a.Up+a.Down) })
	return out, nil
}

// userRanking sums every client's days from..to, busiest first and by email on a
// tie. It returns the first userRankingCap of them and how many clients there are.
func userRanking(db *gorm.DB, from, to int) ([]UserTraffic, int, error) {
	var total int64
	if err := db.Model(&model.ClientRecord{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := make([]UserTraffic, 0, min(total, userRankingCap))
	if err := db.Table("clients AS c").
		Select("c.email AS email, COALESCE(SUM(d.up), 0) AS up, COALESCE(SUM(d.down), 0) AS down").
		Joins("LEFT JOIN client_daily_traffics AS d ON d.email = c.email AND d.day >= ? AND d.day <= ?", from, to).
		Group("c.email").
		Order("COALESCE(SUM(d.up), 0) + COALESCE(SUM(d.down), 0) DESC, c.email ASC").
		Limit(userRankingCap).
		Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, int(total), nil
}

// ClientDaily lists one client's traffic for the last days, ending on the day of now.
func (s *TrafficStatsService) ClientDaily(email string, now time.Time, days int) ([]TrafficDay, error) {
	return dailyTraffic(database.GetDB().Where("email = ?", email), now, days)
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
