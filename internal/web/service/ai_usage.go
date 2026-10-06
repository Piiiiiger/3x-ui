package service

import (
	"cmp"
	"encoding/json"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
)

// AiUsageService stores the Claude Code and Codex usage that Pigger Switch
// reports from each computer and builds the AI usage page from it.
type AiUsageService struct{}

const (
	aiDailyDays          = 30
	aiProjectMaxRunes    = 400
	aiModelMaxRunes      = 128
	aiTitleMaxRunes      = 200
	aiSessionIdMaxRunes  = 128
	aiDeviceNameMaxRunes = 64
	aiMaxDailyRows       = 200_000
	aiMaxSessions        = 5_000
	aiMaxQuotaTiers      = 16
	aiMaxEstimateWindows = 16
	aiMaxHistoryWindows  = 100
	aiMaxTierRunes       = 64
	// A pace that overshoots the limit projects past 100%.
	aiMaxPercent      = 1e6
	aiMaxCostUsd      = 1e9
	aiSessionKeepDays = 90
	aiRankingCap      = 100
	aiSessionCap      = 50
)

var aiDeviceKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

// AiUsageReportDevice identifies the reporting computer; Key is random and stable.
type AiUsageReportDevice struct {
	Key        string `json:"key" example:"5b0d8a1f6c2e4f7a9d3b"`
	Name       string `json:"name" example:"laptop"`
	AppVersion string `json:"appVersion" example:"1.0.0"`
}

// AiUsageReportDay is one model's use in one project on one day; input tokens
// exclude cache reads and cost is the API-price equivalent in USD.
type AiUsageReportDay struct {
	Day              string  `json:"day" example:"2026-10-06"`
	App              string  `json:"app" validate:"oneof=claude codex" example:"claude"`
	Project          string  `json:"project" example:"/home/dev/app"`
	Model            string  `json:"model" example:"claude-opus-5-5"`
	Requests         int64   `json:"requests" example:"42"`
	InputTokens      int64   `json:"inputTokens" example:"1200"`
	OutputTokens     int64   `json:"outputTokens" example:"34000"`
	CacheReadTokens  int64   `json:"cacheReadTokens" example:"2400000"`
	CacheWriteTokens int64   `json:"cacheWriteTokens" example:"56000"`
	CostUsd          float64 `json:"costUsd" example:"3.51"`
}

// AiUsageReportSession is one session's totals; times are unix seconds.
type AiUsageReportSession struct {
	App             string  `json:"app" validate:"oneof=claude codex" example:"claude"`
	SessionId       string  `json:"sessionId" example:"0f6a2d4e-1b3c-4d5e-8f90-a1b2c3d4e5f6"`
	Title           string  `json:"title" example:"Fix the login flow"`
	Project         string  `json:"project" example:"/home/dev/app"`
	Model           string  `json:"model" example:"claude-opus-5-5"`
	Requests        int64   `json:"requests" example:"64"`
	Tokens          int64   `json:"tokens" example:"900000"`
	CacheReadTokens int64   `json:"cacheReadTokens" example:"12000000"`
	CostUsd         float64 `json:"costUsd" example:"3.25"`
	FirstAt         int64   `json:"firstAt" example:"1791200000"`
	LastAt          int64   `json:"lastAt" example:"1791203600"`
}

// AiUsageQuotaTier is one rate-limit window: percent used and when it resets.
type AiUsageQuotaTier struct {
	Name        string  `json:"name" example:"five_hour"`
	Utilization float64 `json:"utilization" example:"45"`
	ResetsAt    string  `json:"resetsAt" example:"2026-10-06T18:00:00Z"`
}

// AiUsageWindowUsage is what one computer used inside a plan window.
type AiUsageWindowUsage struct {
	Requests    int64   `json:"requests" example:"164"`
	CostUsd     float64 `json:"costUsd" example:"29.62"`
	TotalTokens int64   `json:"totalTokens" example:"91200000"`
}

// AiUsageLimitEstimate is a window's total limit, usage ÷ the reported share, with the range a
// whole-number percentage allows; Basis typical means the median of recent windows.
type AiUsageLimitEstimate struct {
	CostUsd  float64 `json:"costUsd" example:"185.15"`
	CostLow  float64 `json:"costLow" example:"179.54"`
	CostHigh float64 `json:"costHigh" example:"191.13"`
	Tokens   int64   `json:"tokens" example:"570000000"`
	Basis    string  `json:"basis" validate:"oneof=current typical" example:"current"`
	Windows  int     `json:"windows" example:"1"`
}

