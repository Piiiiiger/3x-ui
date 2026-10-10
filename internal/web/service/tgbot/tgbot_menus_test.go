package tgbot

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// The portal's one-tap link arrives as "/start <code>": it must bind, where
// every /start used to answer with the help text.
func TestStartWithACodeBindsInOneTap(t *testing.T) {
	tb, rec := newPiggerBot(t)
	ib := seedVlessInbound(t, "a")
	alice := seedClient(t, "alice", []int{ib}, func(c *model.Client) { c.TotalGB = 100 << 30 })
	act, err := service.EnsureAccountActivation(alice)
	if err != nil {
		t.Fatal(err)
	}

	tb.send(5150, "/start "+act.Code)

	if got := clientRecord(t, "alice").TgID; got != 5150 {
		t.Fatalf("tgId after the deep link = %d, want 5150", got)
	}
	shown := rec.last(t)
	if !strings.Contains(shown.Text, "绑定成功") || !strings.Contains(shown.Text, "alice") {
		t.Errorf("reply %q does not confirm the binding", shown.Text)
	}
	if !shown.hasButton("📊 用量详情") {
		t.Errorf("no account menu after binding, buttons %q", shown.Labels)
	}
}

// /start opens each person's own menu. The admin used to get the user help,
// with no way to the admin tools.
func TestStartOpensEachPersonsOwnMenu(t *testing.T) {
	tb, rec := newPiggerBot(t)
	ib := seedVlessInbound(t, "a")
	seedClient(t, "alice", []int{ib}, nil)
	bindTelegram(t, "alice", 5150)

	tb.send(adminTgID, "/start")
	if admin := rec.last(t); !strings.Contains(admin.Text, "管理菜单") || !admin.hasButton("👥 用户列表") {
		t.Errorf("admin /start showed %q with %q, want the admin menu", admin.Text, admin.Labels)
	}
	tb.send(5150, "/start")
	if user := rec.last(t); !strings.Contains(user.Text, "alice") || !user.hasButton("🌐 在线设备") || user.hasButton("👥 用户列表") {
		t.Errorf("user /start showed %q with %q, want their own account", user.Text, user.Labels)
	}
	tb.send(777, "/start")
	if stranger := rec.last(t); strings.Contains(stranger.Text, "alice") || !strings.Contains(stranger.Text, "一键绑定") {
		t.Errorf("stranger /start showed %q, want only how to bind", stranger.Text)
	}
}

// Text that is no command opens the menu, except while an upstream wizard
// waits for that very text.
func TestPlainTextOpensTheMenuUnlessAWizardWaits(t *testing.T) {
	tb, rec := newPiggerBot(t)
	ib := seedVlessInbound(t, "a")
	seedClient(t, "alice", []int{ib}, nil)
	bindTelegram(t, "alice", 5150)

	tb.send(5150, "你好")
	if got := rec.last(t); !got.hasButton("📊 用量详情") {
		t.Errorf("plain text got %q with %q, want the account menu", got.Text, got.Labels)
	}
	userStateMgr.set(chatUser{chatID: adminTgID, userID: adminTgID}, "awaiting_email")
	if tb.handleAccountMessage(privateMessage(adminTgID, "new-user")) {
		t.Error("the account bot took the text the add-client wizard was waiting for")
	}
}

// The devices view reads the IP limit's own scan: the slots in use, the IP
// online now and a banned one with the time it has left.
func TestDevicesViewShowsOnlineIpsAndBans(t *testing.T) {
	tb, rec := newPiggerBot(t)
	ib := seedVlessInbound(t, "a")
	seedClient(t, "alice", []int{ib}, func(c *model.Client) { c.LimitIP = 1 })
	bindTelegram(t, "alice", 5150)
	observe(t, time.Now(), "alice", "198.51.100.7", "203.0.113.9")

	tb.send(5150, "/ips")

	view := rec.last(t)
	for _, want := range []string{"1/1", "203.0.113.9", "暂停使用", "198.51.100.0/24", "还剩"} {
		if !strings.Contains(view.Text, want) {
			t.Errorf("devices view lacks %q:\n%s", want, view.Text)
		}
	}
}

