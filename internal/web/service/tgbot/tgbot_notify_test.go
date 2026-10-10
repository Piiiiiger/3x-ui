package tgbot

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func sentTo(rec *botRecorder, chatID int64) []botCall {
	var out []botCall
	for _, c := range rec.all() {
		if c.Method == "sendMessage" && c.ChatID == chatID {
			out = append(out, c)
		}
	}
	return out
}

func turnOffDaily(t *testing.T, client *model.ClientRecord) {
	t.Helper()
	if _, err := service.EnsureAccountActivation(client); err != nil {
		t.Fatal(err)
	}
	database.GetDB().Model(&model.AccountActivation{}).Where("client_id = ?", client.Id).Update("daily_enabled", false)
}

// A fresh ban reaches the person it hits and the admin once each, the admin's
// with a button that lifts it; the next minute adds nothing.
func TestIpBanNoticeReachesThePersonAndTheAdminOnce(t *testing.T) {
	tb, rec := newPiggerBot(t)
	withProbe(t)
	ib := seedVlessInbound(t, "a")
	alice := seedClient(t, "alice", []int{ib}, func(c *model.Client) { c.LimitIP = 1 })
	bindTelegram(t, "alice", 5150)
	turnOffDaily(t, alice)
	now := time.Now()
	observe(t, now, "alice", "198.51.100.7", "203.0.113.9")
	rec.reset()

	tb.sendAccountNotifications(context.Background(), now)
	tb.sendAccountNotifications(context.Background(), now.Add(time.Minute))

	toUser, toAdmin := sentTo(rec, 5150), sentTo(rec, adminTgID)
	if len(toUser) != 1 || !strings.Contains(toUser[0].Text, "超出上限") || !strings.Contains(toUser[0].Text, "198.51.100.0/24") {
		t.Fatalf("user notices = %d, first %q; want one naming the banned IP", len(toUser), firstText(toUser))
	}
	if len(toAdmin) != 1 {
		t.Fatalf("admin notices = %d, want 1", len(toAdmin))
	}
	unban := toAdmin[0].dataFor(t, "解封")
	if want := fmt.Sprintf("pg:a:bx:%d:198.51.100.0/24!", alice.Id); unban != want {
		t.Errorf("admin unban button = %q, want %q", unban, want)
	}
	tb.tap(adminTgID, unban)
	if bans, _ := (&service.IpLimitService{}).BansForEmail("alice", time.Now()); len(bans) != 0 {
		t.Errorf("the alert's unban button left %d bans", len(bans))
	}
	if n := rec.count("editMessageText"); n != 0 {
		t.Errorf("the unban tap edited %d messages, want the alert kept and the card sent anew", n)
	}
	if card := rec.last(t); !strings.Contains(card.Text, "已解封") {
		t.Errorf("after the unban the bot showed %q", card.Text)
	}
}

func firstText(calls []botCall) string {
	if len(calls) == 0 {
		return ""
	}
	return calls[0].Text
}

// Server renewals are the admin's business: they reach the admin chat, not the
// user who happens to be called pigger, and the probe is read once an hour.
func TestServerRenewalRemindersGoToTheAdmins(t *testing.T) {
	tb, rec := newPiggerBot(t)
	ib := seedVlessInbound(t, "a")
	pigger := seedClient(t, "pigger", []int{ib}, nil)
	bindTelegram(t, "pigger", 5150)
	turnOffDaily(t, pigger)
	now := time.Date(2026, 10, 5, 21, 0, 0, 0, accountLocation)
	probeReads := withProbe(t, service.ProbeServer{Id: "srv-1", Name: "hk-1", ExpiryTime: now.AddDate(0, 0, 3).UnixMilli()})
	rec.reset()

	tb.sendAccountNotifications(context.Background(), now)
	tb.sendAccountNotifications(context.Background(), now.Add(time.Minute))

	if toAdmin := sentTo(rec, adminTgID); len(toAdmin) != 1 || !strings.Contains(toAdmin[0].Text, "hk-1") {
		t.Errorf("admin server reminders = %d (%q), want one for hk-1", len(toAdmin), firstText(toAdmin))
	}
	if n := len(sentTo(rec, 5150)); n != 0 {
		t.Errorf("the user named pigger got %d server reminders", n)
	}
	if *probeReads != 1 {
		t.Errorf("the probe was read %d times in two minutes, want once", *probeReads)
	}
}