// AiUsageWindowEstimate is a current plan window as Pigger Switch measured it (times in
// Unix seconds); null fields are unknown, such as a limit before there is usage to divide.
type AiUsageWindowEstimate struct {
	Tier                 string                `json:"tier" example:"five_hour"`
	Start                *int64                `json:"start" example:"1791203400"`
	End                  *int64                `json:"end" example:"1791221400"`
	ReportedUtilization  *float64              `json:"reportedUtilization" example:"16"`
	ReportedAt           *int64                `json:"reportedAt" example:"1791207000"`
	EstimatedUtilization *float64              `json:"estimatedUtilization" example:"18"`
	Used                 AiUsageWindowUsage    `json:"used"`
	Limit                *AiUsageLimitEstimate `json:"limit"`
	RemainingCostUsd     *float64              `json:"remainingCostUsd" example:"155.53"`
	RemainingTokens      *int64                `json:"remainingTokens" example:"479000000"`
	ProjectedUtilization *float64              `json:"projectedUtilization" example:"67"`
	ExhaustsAt           *int64                `json:"exhaustsAt" example:"1791219000"`
	FiveHourWindowsLeft  *int                  `json:"fiveHourWindowsLeft" example:"27"`
	PerFiveHourCostUsd   *float64              `json:"perFiveHourCostUsd" example:"35.67"`
	PerFiveHourTokens    *int64                `json:"perFiveHourTokens" example:"121000000"`
}

// AiUsagePastWindow is an earlier or the current window; Exact is false when its start was
// inferred from the first request because no plan reading covers it.
type AiUsagePastWindow struct {
	Start           int64                 `json:"start" example:"1791203400"`
	End             int64                 `json:"end" example:"1791221400"`
	Exact           bool                  `json:"exact" example:"true"`
	Current         bool                  `json:"current" example:"false"`
	Used            AiUsageWindowUsage    `json:"used"`
	PeakUtilization *float64              `json:"peakUtilization" example:"45"`
	Limit           *AiUsageLimitEstimate `json:"limit"`
}

// AiUsageEstimates are a tool's plan windows measured against one computer's usage.
type AiUsageEstimates struct {
	Windows         []AiUsageWindowEstimate `json:"windows"`
	FiveHourHistory []AiUsagePastWindow     `json:"fiveHourHistory"`
	WeeklyHistory   []AiUsagePastWindow     `json:"weeklyHistory"`
}

// AiUsageReportQuota is a plan-limit reading; QueriedAt is unix milliseconds.
type AiUsageReportQuota struct {
	Tool        string             `json:"tool" validate:"oneof=claude codex" example:"claude"`
	Success     bool               `json:"success" example:"true"`
	PlanLabel   string             `json:"planLabel" example:"Max 5x"`
	ActiveUntil string             `json:"activeUntil" example:""`
	Tiers       []AiUsageQuotaTier `json:"tiers"`
	Error       string             `json:"error" example:""`
	QueriedAt   int64              `json:"queriedAt" example:"1791203600000"`
	Estimates   *AiUsageEstimates  `json:"estimates"`
}

// normalize checks a tool's window estimates and fills empty lists, so the page never sees null.
func (e *AiUsageEstimates) normalize(tool string) error {
	if len(e.Windows) > aiMaxEstimateWindows || len(e.FiveHourHistory) > aiMaxHistoryWindows ||
		len(e.WeeklyHistory) > aiMaxHistoryWindows {
		return common.NewErrorf("the %s estimates are too large", tool)
	}
	amount := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= aiMaxCostUsd }
	optAmount := func(v *float64) bool { return v == nil || amount(*v) }
	percent := func(v *float64) bool {
		return v == nil || (!math.IsNaN(*v) && !math.IsInf(*v, 0) && *v >= 0 && *v <= aiMaxPercent)
	}
	count := func(v *int64) bool { return v == nil || *v >= 0 }
	usage := func(u AiUsageWindowUsage) bool { return u.Requests >= 0 && u.TotalTokens >= 0 && amount(u.CostUsd) }
	invalid := common.NewErrorf("the %s estimates have an invalid number", tool)
	limit := func(l *AiUsageLimitEstimate) error {
		if l == nil {
			return nil
		}
		if l.Basis != "current" && l.Basis != "typical" {
			return common.NewErrorf("the %s estimates have an unknown basis %q", tool, l.Basis)
		}
		if !amount(l.CostUsd) || !amount(l.CostLow) || !amount(l.CostHigh) || l.Tokens < 0 || l.Windows < 0 {
			return invalid
		}
		return nil
	}
	for i := range e.Windows {
		w := &e.Windows[i]
		if w.Tier = truncateRunes(w.Tier, aiMaxTierRunes); w.Tier == "" {
			return common.NewErrorf("a %s window estimate has no tier", tool)
		}
		if !usage(w.Used) || !percent(w.ReportedUtilization) || !percent(w.EstimatedUtilization) ||
			!percent(w.ProjectedUtilization) || !optAmount(w.RemainingCostUsd) || !optAmount(w.PerFiveHourCostUsd) ||
			!count(w.Start) || !count(w.End) || !count(w.ReportedAt) || !count(w.ExhaustsAt) ||
			!count(w.RemainingTokens) || !count(w.PerFiveHourTokens) ||
			(w.FiveHourWindowsLeft != nil && *w.FiveHourWindowsLeft < 0) {
			return invalid
		}
		if err := limit(w.Limit); err != nil {
			return err
		}
	}
	for _, list := range [][]AiUsagePastWindow{e.FiveHourHistory, e.WeeklyHistory} {
		for _, w := range list {
			if w.Start < 0 || w.End < w.Start || !usage(w.Used) || !percent(w.PeakUtilization) {
				return invalid
			}
			if err := limit(w.Limit); err != nil {
				return err
			}
		}
	}
	if e.Windows == nil {
		e.Windows = []AiUsageWindowEstimate{}
	}
	if e.FiveHourHistory == nil {
		e.FiveHourHistory = []AiUsagePastWindow{}
	}
	if e.WeeklyHistory == nil {
		e.WeeklyHistory = []AiUsagePastWindow{}
	}
	return nil
}

