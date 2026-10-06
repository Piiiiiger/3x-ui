package service

import (
	"math"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

var aiNow = time.Date(2026, 10, 6, 15, 0, 0, 0, time.UTC)

// setupAiUsageDB empties the AI usage tables too: on PostgreSQL every test of the
// package shares one schema, so another test's devices would still be there.
func setupAiUsageDB(t *testing.T) {
	t.Helper()
	setupConflictDB(t)
	for _, m := range []any{&model.AiUsageDaily{}, &model.AiUsageSession{}, &model.AiUsageQuota{}, &model.AiUsageDevice{}} {
		if err := database.GetDB().Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(m).Error; err != nil {
			t.Fatalf("empty %T: %v", m, err)
		}
	}
}

func aiDevice(key, name string) AiUsageReportDevice {
	return AiUsageReportDevice{Key: key, Name: name, AppVersion: "1.0.0"}
}

func aiDay(day, app, project, model string, requests int64, cost float64) AiUsageReportDay {
	return AiUsageReportDay{
		Day: day, App: app, Project: project, Model: model, Requests: requests,
		InputTokens: requests * 10, OutputTokens: requests * 100, CacheReadTokens: requests * 1000,
		CacheWriteTokens: requests, CostUsd: cost,
	}
}

func mustIngest(t *testing.T, report AiUsageReport) int {
	t.Helper()
	res, err := (&AiUsageService{}).Ingest(&report, aiNow)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	return res.DeviceId
}

type aiRowKey struct {
	day            int
	project, model string
}

func aiDailyRows(t *testing.T, deviceId int) map[aiRowKey]model.AiUsageDaily {
	t.Helper()
	var rows []model.AiUsageDaily
	if err := database.GetDB().Where("device_id = ?", deviceId).Find(&rows).Error; err != nil {
		t.Fatalf("read dailies: %v", err)
	}
	out := map[aiRowKey]model.AiUsageDaily{}
	for _, r := range rows {
		out[aiRowKey{r.Day, r.Project, r.Model}] = r
	}
	return out
}

// A report covers whole days: what it says about them replaces the old rows,
// and days outside its window stay as they were.
func TestAiUsageIngestReplacesOnlyTheDaysItCovers(t *testing.T) {
	setupAiUsageDB(t)
	dev := aiDevice("device-key-1", "laptop")
	id := mustIngest(t, AiUsageReport{Device: dev, From: "2026-10-01", To: "2026-10-02", Daily: []AiUsageReportDay{
		aiDay("2026-10-01", "claude", "/w/a", "opus", 3, 1.5),
		aiDay("2026-10-02", "claude", "/w/a", "opus", 4, 2),
		aiDay("2026-10-02", "codex", "/w/b", "gpt", 5, 0.5),
	}})
	mustIngest(t, AiUsageReport{Device: dev, From: "2026-10-02", To: "2026-10-03", Daily: []AiUsageReportDay{
		aiDay("2026-10-02", "claude", "/w/a", "opus", 6, 3),
		aiDay("2026-10-03", "claude", "/w/a", "opus", 1, 0.25),
	}})

	rows := aiDailyRows(t, id)
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3: %+v", len(rows), rows)
	}
	if r := rows[aiRowKey{20261001, "/w/a", "opus"}]; r.Requests != 3 || r.CostMicros != 1_500_000 {
		t.Fatalf("2026-10-01 outside the second window changed: %+v", r)
	}
	if r := rows[aiRowKey{20261002, "/w/a", "opus"}]; r.Requests != 6 || r.CostMicros != 3_000_000 {
		t.Fatalf("2026-10-02 = %+v, want the second report's 6 requests / $3", r)
	}
	if _, ok := rows[aiRowKey{20261002, "/w/b", "gpt"}]; ok {
		t.Fatal("a row the newer report no longer has must be gone from its days")
	}
}

// Pigger Switch resends the same days every few minutes; that must not add up.
func TestAiUsageIngestTwiceCountsOnce(t *testing.T) {
	setupAiUsageDB(t)
	report := AiUsageReport{
		Device: aiDevice("device-key-1", "laptop"), From: "2026-10-06", To: "2026-10-06",
		Daily: []AiUsageReportDay{aiDay("2026-10-06", "claude", "/w/a", "opus", 2, 1)},
	}
	id := mustIngest(t, report)
	mustIngest(t, report)
	if r := aiDailyRows(t, id)[aiRowKey{20261006, "/w/a", "opus"}]; r.Requests != 2 || r.CostMicros != 1_000_000 {
		t.Fatalf("after two identical reports: %+v, want 2 requests / $1", r)
	}
}

