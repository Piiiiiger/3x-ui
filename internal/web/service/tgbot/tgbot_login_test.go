package tgbot

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
)

func loginData(status, ip, username string, newIP bool) *eventbus.LoginEventData {
	return &eventbus.LoginEventData{
		Username: username, IP: ip, Time: "2026-10-07 11:23:13", Status: status,
		Reason: "invalid credentials", NewIP: newIP,
	}
}

func loginNotices(rec *botRecorder) []string {
	var out []string
	for _, m := range sentTo(rec, adminTgID) {
		if strings.Contains(m.Text, "登录") {
			out = append(out, m.Text)
		}
	}
	return out
}

// A sign-in reaches the admins only from a network that never signed in before.
func TestPanelSignInsAreReportedOnlyFromANewNetwork(t *testing.T) {
	tb, rec := newPiggerBot(t)
	tb.HandleEvent(eventbus.Event{Type: eventbus.EventLoginAttempt, Data: loginData("success", "198.51.100.7", "pigger", false)})
	if got := loginNotices(rec); len(got) != 0 {
		t.Fatalf("a sign-in from a known network was reported:\n%s", got[0])
	}
	tb.HandleEvent(eventbus.Event{Type: eventbus.EventLoginAttempt, Data: loginData("success", "203.0.113.9", "pigger", true)})
	got := loginNotices(rec)
	if len(got) != 1 || !strings.Contains(got[0], "新 IP 登录了面板") || !strings.Contains(got[0], "203.0.113.9") {
		t.Fatalf("notices = %q, want one about the new network 203.0.113.9", got)
	}
}

// Failures from one network are reported once, then summed up when the hold ends: an
// hour for a new network, a day for one the admin signed in from. IPv6 counts by /64.
func TestPanelLoginFailuresAreReportedOncePerHold(t *testing.T) {
	tb, rec := newPiggerBot(t)
	ctx := context.Background()
	t0 := tomorrowAt(10)

	tb.noteLoginAttempt(loginData("fail", "2001:db8:1:2::a", "pigger", true), t0)
	tb.noteLoginAttempt(loginData("fail", "2001:db8:1:2:bf4:6f2:9058:5d7", "Pigger", true), t0.Add(time.Minute))
	tb.noteLoginAttempt(loginData("fail", "2001:db8:1:2::a", "pigger", true), t0.Add(2*time.Minute))
	tb.noteLoginAttempt(loginData("fail", "198.51.100.7", "pigger", false), t0.Add(3*time.Minute))
	tb.noteLoginAttempt(loginData("fail", "198.51.100.7", "pigger", false), t0.Add(2*time.Hour))
	got := loginNotices(rec)
	if len(got) != 2 || !strings.Contains(got[0], "新 IP") || !strings.Contains(got[1], "登录成功过的 IP") {
		t.Fatalf("immediate notices = %q, want the first failure of each network", got)
	}

	tb.sendAccountNotifications(ctx, t0.Add(59*time.Minute))
	if n := len(loginNotices(rec)); n != 2 {
		t.Fatalf("%d notices before the new network's hour ended", n)
	}
	// The hour is over but the minute's flush has not run yet.
	tb.noteLoginAttempt(loginData("fail", "2001:db8:1:2::a", "pigger", true), t0.Add(time.Hour))
	got = loginNotices(rec)
	if len(got) != 4 || !strings.Contains(got[2], "又失败 2 次") || !strings.Contains(got[2], "Pigger、pigger") ||
		!strings.Contains(got[3], "IP：2001:db8:1:2::a") {
		t.Fatalf("notices = %q, want the /64's summary, then its new failure", got)
	}
	tb.sendAccountNotifications(ctx, t0.Add(3*time.Hour))
	tb.sendAccountNotifications(ctx, t0.Add(23*time.Hour))
	if n := len(loginNotices(rec)); n != 4 {
		t.Fatalf("%d notices, want no summary of a hold without further failures", n)
	}
	tb.sendAccountNotifications(ctx, t0.Add(25*time.Hour))
	got = loginNotices(rec)
	if len(got) != 5 || !strings.Contains(got[4], "又失败 1 次") || !strings.Contains(got[4], "198.51.100.7") {
		t.Fatalf("notices = %q, want the known network's summary after a day", got)
	}
}