// AiUsageReport is what Pigger Switch uploads: its usage on the whole days
// From..To of its time zone, which replaces what Pigger held for those days.
type AiUsageReport struct {
	Device   AiUsageReportDevice    `json:"device"`
	From     string                 `json:"from" example:"2026-10-05"`
	To       string                 `json:"to" example:"2026-10-06"`
	Daily    []AiUsageReportDay     `json:"daily"`
	Sessions []AiUsageReportSession `json:"sessions"`
	// SessionsSince (Unix seconds), when set, makes the report speak for every session of the
	// device active since then: the ones it leaves out are deleted. Zero only upserts.
	SessionsSince int64                `json:"sessionsSince" example:"0"`
	Quotas        []AiUsageReportQuota `json:"quotas"`
}

// AiUsageIngestResult names the device a report landed on and what it stored.
type AiUsageIngestResult struct {
	DeviceId int `json:"deviceId" example:"1"`
	Days     int `json:"days" example:"2"`
	Rows     int `json:"rows" example:"18"`
	Sessions int `json:"sessions" example:"6"`
}

func truncateRunes(s string, max int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > max {
		return string(r[:max])
	}
	return s
}

func usdToMicros(usd float64) int64 {
	return int64(math.Round(usd * 1e6))
}

func microsToUsd(micros int64) float64 {
	return float64(micros) / 1e6
}

func validAiApp(app string) bool {
	return app == "claude" || app == "codex"
}

func parseAiDay(s string) (int, error) {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return 0, common.NewErrorf("invalid date %q: use YYYY-MM-DD", s)
	}
	return dayNumber(t), nil
}

func checkAiCounts(what string, cost float64, counts ...int64) error {
	for _, n := range counts {
		if n < 0 {
			return common.NewErrorf("%s has a negative count", what)
		}
	}
	if math.IsNaN(cost) || math.IsInf(cost, 0) || cost < 0 || cost > aiMaxCostUsd {
		return common.NewErrorf("%s has an invalid cost", what)
	}
	return nil
}

type aiDailyKey struct {
	day                 int
	app, project, model string
}