// estimatesQuota is a claude reading whose estimates are valid until mutate breaks them.
func estimatesQuota(mutate func(e *AiUsageEstimates)) AiUsageReportQuota {
	start, end, util := int64(1_000), int64(19_000), 16.0
	e := &AiUsageEstimates{Windows: []AiUsageWindowEstimate{{
		Tier: "five_hour", Start: &start, End: &end, ReportedUtilization: &util,
		Used:  AiUsageWindowUsage{Requests: 3, CostUsd: 1.5, TotalTokens: 300},
		Limit: &AiUsageLimitEstimate{CostUsd: 9.4, CostLow: 9.1, CostHigh: 9.7, Tokens: 1900, Basis: "current", Windows: 1},
	}}}
	mutate(e)
	return AiUsageReportQuota{Tool: "claude", Success: true, QueriedAt: 1, Estimates: e}
}

func TestAiUsageIngestRejectsBadReportsAndWritesNothing(t *testing.T) {
	good := func() AiUsageReport {
		return AiUsageReport{
			Device: aiDevice("device-key-1", "laptop"), From: "2026-10-05", To: "2026-10-06",
			Daily: []AiUsageReportDay{aiDay("2026-10-06", "claude", "/w/a", "opus", 1, 1)},
		}
	}
	cases := []struct {
		name   string
		mutate func(r *AiUsageReport)
		want   string
	}{
		{"day outside the window", func(r *AiUsageReport) { r.Daily[0].Day = "2026-10-07" }, "outside"},
		{"window backwards", func(r *AiUsageReport) { r.From, r.To = "2026-10-06", "2026-10-05" }, "from"},
		{"bad date", func(r *AiUsageReport) { r.To = "06/10/2026" }, "date"},
		{"unknown app", func(r *AiUsageReport) { r.Daily[0].App = "gemini" }, "app"},
		{"negative tokens", func(r *AiUsageReport) { r.Daily[0].OutputTokens = -1 }, "negative"},
		{"cost not a number", func(r *AiUsageReport) { r.Daily[0].CostUsd = math.NaN() }, "cost"},
		{"short device key", func(r *AiUsageReport) { r.Device.Key = "abc" }, "device key"},
		{"no device name", func(r *AiUsageReport) { r.Device.Name = "  " }, "device name"},
		{"estimate cost not a number", func(r *AiUsageReport) {
			r.Quotas = []AiUsageReportQuota{estimatesQuota(func(e *AiUsageEstimates) { e.Windows[0].Used.CostUsd = math.NaN() })}
		}, "estimate"},
		{"negative remaining", func(r *AiUsageReport) {
			r.Quotas = []AiUsageReportQuota{estimatesQuota(func(e *AiUsageEstimates) { left := -1.0; e.Windows[0].RemainingCostUsd = &left })}
		}, "estimate"},
		{"unknown estimate basis", func(r *AiUsageReport) {
			r.Quotas = []AiUsageReportQuota{estimatesQuota(func(e *AiUsageEstimates) { e.Windows[0].Limit.Basis = "guess" })}
		}, "basis"},
		{"window without a tier", func(r *AiUsageReport) {
			r.Quotas = []AiUsageReportQuota{estimatesQuota(func(e *AiUsageEstimates) { e.Windows[0].Tier = " " })}
		}, "tier"},
		{"too many windows", func(r *AiUsageReport) {
			r.Quotas = []AiUsageReportQuota{estimatesQuota(func(e *AiUsageEstimates) {
				for len(e.Windows) <= aiMaxEstimateWindows {
					e.Windows = append(e.Windows, e.Windows[0])
				}
			})}
		}, "estimates"},
		{"negative session window", func(r *AiUsageReport) { r.SessionsSince = -1 }, "sessionsSince"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupAiUsageDB(t)
			r := good()
			tc.mutate(&r)
			_, err := (&AiUsageService{}).Ingest(&r, aiNow)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one mentioning %q", err, tc.want)
			}
			var n int64
			database.GetDB().Model(&model.AiUsageDevice{}).Count(&n)
			if n != 0 {
				t.Fatalf("a rejected report left %d devices behind", n)
			}
		})
	}
}