// The daily report is set with buttons, or by typing a time as people write it.
func TestDailySettingsByButtonsAndTypedTime(t *testing.T) {
	tb, rec := newPiggerBot(t)
	ib := seedVlessInbound(t, "a")
	alice := seedClient(t, "alice", []int{ib}, nil)
	bindTelegram(t, "alice", 5150)
	prefs := func() model.AccountActivation {
		var row model.AccountActivation
		database.GetDB().First(&row, "client_id = ?", alice.Id)
		return row
	}

	tb.tap(5150, "pg:daily:off")
	if prefs().DailyEnabled {
		t.Error("the off button left the daily report on")
	}
	tb.tap(5150, "pg:daily:t:0800")
	if p := prefs(); !p.DailyEnabled || p.DailyTime != "08:00" {
		t.Errorf("after the 08:00 button: enabled=%v time=%s", p.DailyEnabled, p.DailyTime)
	}
	tb.tap(5150, "pg:daily:ask")
	tb.send(5150, "25:00")
	if got := prefs().DailyTime; got != "08:00" {
		t.Errorf("an impossible time was saved: %s", got)
	}
	tb.send(5150, "9：05")
	if got := prefs().DailyTime; got != "09:05" {
		t.Errorf("typed 9：05 saved as %s, want 09:05", got)
	}
	if !strings.Contains(rec.last(t).Text, "09:05") {
		t.Errorf("the reply %q does not show the new time", rec.last(t).Text)
	}
}

// A button acts for whoever pressed it: a user's tap on admin data does
// nothing, and a stranger's account button shows nobody's account.
func TestButtonsActOnlyForThePersonWhoPressedThem(t *testing.T) {
	tb, rec := newPiggerBot(t)
	ib := seedVlessInbound(t, "a")
	bob := seedClient(t, "bob", []int{ib}, func(c *model.Client) { c.ExpiryTime = time.Now().Add(48 * time.Hour).UnixMilli() })
	bindTelegram(t, "bob", 6160)
	before := clientRecord(t, "bob").ExpiryTime
	rec.reset()

	tb.tap(6160, fmt.Sprintf("pg:a:rx:%d:30:1:%s", bob.Id, issueActionToken(time.Now())))
	if got := clientRecord(t, "bob").ExpiryTime; got != before {
		t.Fatal("a user renewed their own account through admin button data")
	}
	if n := len(rec.shown()); n != 0 {
		t.Errorf("a user's tap on admin data showed %d messages", n)
	}
	tb.send(6160, "/users")
	if strings.Contains(rec.last(t).Text, "用户列表") {
		t.Error("a user opened the admin's user list")
	}
	tb.tap(777, "pg:use")
	if strings.Contains(rec.last(t).Text, "bob") {
		t.Error("a stranger's tap showed an account")
	}
}

// The user page link follows the subscription address, and stays out while the
// panel only knows a host name a phone could not open.
func TestHomeLinksTheUserPageOnlyAtAPublicAddress(t *testing.T) {
	tb, rec := newPiggerBot(t)
	ib := seedVlessInbound(t, "a")
	seedClient(t, "alice", []int{ib}, nil)
	bindTelegram(t, "alice", 5150)

	tb.send(5150, "/start")
	if urls := strings.Join(rec.last(t).URLs, ""); urls != "" {
		t.Errorf("home links %q without a public address", urls)
	}
	if err := database.GetDB().Create(&model.Setting{Key: "subURI", Value: "https://sub.example.com/x/"}).Error; err != nil {
		t.Fatal(err)
	}
	tb.send(5150, "/start")
	if urls := strings.Join(rec.last(t).URLs, " "); !strings.Contains(urls, "https://sub.example.com/x/portal") {
		t.Errorf("home links %q, want the user page under the subscription address", urls)
	}
}

// Regression: a share of the quota under 1% drew "<1%", which Telegram reads as
// an unknown tag, refusing the whole view: 我的账号 and 我的用量 did nothing.
func TestAccountViewsUnderOnePercentOfTheQuotaReachTelegram(t *testing.T) {
	tb, rec := newPiggerBot(t)
	ib := seedVlessInbound(t, "a")
	seedClient(t, "pigger", []int{ib}, func(c *model.Client) { c.TotalGB = 1000 << 30 })
	setUsage(t, "pigger", 1<<30, 3<<30)
	bindTelegram(t, "pigger", adminTgID)

	tb.send(adminTgID, "/start")
	home := rec.last(t)
	tb.tap(adminTgID, home.dataFor(t, "我的账号"))
	if account := rec.last(t); !strings.Contains(account.Text, "pigger") || !account.hasButton("🛠 管理菜单") {
		t.Errorf("我的账号 showed %q, want the admin's own account", account.Text)
	}
	tb.tap(adminTgID, home.dataFor(t, "我的用量"))
	if usage := rec.last(t); !strings.Contains(usage.Text, "用量详情") || !strings.Contains(usage.Text, "1%") {
		t.Errorf("我的用量 showed %q, want the usage details with the share used", usage.Text)
	}
}