// normalize validates the whole report before anything is written, so a bad
// report changes nothing, and folds rows that truncation made identical.
func (r *AiUsageReport) normalize() (from, to int, rows []model.AiUsageDaily, sessions []model.AiUsageSession, err error) {
	if !aiDeviceKeyPattern.MatchString(r.Device.Key) {
		return 0, 0, nil, nil, common.NewError("device key must be 8 to 64 letters, digits, '-' or '_'")
	}
	r.Device.Name = truncateRunes(r.Device.Name, aiDeviceNameMaxRunes)
	if r.Device.Name == "" {
		return 0, 0, nil, nil, common.NewError("device name is required")
	}
	r.Device.AppVersion = truncateRunes(r.Device.AppVersion, 32)
	if from, err = parseAiDay(r.From); err != nil {
		return
	}
	if to, err = parseAiDay(r.To); err != nil {
		return
	}
	if from > to {
		return 0, 0, nil, nil, common.NewErrorf("report from %s is after to %s", r.From, r.To)
	}
	if r.SessionsSince < 0 {
		return 0, 0, nil, nil, common.NewError("sessionsSince must be a Unix time, or 0")
	}
	if len(r.Daily) > aiMaxDailyRows || len(r.Sessions) > aiMaxSessions {
		return 0, 0, nil, nil, common.NewError("report is too large: split it into smaller windows")
	}

	merged := map[aiDailyKey]*model.AiUsageDaily{}
	order := []aiDailyKey{}
	for _, d := range r.Daily {
		day, perr := parseAiDay(d.Day)
		if perr != nil {
			return 0, 0, nil, nil, perr
		}
		if day < from || day > to {
			return 0, 0, nil, nil, common.NewErrorf("day %s is outside the report's window %s..%s", d.Day, r.From, r.To)
		}
		if !validAiApp(d.App) {
			return 0, 0, nil, nil, common.NewErrorf("unknown app %q: use claude or codex", d.App)
		}
		if cerr := checkAiCounts("day "+d.Day, d.CostUsd, d.Requests, d.InputTokens, d.OutputTokens, d.CacheReadTokens, d.CacheWriteTokens); cerr != nil {
			return 0, 0, nil, nil, cerr
		}
		key := aiDailyKey{day, d.App, truncateRunes(d.Project, aiProjectMaxRunes), truncateRunes(d.Model, aiModelMaxRunes)}
		row, seen := merged[key]
		if !seen {
			row = &model.AiUsageDaily{Day: day, App: key.app, Project: key.project, Model: key.model}
			merged[key] = row
			order = append(order, key)
		}
		row.Requests += d.Requests
		row.InputTokens += d.InputTokens
		row.OutputTokens += d.OutputTokens
		row.CacheReadTokens += d.CacheReadTokens
		row.CacheWriteTokens += d.CacheWriteTokens
		row.CostMicros += usdToMicros(d.CostUsd)
	}
	for _, k := range order {
		rows = append(rows, *merged[k])
	}

	seenSession := map[string]int{}
	for _, s := range r.Sessions {
		if !validAiApp(s.App) {
			return 0, 0, nil, nil, common.NewErrorf("unknown app %q: use claude or codex", s.App)
		}
		id := truncateRunes(s.SessionId, aiSessionIdMaxRunes)
		if id == "" {
			return 0, 0, nil, nil, common.NewError("a session has no id")
		}
		if cerr := checkAiCounts("session "+id, s.CostUsd, s.Requests, s.Tokens, s.CacheReadTokens, s.FirstAt, s.LastAt); cerr != nil {
			return 0, 0, nil, nil, cerr
		}
		row := model.AiUsageSession{
			App: s.App, SessionId: id, Title: truncateRunes(s.Title, aiTitleMaxRunes),
			Project: truncateRunes(s.Project, aiProjectMaxRunes), Model: truncateRunes(s.Model, aiModelMaxRunes),
			Requests: s.Requests, Tokens: s.Tokens, CacheReadTokens: s.CacheReadTokens,
			CostMicros: usdToMicros(s.CostUsd), FirstAt: s.FirstAt, LastAt: s.LastAt,
		}
		if i, dup := seenSession[s.App+"\x00"+id]; dup {
			sessions[i] = row
			continue
		}
		seenSession[s.App+"\x00"+id] = len(sessions)
		sessions = append(sessions, row)
	}

	for i := range r.Quotas {
		q := &r.Quotas[i]
		if !validAiApp(q.Tool) {
			return 0, 0, nil, nil, common.NewErrorf("unknown tool %q: use claude or codex", q.Tool)
		}
		if len(q.Tiers) > aiMaxQuotaTiers {
			return 0, 0, nil, nil, common.NewErrorf("the %s reading has too many windows", q.Tool)
		}
		if q.Estimates != nil {
			if err := q.Estimates.normalize(q.Tool); err != nil {
				return 0, 0, nil, nil, err
			}
		}
	}
	return from, to, rows, sessions, nil
}