// The key, not the name, is the device: renaming the computer keeps its history.
func TestAiUsageIngestKeepsADeviceAcrossRenames(t *testing.T) {
	setupAiUsageDB(t)
	first := mustIngest(t, AiUsageReport{Device: aiDevice("device-key-1", "laptop"), From: "2026-10-06", To: "2026-10-06"})
	renamed := mustIngest(t, AiUsageReport{Device: aiDevice("device-key-1", "work laptop"), From: "2026-10-06", To: "2026-10-06"})
	other := mustIngest(t, AiUsageReport{Device: aiDevice("device-key-2", "laptop"), From: "2026-10-06", To: "2026-10-06"})
	if renamed != first || other == first {
		t.Fatalf("device ids %d, %d, %d: want the first two equal and the third new", first, renamed, other)
	}
	var dev model.AiUsageDevice
	database.GetDB().First(&dev, first)
	if dev.Name != "work laptop" || dev.LastSyncAt != aiNow.Unix() {
		t.Fatalf("device = %+v, want the new name and this sync's time", dev)
	}
}

// Long paths are cut to the column's size; two that become equal share one row.
func TestAiUsageIngestMergesProjectsThatMeetAfterTruncation(t *testing.T) {
	setupAiUsageDB(t)
	long := "/" + strings.Repeat("p", 500)
	id := mustIngest(t, AiUsageReport{
		Device: aiDevice("device-key-1", "laptop"), From: "2026-10-06", To: "2026-10-06",
		Daily: []AiUsageReportDay{
			aiDay("2026-10-06", "claude", long+"/one", "opus", 1, 1),
			aiDay("2026-10-06", "claude", long+"/two", "opus", 2, 2),
		},
	})
	rows := aiDailyRows(t, id)
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want the two truncated projects merged into 1", len(rows))
	}
	for k, r := range rows {
		if len([]rune(k.project)) != aiProjectMaxRunes || r.Requests != 3 || r.CostMicros != 3_000_000 {
			t.Fatalf("merged row %q (%d runes) = %+v", k.project[:10], len([]rune(k.project)), r)
		}
	}
}

func aiSessionIds(t *testing.T, deviceId int) map[string]model.AiUsageSession {
	t.Helper()
	var rows []model.AiUsageSession
	database.GetDB().Where("device_id = ?", deviceId).Find(&rows)
	out := map[string]model.AiUsageSession{}
	for _, r := range rows {
		out[r.SessionId] = r
	}
	return out
}

// A report with sessionsSince speaks for the device's sessions active since then:
// the ones it leaves out go, while older ones and other devices' stay.
func TestAiUsageIngestSessionsSinceReplacesOnlyItsWindow(t *testing.T) {
	setupAiUsageDB(t)
	dev := aiDevice("device-key-1", "laptop")
	at := aiNow.Unix()
	sess := func(id string, cost float64, lastAt int64) AiUsageReportSession {
		return AiUsageReportSession{App: "claude", SessionId: id, Title: "t-" + id, Requests: 1, CostUsd: cost, FirstAt: lastAt - 60, LastAt: lastAt}
	}
	other := mustIngest(t, AiUsageReport{
		Device: aiDevice("device-key-2", "desktop"), From: "2026-10-06", To: "2026-10-06",
		Sessions: []AiUsageReportSession{sess("d", 1, at)},
	})
	id := mustIngest(t, AiUsageReport{
		Device: dev, From: "2026-10-06", To: "2026-10-06",
		Sessions: []AiUsageReportSession{sess("old", 9, at-40*86400), sess("a", 1, at), sess("b", 2, at)},
	})
	mustIngest(t, AiUsageReport{
		Device: dev, From: "2026-10-06", To: "2026-10-06",
		Sessions: []AiUsageReportSession{sess("b", 5, at), sess("c", 1, at)},
	})
	got := aiSessionIds(t, id)
	if len(got) != 4 || got["b"].CostMicros != 5_000_000 {
		t.Fatalf("after an upsert: %+v, want old, a, b ($5) and c", got)
	}
	mustIngest(t, AiUsageReport{
		Device: dev, From: "2026-10-06", To: "2026-10-06", SessionsSince: at - 30*86400,
		Sessions: []AiUsageReportSession{sess("c", 1, at)},
	})
	got = aiSessionIds(t, id)
	if len(got) != 2 || got["c"].SessionId != "c" || got["old"].SessionId != "old" {
		t.Fatalf("after a windowed replace: %+v, want old and c", got)
	}
	if len(aiSessionIds(t, other)) != 1 {
		t.Fatal("a report from one device deleted another device's session")
	}
}

