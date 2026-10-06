package tgbot

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

const reviewedAI = "payload:\n  # Example AI / Core\n  - DOMAIN-SUFFIX,example-ai.com\n"

var ruleSetsChanged = time.Date(2026, 10, 6, 10, 0, 0, 0, accountLocation)

// ruleSetPending leaves the ai set with the latest upstream list unreviewed since
// ruleSetsChanged.
func ruleSetPending(t *testing.T, columns map[string]any) {
	t.Helper()
	base := map[string]any{
		"reviewed": reviewedAI, "reviewed_at": ruleSetsChanged.Add(-24 * time.Hour).UnixMilli(), "latest": reviewedAI,
		"fetched_at": ruleSetsChanged.UnixMilli(), "pending_since": ruleSetsChanged.UnixMilli(),
	}
	for k, v := range columns {
		base[k] = v
	}
	res := database.GetDB().Model(&model.RuleSet{}).Where("name = ?", "ai").Updates(base)
	if res.Error != nil || res.RowsAffected != 1 {
		t.Fatalf("seed the ai rule set: %v (%d rows)", res.Error, res.RowsAffected)
	}
}

// Upstream changes worth a review reach the admins once, saying what changed and
// what to look at closely, then once a week while the review waits.
func TestRuleSetReviewNoticeReachesTheAdminsOnceThenWeekly(t *testing.T) {
	tb, rec := newPiggerBot(t)
	withProbe(t)
	ruleSetPending(t, map[string]any{"latest": reviewedAI +
		"  # New AI / Core\n  - DOMAIN-SUFFIX,new-ai.com\n  # New AI / Third-Party\n  - DOMAIN-KEYWORD,sentry\n"})
	ctx, first := context.Background(), ruleSetsChanged.Add(time.Minute)

	tb.sendAccountNotifications(ctx, first)
	tb.sendAccountNotifications(ctx, first.Add(time.Minute))
	notices := sentTo(rec, adminTgID)
	if len(notices) != 1 {
		t.Fatalf("notices = %d, want 1", len(notices))
	}
	for _, want := range []string{
		"AI 规则建议更新", "评分 17 / 10", "自 10-05 审核以来", "新服务 1 个（New AI）",
		"新增 2 条（核心 1、第三方 1）", "DOMAIN-KEYWORD,sentry（关键词", "/pigger-ai-rules",
	} {
		if !strings.Contains(notices[0].Text, want) {
			t.Errorf("notice lacks %q:\n%s", want, notices[0].Text)
		}
	}

	rec.reset()
	tb.sendAccountNotifications(ctx, first.Add(6*24*time.Hour))
	if n := len(sentTo(rec, adminTgID)); n != 0 {
		t.Fatalf("%d reminders before a week", n)
	}
	tb.sendAccountNotifications(ctx, first.Add(7*24*time.Hour))
	tb.sendAccountNotifications(ctx, first.Add(7*24*time.Hour+time.Minute))
	reminders := sentTo(rec, adminTgID)
	if len(reminders) != 1 || !strings.Contains(reminders[0].Text, "AI 规则仍待审核") || !strings.Contains(reminders[0].Text, "已等 7 天") {
		t.Fatalf("reminders after a week = %d (%q), want 1", len(reminders), firstText(reminders))
	}
	rec.reset()
	tb.sendAccountNotifications(ctx, first.Add(14*24*time.Hour))
	if reminders := sentTo(rec, adminTgID); len(reminders) != 1 || !strings.Contains(reminders[0].Text, "已等 14 天") {
		t.Fatalf("reminders after two weeks = %d (%q), want 1", len(reminders), firstText(reminders))
	}
}

// Changes that do not reach the score wait quietly.
func TestSmallRuleSetChangesSendNoNotice(t *testing.T) {
	tb, rec := newPiggerBot(t)
	withProbe(t)
	ruleSetPending(t, map[string]any{"latest": reviewedAI + "  # Example AI / Third-Party\n  - DOMAIN,telemetry.example-ai.net\n"})

	tb.sendAccountNotifications(context.Background(), ruleSetsChanged.Add(time.Minute))
	if notices := sentTo(rec, adminTgID); len(notices) != 0 {
		t.Fatalf("a 1-point change sent %q", firstText(notices))
	}
}

// Fetching that keeps failing reaches the admins once, with the error; the rules
// subscriptions get are untouched by it.
func TestFailingUpstreamFetchesReachTheAdminsOnce(t *testing.T) {
	tb, rec := newPiggerBot(t)
	withProbe(t)
	ctx := context.Background()
	failing := map[string]any{"pending_since": 0, "fetch_failures": 2, "failing_since": ruleSetsChanged.UnixMilli(), "fetch_error": "HTTP 503"}
	ruleSetPending(t, failing)
	tb.sendAccountNotifications(ctx, ruleSetsChanged.Add(48*time.Hour))
	if notices := sentTo(rec, adminTgID); len(notices) != 0 {
		t.Fatalf("two failures sent %q", firstText(notices))
	}

	failing["fetch_failures"] = 3
	ruleSetPending(t, failing)
	tb.sendAccountNotifications(ctx, ruleSetsChanged.Add(72*time.Hour))
	tb.sendAccountNotifications(ctx, ruleSetsChanged.Add(73*time.Hour))
	notices := sentTo(rec, adminTgID)
	if len(notices) != 1 || !strings.Contains(notices[0].Text, "连续 3 次") || !strings.Contains(notices[0].Text, "HTTP 503") {
		t.Fatalf("notices = %d (%q), want one naming the failures and the error", len(notices), firstText(notices))
	}

	rec.reset()
	failing["failing_since"] = ruleSetsChanged.Add(10 * 24 * time.Hour).UnixMilli()
	ruleSetPending(t, failing)
	tb.sendAccountNotifications(ctx, ruleSetsChanged.Add(13*24*time.Hour))
	if notices := sentTo(rec, adminTgID); len(notices) != 1 {
		t.Fatalf("a later failing streak sent %d notices, want 1", len(notices))
	}
}