// Ingest stores one report in a single transaction: the device row, its days
// From..To replaced wholesale, its sessions and its plan-limit readings.
func (s *AiUsageService) Ingest(report *AiUsageReport, now time.Time) (*AiUsageIngestResult, error) {
	from, to, rows, sessions, err := report.normalize()
	if err != nil {
		return nil, err
	}
	result := &AiUsageIngestResult{Rows: len(rows), Sessions: len(sessions)}
	err = runSerializedTx(func(tx *gorm.DB) error {
		device := model.AiUsageDevice{DeviceKey: report.Device.Key}
		if err := tx.Where("device_key = ?", device.DeviceKey).Limit(1).Find(&device).Error; err != nil {
			return err
		}
		device.Name = report.Device.Name
		device.AppVersion = report.Device.AppVersion
		device.LastSyncAt = now.Unix()
		if err := tx.Save(&device).Error; err != nil {
			return err
		}
		result.DeviceId = device.Id

		if err := tx.Where("device_id = ? AND day BETWEEN ? AND ?", device.Id, from, to).
			Delete(&model.AiUsageDaily{}).Error; err != nil {
			return err
		}
		days := map[int]struct{}{}
		for i := range rows {
			rows[i].DeviceId = device.Id
			days[rows[i].Day] = struct{}{}
		}
		result.Days = len(days)
		if len(rows) > 0 {
			if err := tx.CreateInBatches(rows, 500).Error; err != nil {
				return err
			}
		}

		if report.SessionsSince > 0 {
			if err := tx.Where("device_id = ? AND last_at >= ?", device.Id, report.SessionsSince).Delete(&model.AiUsageSession{}).Error; err != nil {
				return err
			}
		}
		for i := range sessions {
			sessions[i].DeviceId = device.Id
		}
		if len(sessions) > 0 {
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "device_id"}, {Name: "app"}, {Name: "session_id"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"title", "project", "model", "requests", "tokens",
					"cache_read_tokens", "cost_micros", "first_at", "last_at",
				}),
			}).CreateInBatches(sessions, 500).Error; err != nil {
				return err
			}
		}
		cutoff := now.AddDate(0, 0, -aiSessionKeepDays).Unix()
		if err := tx.Where("last_at < ?", cutoff).Delete(&model.AiUsageSession{}).Error; err != nil {
			return err
		}

		for _, q := range report.Quotas {
			if q.Tiers == nil {
				q.Tiers = []AiUsageQuotaTier{}
			}
			tiers, err := json.Marshal(q.Tiers)
			if err != nil {
				return err
			}
			// A reading without estimates (an older Pigger Switch) clears the old ones.
			estimates := ""
			if q.Estimates != nil {
				encoded, err := json.Marshal(q.Estimates)
				if err != nil {
					return err
				}
				estimates = string(encoded)
			}
			row := model.AiUsageQuota{
				DeviceId: device.Id, Tool: q.Tool, Success: q.Success,
				PlanLabel: truncateRunes(q.PlanLabel, 64), ActiveUntil: truncateRunes(q.ActiveUntil, 40),
				Tiers: string(tiers), Error: truncateRunes(q.Error, 500), QueriedAt: q.QueriedAt,
				Estimates: estimates,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "device_id"}, {Name: "tool"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"success", "plan_label", "active_until", "tiers", "error", "queried_at", "estimates",
				}),
			}).Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// DeleteDevice forgets a computer and everything it reported.
func (s *AiUsageService) DeleteDevice(id int) error {
	return runSerializedTx(func(tx *gorm.DB) error {
		for _, m := range []any{&model.AiUsageDaily{}, &model.AiUsageSession{}, &model.AiUsageQuota{}} {
			if err := tx.Where("device_id = ?", id).Delete(m).Error; err != nil {
				return err
			}
		}
		return tx.Delete(&model.AiUsageDevice{}, id).Error
	})
}

// AiUsagePeriod is the span the totals and rankings cover, like the traffic
// page's periods, plus all of the history.
type AiUsagePeriod string

const (
	AiUsageToday AiUsagePeriod = "today"
	AiUsageWeek  AiUsagePeriod = "week"
	AiUsageMonth AiUsagePeriod = "month"
	AiUsageAll   AiUsagePeriod = "all"
)

// ParseAiUsagePeriod reads the page's period switch; empty means the month.
func ParseAiUsagePeriod(s string) (AiUsagePeriod, error) {
	switch p := AiUsagePeriod(s); p {
	case "":
		return AiUsageMonth, nil
	case AiUsageToday, AiUsageWeek, AiUsageMonth, AiUsageAll:
		return p, nil
	}
	return "", common.NewErrorf("unknown period %q: use today, week, month or all", s)
}

// AiUsageTotals adds up the period; tokens are split the way the models bill them.
type AiUsageTotals struct {
	CostUsd          float64 `json:"costUsd" example:"128.4"`
	ClaudeCostUsd    float64 `json:"claudeCostUsd" example:"120.1"`
	CodexCostUsd     float64 `json:"codexCostUsd" example:"8.3"`
	Requests         int64   `json:"requests" example:"5120"`
	InputTokens      int64   `json:"inputTokens" example:"420000"`
	OutputTokens     int64   `json:"outputTokens" example:"3100000"`
	CacheReadTokens  int64   `json:"cacheReadTokens" example:"410000000"`
	CacheWriteTokens int64   `json:"cacheWriteTokens" example:"9800000"`
	Sessions         int64   `json:"sessions" example:"23"`
}