// Estimates travel with a tool's plan reading: the newest reading's are shown, and a
// reading from an older Pigger Switch without them clears them instead of leaving stale ones.
func TestAiUsageIngestKeepsEachToolsWindowEstimates(t *testing.T) {
	setupAiUsageDB(t)
	dev := aiDevice("device-key-1", "laptop")
	start, end, util, left := aiNow.Unix()-3600, aiNow.Unix()+4*3600, 16.0, 155.5
	estimates := &AiUsageEstimates{
		Windows: []AiUsageWindowEstimate{{
			Tier: "five_hour", Start: &start, End: &end, ReportedUtilization: &util,
			Used:             AiUsageWindowUsage{Requests: 164, CostUsd: 29.62, TotalTokens: 91_200_000},
			Limit:            &AiUsageLimitEstimate{CostUsd: 185.15, CostLow: 179.54, CostHigh: 191.13, Tokens: 570_000_000, Basis: "current", Windows: 1},
			RemainingCostUsd: &left,
		}},
		FiveHourHistory: []AiUsagePastWindow{{
			Start: start, End: end, Exact: true, Current: true,
			Used: AiUsageWindowUsage{Requests: 164, CostUsd: 29.62}, PeakUtilization: &util,
		}},
	}
	reading := func(at int64, e *AiUsageEstimates) AiUsageReport {
		return AiUsageReport{Device: dev, From: "2026-10-06", To: "2026-10-06", Quotas: []AiUsageReportQuota{{
			Tool: "claude", Success: true, QueriedAt: at, Estimates: e,
			Tiers: []AiUsageQuotaTier{{Name: "five_hour", Utilization: 16}},
		}}}
	}
	mustIngest(t, reading(1, estimates))
	ov, err := (&AiUsageService{}).Overview(aiNow, AiUsageMonth, 0, "claude")
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	got := ov.Quotas[0].Estimates
	if got == nil || len(got.Windows) != 1 || got.Windows[0].Limit.CostUsd != 185.15 ||
		*got.Windows[0].RemainingCostUsd != 155.5 || len(got.FiveHourHistory) != 1 || got.WeeklyHistory == nil {
		t.Fatalf("estimates = %+v", got)
	}

	mustIngest(t, reading(2, nil))
	if ov, err = (&AiUsageService{}).Overview(aiNow, AiUsageMonth, 0, "claude"); err != nil {
		t.Fatalf("overview: %v", err)
	}
	if ov.Quotas[0].Estimates != nil {
		t.Fatalf("a reading without estimates left %+v behind", ov.Quotas[0].Estimates)
	}
}

// Each tool's page counts a computer's spend on that tool only.
func TestAiUsageDevicesCountOnlyTheChosenApp(t *testing.T) {
	setupAiUsageDB(t)
	laptop, desktop := seedAiOverview(t)
	cost := func(app string) map[int]float64 {
		ov, err := (&AiUsageService{}).Overview(aiNow, AiUsageMonth, 0, app)
		if err != nil {
			t.Fatalf("overview: %v", err)
		}
		out := map[int]float64{}
		for _, d := range ov.Devices {
			out[d.Id] = d.CostUsd
		}
		return out
	}
	if c := cost("codex"); c[laptop] != 2 || c[desktop] != 0 {
		t.Fatalf("codex spend per computer = %v, want laptop $2, desktop $0", c)
	}
	if c := cost("claude"); c[laptop] != 57 || c[desktop] != 4 {
		t.Fatalf("claude spend per computer = %v, want laptop $57, desktop $4", c)
	}
	if c := cost(""); c[laptop] != 59 {
		t.Fatalf("all spend = %v, want laptop $59", c)
	}
}

func TestAiUsageIngestKeepsTheLatestPlanReadingPerTool(t *testing.T) {
	setupAiUsageDB(t)
	dev := aiDevice("device-key-1", "laptop")
	quota := func(used float64, at int64) AiUsageReportQuota {
		return AiUsageReportQuota{
			Tool: "claude", Success: true, PlanLabel: "Max 5x", QueriedAt: at,
			Tiers: []AiUsageQuotaTier{{Name: "five_hour", Utilization: used, ResetsAt: "2026-10-06T18:00:00Z"}},
		}
	}
	id := mustIngest(t, AiUsageReport{Device: dev, From: "2026-10-06", To: "2026-10-06", Quotas: []AiUsageReportQuota{quota(10, 1)}})
	mustIngest(t, AiUsageReport{Device: dev, From: "2026-10-06", To: "2026-10-06", Quotas: []AiUsageReportQuota{quota(45, 2)}})
	var rows []model.AiUsageQuota
	database.GetDB().Where("device_id = ?", id).Find(&rows)
	if len(rows) != 1 || rows[0].QueriedAt != 2 || !strings.Contains(rows[0].Tiers, `"utilization":45`) {
		t.Fatalf("quotas = %+v, want one claude row with the 45%% reading", rows)
	}
}

