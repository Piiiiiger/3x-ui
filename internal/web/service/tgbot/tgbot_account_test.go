package tgbot

import (
	"context"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func TestAccountReminderCalendarAndRenewal(t *testing.T) {
	now := time.Date(2026, 10, 4, 20, 0, 0, 0, accountLocation)
	for _, days := range []int{-1, 0, 1, 2, 3, 6, 7, 8, 30} {
		want := 0
		if days == 1 || days == 3 || days == 7 {
			want = days
		}
		expiry := now.AddDate(0, 0, days).Add(-time.Hour).UnixMilli()
		if got := accountReminderDay(now, expiry); got != want {
			t.Errorf("days=%d got=%d want=%d", days, got, want)
		}
	}
	for value, want := range map[string]bool{"20:00": true, "00:00": true, "23:59": true, "24:00": false, "9:00": false, "20:60": false, "": false} {
		if validDailyTime(value) != want {
			t.Error(value)
		}
	}
}

func TestAccountPrivateBindingAndPreferences(t *testing.T) {
	tb, _ := newLevelTgbot(t)
	var c model.ClientRecord
	database.GetDB().Where("email = ?", ownerMail).First(&c)
	prefs, err := service.EnsureAccountActivation(&c)
	if err != nil {
		t.Fatal(err)
	}
	m := commandFrom(555, "/bind "+prefs.Code)
	m.Chat.Type = "group"
	m.Chat.ID = -123
	if !tb.handleAccountMessage(m) {
		t.Fatal("not handled")
	}
	database.GetDB().First(&c, c.Id)
	if c.TgID != ownerTgID {
		t.Fatal("group took binding")
	}
	tb.handleAccountMessage(commandFrom(ownerTgID, "/daily off"))
	database.GetDB().First(prefs, "client_id = ?", c.Id)
	if prefs.DailyEnabled {
		t.Fatal("off not saved")
	}
	tb.handleAccountMessage(commandFrom(ownerTgID, "/daily 09:30"))
	database.GetDB().First(prefs, "client_id = ?", c.Id)
	if !prefs.DailyEnabled || prefs.DailyTime != "09:30" {
		t.Fatal("time not saved")
	}
	tb.handleAccountMessage(commandFrom(ownerTgID, "/daily 25:00"))
	database.GetDB().First(prefs, "client_id = ?", c.Id)
	if prefs.DailyTime != "09:30" {
		t.Fatal("invalid time saved")
	}
}

func TestAccountNotificationPersistsAndRenewalChangesKey(t *testing.T) {
	tb, calls := newLevelTgbot(t)
	now := time.Now()
	tb.deliverAccountNotification(context.Background(), "account:1:4242:1000:7", ownerTgID, "test", now)
	tb.deliverAccountNotification(context.Background(), "account:1:4242:1000:7", ownerTgID, "test", now)
	if calls("sendMessage") != 1 {
		t.Fatal("duplicate delivery")
	}
	tb.deliverAccountNotification(context.Background(), "account:1:4242:2000:7", ownerTgID, "renewed", now)
	if calls("sendMessage") != 2 {
		t.Fatal("new expiry suppressed")
	}
}

// Exercise the scheduler, not just its receipt keys: renewals outside the
// reminder window suppress delivery, while the new 7-day window can notify.
func TestAccountScheduleRespectsRenewalAndDailyOff(t *testing.T) {
	tb, calls := newLevelTgbot(t)
	now := time.Date(2026, 10, 4, 20, 0, 0, 0, accountLocation)
	db := database.GetDB()
	var c model.ClientRecord
	db.Where("email = ?", ownerMail).First(&c)
	prefs, err := service.EnsureAccountActivation(&c)
	if err != nil {
		t.Fatal(err)
	}
	db.Model(prefs).Update("daily_enabled", false)
	db.Model(&c).Update("expiry_time", now.AddDate(0, 0, 7).UnixMilli())
	tb.sendAccountNotifications(context.Background(), now)
	if calls("sendMessage") != 1 {
		t.Fatalf("7 day reminder count=%d", calls("sendMessage"))
	}
	tb.sendAccountNotifications(context.Background(), now.Add(time.Minute))
	if calls("sendMessage") != 1 {
		t.Fatal("duplicate reminder")
	}
	db.Model(&c).Update("expiry_time", now.AddDate(0, 0, 30).UnixMilli())
	tb.sendAccountNotifications(context.Background(), now.Add(2*time.Minute))
	if calls("sendMessage") != 1 {
		t.Fatal("renewal did not suppress previous deadline")
	}
	tb.sendAccountNotifications(context.Background(), now.AddDate(0, 0, 23))
	if calls("sendMessage") != 2 {
		t.Fatal("new expiry missing reminder")
	}
}