// AiUsageDay is one day of the 30-day chart, split by app.
type AiUsageDay struct {
	Day           string  `json:"day" example:"2026-10-06"`
	ClaudeCostUsd float64 `json:"claudeCostUsd" example:"12.5"`
	CodexCostUsd  float64 `json:"codexCostUsd" example:"1.2"`
	ClaudeTokens  int64   `json:"claudeTokens" example:"45000000"`
	CodexTokens   int64   `json:"codexTokens" example:"3000000"`
}

// AiUsageRank is a project or a model over the period; Tokens leaves out cache reads.
type AiUsageRank struct {
	Name          string  `json:"name" example:"/home/dev/app"`
	App           string  `json:"app" example:"claude"`
	CostUsd       float64 `json:"costUsd" example:"42.5"`
	ClaudeCostUsd float64 `json:"claudeCostUsd" example:"40"`
	CodexCostUsd  float64 `json:"codexCostUsd" example:"2.5"`
	Requests      int64   `json:"requests" example:"812"`
	Tokens        int64   `json:"tokens" example:"1200000"`
}

// AiUsageSessionView is a session that was active in the period.
type AiUsageSessionView struct {
	App        string  `json:"app" example:"claude"`
	SessionId  string  `json:"sessionId" example:"0f6a2d4e-1b3c-4d5e-8f90-a1b2c3d4e5f6"`
	Title      string  `json:"title" example:"Fix the login flow"`
	Project    string  `json:"project" example:"/home/dev/app"`
	Model      string  `json:"model" example:"claude-opus-5-5"`
	DeviceName string  `json:"deviceName" example:"laptop"`
	Requests   int64   `json:"requests" example:"64"`
	Tokens     int64   `json:"tokens" example:"900000"`
	CostUsd    float64 `json:"costUsd" example:"3.25"`
	FirstAt    int64   `json:"firstAt" example:"1791200000"`
	LastAt     int64   `json:"lastAt" example:"1791203600"`
}

// AiUsageQuotaView is the newest plan-limit reading of one tool.
type AiUsageQuotaView struct {
	Tool        string             `json:"tool" example:"claude"`
	DeviceName  string             `json:"deviceName" example:"laptop"`
	Success     bool               `json:"success" example:"true"`
	PlanLabel   string             `json:"planLabel" example:"Max 5x"`
	ActiveUntil string             `json:"activeUntil" example:""`
	Tiers       []AiUsageQuotaTier `json:"tiers"`
	Error       string             `json:"error" example:""`
	QueriedAt   int64              `json:"queriedAt" example:"1791203600000"`
	// Estimates are the reading device's window estimates; null from an older Pigger Switch.
	Estimates *AiUsageEstimates `json:"estimates"`
}

// AiUsageDeviceView is a reporting computer with its all-time spend.
type AiUsageDeviceView struct {
	Id         int     `json:"id" example:"1"`
	Name       string  `json:"name" example:"laptop"`
	AppVersion string  `json:"appVersion" example:"1.0.0"`
	LastSyncAt int64   `json:"lastSyncAt" example:"1791203600"`
	FirstDay   string  `json:"firstDay" example:"2026-08-01"`
	LastDay    string  `json:"lastDay" example:"2026-10-06"`
	CostUsd    float64 `json:"costUsd" example:"912.3"`
}

// AiUsageOverview is the AI usage page: the period's totals and rankings, the
// last 30 days, the sessions of the period, the plan limits and the devices.
type AiUsageOverview struct {
	Period      string               `json:"period" validate:"oneof=today week month all" example:"month"`
	PeriodStart string               `json:"periodStart" example:"2026-10-01"`
	Totals      AiUsageTotals        `json:"totals"`
	Daily       []AiUsageDay         `json:"daily"`
	Projects    []AiUsageRank        `json:"projects"`
	Models      []AiUsageRank        `json:"models"`
	Sessions    []AiUsageSessionView `json:"sessions"`
	Quotas      []AiUsageQuotaView   `json:"quotas"`
	Devices     []AiUsageDeviceView  `json:"devices"`
}