func seedAiOverview(t *testing.T) (laptop, desktop int) {
	t.Helper()
	at := aiNow.Unix()
	laptop = mustIngest(t, AiUsageReport{
		Device: aiDevice("device-key-1", "laptop"), From: "2026-09-20", To: "2026-10-06",
		Daily: []AiUsageReportDay{
			aiDay("2026-09-20", "claude", "/w/old", "opus", 50, 50),
			aiDay("2026-10-02", "claude", "/w/app", "opus", 10, 6),
			aiDay("2026-10-06", "claude", "/w/app", "sonnet", 4, 1),
			aiDay("2026-10-06", "codex", "/w/lib", "gpt-5.5", 8, 2),
		},
		Sessions: []AiUsageReportSession{
			{App: "claude", SessionId: "s-old", Title: "september", CostUsd: 50, FirstAt: at - 20*86400, LastAt: at - 16*86400},
			{App: "claude", SessionId: "s-app", Title: "fix login", Project: "/w/app", CostUsd: 7, Requests: 14, FirstAt: at - 4*86400, LastAt: at - 60},
			{App: "codex", SessionId: "s-lib", Project: "/w/lib", CostUsd: 2, Requests: 8, FirstAt: at - 3600, LastAt: at - 30},
		},
		Quotas: []AiUsageReportQuota{{
			Tool: "claude", Success: true, PlanLabel: "Pro", QueriedAt: 1000,
			Tiers: []AiUsageQuotaTier{{Name: "five_hour", Utilization: 45}},
		}},
	})
	desktop = mustIngest(t, AiUsageReport{
		Device: aiDevice("device-key-2", "desktop"), From: "2026-10-06", To: "2026-10-06",
		Daily: []AiUsageReportDay{aiDay("2026-10-06", "claude", "/w/app", "opus", 2, 4)},
		Quotas: []AiUsageReportQuota{{
			Tool: "claude", Success: true, PlanLabel: "Max 5x", QueriedAt: 2000,
			Tiers: []AiUsageQuotaTier{{Name: "five_hour", Utilization: 12}},
		}},
	})
	return laptop, desktop
}

func TestAiUsageOverviewSumsTheMonthPerApp(t *testing.T) {
	setupAiUsageDB(t)
	seedAiOverview(t)
	ov, err := (&AiUsageService{}).Overview(aiNow, AiUsageMonth, 0, "")
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if ov.PeriodStart != "2026-10-01" {
		t.Fatalf("period start = %s, want 2026-10-01", ov.PeriodStart)
	}
	// September's $50 lies before the month.
	if ov.Totals.CostUsd != 13 || ov.Totals.ClaudeCostUsd != 11 || ov.Totals.CodexCostUsd != 2 || ov.Totals.Requests != 24 {
		t.Fatalf("totals = %+v, want $13 = claude $11 + codex $2 over 24 requests", ov.Totals)
	}
	if ov.Totals.CacheReadTokens != 24_000 || ov.Totals.InputTokens != 240 {
		t.Fatalf("token totals = %+v", ov.Totals)
	}
}

func TestAiUsageOverviewDailySeriesIsThirtyDaysEndingToday(t *testing.T) {
	setupAiUsageDB(t)
	seedAiOverview(t)
	ov, err := (&AiUsageService{}).Overview(aiNow, AiUsageToday, 0, "")
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if len(ov.Daily) != aiDailyDays || ov.Daily[0].Day != "2026-09-07" || ov.Daily[len(ov.Daily)-1].Day != "2026-10-06" {
		t.Fatalf("daily has %d days from %s, want 30 from 2026-09-07 to 2026-10-06", len(ov.Daily), ov.Daily[0].Day)
	}
	last := ov.Daily[len(ov.Daily)-1]
	if last.ClaudeCostUsd != 5 || last.CodexCostUsd != 2 {
		t.Fatalf("today = %+v, want claude $5 (1 + 4) and codex $2", last)
	}
	for _, d := range ov.Daily {
		if d.Day == "2026-10-01" && (d.ClaudeCostUsd != 0 || d.ClaudeTokens != 0) {
			t.Fatalf("a day without usage must be zero, got %+v", d)
		}
	}
}

func TestAiUsageOverviewRanksByCost(t *testing.T) {
	setupAiUsageDB(t)
	seedAiOverview(t)
	ov, err := (&AiUsageService{}).Overview(aiNow, AiUsageMonth, 0, "")
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if len(ov.Projects) != 2 || ov.Projects[0].Name != "/w/app" || ov.Projects[0].CostUsd != 11 || ov.Projects[1].Name != "/w/lib" {
		t.Fatalf("projects = %+v, want /w/app ($11) before /w/lib", ov.Projects)
	}
	if len(ov.Models) != 3 || ov.Models[0].Name != "opus" || ov.Models[0].CostUsd != 10 || ov.Models[0].App != "claude" {
		t.Fatalf("models = %+v, want opus ($10, claude) first", ov.Models)
	}
	// Sessions that ended before the month are left out; the rest go by cost.
	if len(ov.Sessions) != 2 || ov.Sessions[0].SessionId != "s-app" || ov.Sessions[0].DeviceName != "laptop" {
		t.Fatalf("sessions = %+v, want s-app (laptop) then s-lib", ov.Sessions)
	}
	if ov.Totals.Sessions != 2 {
		t.Fatalf("session count = %d, want 2", ov.Totals.Sessions)
	}
}

func TestAiUsageOverviewFiltersByDeviceAndApp(t *testing.T) {
	setupAiUsageDB(t)
	_, desktop := seedAiOverview(t)
	s := &AiUsageService{}
	byDevice, err := s.Overview(aiNow, AiUsageMonth, desktop, "")
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if byDevice.Totals.CostUsd != 4 || len(byDevice.Sessions) != 0 {
		t.Fatalf("desktop only = %+v, want its $4 and none of the laptop's sessions", byDevice.Totals)
	}
	codex, err := s.Overview(aiNow, AiUsageMonth, 0, "codex")
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if codex.Totals.CostUsd != 2 || len(codex.Projects) != 1 || len(codex.Sessions) != 1 || codex.Sessions[0].App != "codex" {
		t.Fatalf("codex only = %+v", codex)
	}
	if _, err := s.Overview(aiNow, AiUsageMonth, 0, "gemini"); err == nil {
		t.Fatal("an unknown app filter must be refused")
	}
}

// With two computers on one account the page shows the newest reading.
func TestAiUsageOverviewShowsEachToolsNewestPlanReading(t *testing.T) {
	setupAiUsageDB(t)
	seedAiOverview(t)
	ov, err := (&AiUsageService{}).Overview(aiNow, AiUsageMonth, 0, "")
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if len(ov.Quotas) != 1 || ov.Quotas[0].PlanLabel != "Max 5x" || ov.Quotas[0].DeviceName != "desktop" ||
		len(ov.Quotas[0].Tiers) != 1 || ov.Quotas[0].Tiers[0].Utilization != 12 {
		t.Fatalf("quotas = %+v, want the desktop's newer Max 5x reading", ov.Quotas)
	}
	if len(ov.Devices) != 2 || ov.Devices[0].Name != "laptop" || ov.Devices[0].FirstDay != "2026-09-20" || ov.Devices[0].CostUsd != 59 {
		t.Fatalf("devices = %+v, want the laptop with its first day and all-time $59", ov.Devices)
	}
}

func TestAiUsageDeleteDeviceDropsItsData(t *testing.T) {
	setupAiUsageDB(t)
	laptop, desktop := seedAiOverview(t)
	if err := (&AiUsageService{}).DeleteDevice(laptop); err != nil {
		t.Fatalf("delete: %v", err)
	}
	db := database.GetDB()
	for _, m := range []any{&model.AiUsageDaily{}, &model.AiUsageSession{}, &model.AiUsageQuota{}} {
		var n int64
		db.Model(m).Where("device_id = ?", laptop).Count(&n)
		if n != 0 {
			t.Fatalf("%T still has %d rows of the deleted device", m, n)
		}
	}
	if len(aiDailyRows(t, desktop)) != 1 {
		t.Fatal("the other device's rows must stay")
	}
}