func dayString(day int) string {
	if day <= 0 {
		return ""
	}
	return time.Date(day/10000, time.Month(day/100%100), day%100, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
}

type aiFilter struct {
	deviceId int
	app      string
}

func (f aiFilter) apply(q *gorm.DB) *gorm.DB {
	if f.deviceId > 0 {
		q = q.Where("device_id = ?", f.deviceId)
	}
	if f.app != "" {
		q = q.Where("app = ?", f.app)
	}
	return q
}

type aiSum struct {
	Name, App                                                           string
	Day                                                                 int
	Requests, Input, Output, CacheRead, CacheWrite, Cost, Claude, Codex int64
}

const aiSumColumns = `COALESCE(SUM(requests), 0) AS requests,
	COALESCE(SUM(input_tokens), 0) AS input, COALESCE(SUM(output_tokens), 0) AS output,
	COALESCE(SUM(cache_read_tokens), 0) AS cache_read, COALESCE(SUM(cache_write_tokens), 0) AS cache_write,
	COALESCE(SUM(cost_micros), 0) AS cost,
	COALESCE(SUM(CASE WHEN app = 'claude' THEN cost_micros ELSE 0 END), 0) AS claude,
	COALESCE(SUM(CASE WHEN app = 'codex' THEN cost_micros ELSE 0 END), 0) AS codex`

// Overview builds the page for the period ending at now (the panel's time
// zone). deviceId 0 is every device; app "" is both apps.
func (s *AiUsageService) Overview(now time.Time, period AiUsagePeriod, deviceId int, app string) (*AiUsageOverview, error) {
	if app != "" && !validAiApp(app) {
		return nil, common.NewErrorf("unknown app %q: use claude or codex", app)
	}
	filter := aiFilter{deviceId: deviceId, app: app}
	db := database.GetDB()
	ov := &AiUsageOverview{Period: string(period)}

	startDay, startUnix := 0, int64(0)
	if period != AiUsageAll {
		start := TrafficPeriod(period).Start(now)
		startDay, startUnix = dayNumber(start), start.Unix()
		ov.PeriodStart = start.Format(time.DateOnly)
	}
	inPeriod := func() *gorm.DB {
		return filter.apply(db.Model(&model.AiUsageDaily{})).Where("day >= ?", startDay)
	}

	var total aiSum
	if err := inPeriod().Select(aiSumColumns).Scan(&total).Error; err != nil {
		return nil, err
	}
	ov.Totals = AiUsageTotals{
		CostUsd: microsToUsd(total.Cost), ClaudeCostUsd: microsToUsd(total.Claude), CodexCostUsd: microsToUsd(total.Codex),
		Requests: total.Requests, InputTokens: total.Input, OutputTokens: total.Output,
		CacheReadTokens: total.CacheRead, CacheWriteTokens: total.CacheWrite,
	}

	first := now.AddDate(0, 0, 1-aiDailyDays)
	var perDay []aiSum
	if err := filter.apply(db.Model(&model.AiUsageDaily{})).
		Where("day >= ?", dayNumber(first)).
		Select("day, app, " + aiSumColumns).
		Group("day, app").Scan(&perDay).Error; err != nil {
		return nil, err
	}
	ov.Daily = make([]AiUsageDay, aiDailyDays)
	byDay := map[int]*AiUsageDay{}
	for i := range ov.Daily {
		d := first.AddDate(0, 0, i)
		ov.Daily[i].Day = d.Format(time.DateOnly)
		byDay[dayNumber(d)] = &ov.Daily[i]
	}
	for _, r := range perDay {
		d, ok := byDay[r.Day]
		if !ok {
			continue
		}
		tokens := r.Input + r.Output + r.CacheRead + r.CacheWrite
		if r.App == "claude" {
			d.ClaudeCostUsd, d.ClaudeTokens = microsToUsd(r.Cost), tokens
		} else {
			d.CodexCostUsd, d.CodexTokens = microsToUsd(r.Cost), tokens
		}
	}

	rank := func(group string) ([]AiUsageRank, error) {
		var sums []aiSum
		if err := inPeriod().Select(group + " AS name, MIN(app) AS app, " + aiSumColumns).
			Group(group).Scan(&sums).Error; err != nil {
			return nil, err
		}
		slices.SortFunc(sums, func(a, b aiSum) int {
			return cmp.Or(cmp.Compare(b.Cost, a.Cost), cmp.Compare(b.Requests, a.Requests), strings.Compare(a.Name, b.Name))
		})
		out := make([]AiUsageRank, 0, min(len(sums), aiRankingCap))
		for _, r := range sums[:min(len(sums), aiRankingCap)] {
			out = append(out, AiUsageRank{
				Name: r.Name, App: r.App, CostUsd: microsToUsd(r.Cost),
				ClaudeCostUsd: microsToUsd(r.Claude), CodexCostUsd: microsToUsd(r.Codex),
				Requests: r.Requests, Tokens: r.Input + r.Output,
			})
		}
		return out, nil
	}
	var err error
	if ov.Projects, err = rank("project"); err != nil {
		return nil, err
	}
	if ov.Models, err = rank("model"); err != nil {
		return nil, err
	}

	var devices []model.AiUsageDevice
	if err := db.Order("id asc").Find(&devices).Error; err != nil {
		return nil, err
	}
	names := make(map[int]string, len(devices))
	for _, d := range devices {
		names[d.Id] = d.Name
	}

	sessionsQ := filter.apply(db.Model(&model.AiUsageSession{})).Where("last_at >= ?", startUnix)
	if err := sessionsQ.Count(&ov.Totals.Sessions).Error; err != nil {
		return nil, err
	}
	var sessions []model.AiUsageSession
	if err := filter.apply(db.Model(&model.AiUsageSession{})).Where("last_at >= ?", startUnix).
		Order("cost_micros desc, last_at desc").Limit(aiSessionCap).Find(&sessions).Error; err != nil {
		return nil, err
	}
	ov.Sessions = make([]AiUsageSessionView, 0, len(sessions))
	for _, r := range sessions {
		ov.Sessions = append(ov.Sessions, AiUsageSessionView{
			App: r.App, SessionId: r.SessionId, Title: r.Title, Project: r.Project, Model: r.Model,
			DeviceName: names[r.DeviceId], Requests: r.Requests, Tokens: r.Tokens,
			CostUsd: microsToUsd(r.CostMicros), FirstAt: r.FirstAt, LastAt: r.LastAt,
		})
	}

	if ov.Quotas, err = s.latestQuotas(db, deviceId, names); err != nil {
		return nil, err
	}

	var spans []struct {
		DeviceId, FirstDay, LastDay int
		Cost                        int64
	}
	// Each tool's page counts a computer's spend on that tool only.
	spansQ := db.Model(&model.AiUsageDaily{})
	if app != "" {
		spansQ = spansQ.Where("app = ?", app)
	}
	if err := spansQ.
		Select("device_id, MIN(day) AS first_day, MAX(day) AS last_day, COALESCE(SUM(cost_micros), 0) AS cost").
		Group("device_id").Scan(&spans).Error; err != nil {
		return nil, err
	}
	spanOf := map[int]int{}
	for i, sp := range spans {
		spanOf[sp.DeviceId] = i
	}
	ov.Devices = make([]AiUsageDeviceView, 0, len(devices))
	for _, d := range devices {
		view := AiUsageDeviceView{Id: d.Id, Name: d.Name, AppVersion: d.AppVersion, LastSyncAt: d.LastSyncAt}
		if i, ok := spanOf[d.Id]; ok {
			view.FirstDay, view.LastDay = dayString(spans[i].FirstDay), dayString(spans[i].LastDay)
			view.CostUsd = microsToUsd(spans[i].Cost)
		}
		ov.Devices = append(ov.Devices, view)
	}
	return ov, nil
}

// latestQuotas keeps, per tool, the reading taken most recently on any device.
func (s *AiUsageService) latestQuotas(db *gorm.DB, deviceId int, names map[int]string) ([]AiUsageQuotaView, error) {
	q := db.Model(&model.AiUsageQuota{})
	if deviceId > 0 {
		q = q.Where("device_id = ?", deviceId)
	}
	var rows []model.AiUsageQuota
	if err := q.Order("queried_at desc").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := []AiUsageQuotaView{}
	seen := map[string]bool{}
	for _, r := range rows {
		if seen[r.Tool] {
			continue
		}
		seen[r.Tool] = true
		tiers := []AiUsageQuotaTier{}
		if err := json.Unmarshal([]byte(r.Tiers), &tiers); err != nil || tiers == nil {
			tiers = []AiUsageQuotaTier{}
		}
		var estimates *AiUsageEstimates
		if r.Estimates != "" {
			var e AiUsageEstimates
			if err := json.Unmarshal([]byte(r.Estimates), &e); err == nil {
				estimates = &e
			}
		}
		out = append(out, AiUsageQuotaView{
			Tool: r.Tool, DeviceName: names[r.DeviceId], Success: r.Success, PlanLabel: r.PlanLabel,
			ActiveUntil: r.ActiveUntil, Tiers: tiers, Error: r.Error, QueriedAt: r.QueriedAt,
			Estimates: estimates,
		})
	}
	slices.SortFunc(out, func(a, b AiUsageQuotaView) int { return strings.Compare(a.Tool, b.Tool) })
	return out, nil
}
