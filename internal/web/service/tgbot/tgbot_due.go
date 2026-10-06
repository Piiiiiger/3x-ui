package tgbot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
	"gorm.io/gorm/clause"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

// dueFromHour and dueUntilHour bound, in Beijing time, when the admins hear of users
// close to their end: a crossing at night waits for the morning.
const dueFromHour, dueUntilHour = 9, 22

// dueButtonsMax caps the user buttons of one notice; the rest are a tap away.
const dueButtonsMax = 10

type dueUser struct {
	row   adminRow
	keys  []string
	parts []string
}

// notifyDueUsers tells the admins of users nine tenths through their quota or
// within a week of their expiry: each crossing once, in one message per check.
func (t *Tgbot) notifyDueUsers(ctx context.Context, now time.Time) {
	rows, err := t.adminRows(now)
	if err != nil {
		return
	}
	db := database.GetDB()
	var sentKeys []string
	if err := db.Model(&model.AccountNotification{}).Where("key LIKE ?", "due-%").Pluck("key", &sentKeys).Error; err != nil {
		return
	}
	var dues []dueUser
	for _, r := range rows {
		if !r.rec.Enable {
			continue
		}
		d := dueUser{row: r}
		traffic := fmt.Sprintf("due-traffic:%d", r.rec.Id)
		if total := r.rec.TotalGB; total > 0 && r.used < total && r.used*10 >= total*9 {
			d.keys = append(d.keys, traffic)
			d.parts = append(d.parts, fmt.Sprintf("流量已用 %d%%（%s / %s）", r.used*100/total, formatBytes(r.used), formatBytes(total)))
		} else if total > 0 && r.used*10 < total*9 && hasKeyPrefix(sentKeys, traffic+":") {
			// Below the line again after a reset or a bigger quota: the next crossing is news.
			db.Where("key LIKE ?", traffic+":%").Delete(&model.AccountNotification{})
		}
		if expiry := r.rec.ExpiryTime; expiry > now.UnixMilli() && calendarDaysUntil(expiry, now) <= 7 {
			d.keys = append(d.keys, fmt.Sprintf("due-expiry:%d:%d", r.rec.Id, expiry))
			d.parts = append(d.parts, "到期 "+expiryText(expiry, now))
		}
		if len(d.keys) > 0 {
			dues = append(dues, d)
		}
	}
	if hour := now.In(accountLocation).Hour(); len(dues) == 0 || hour < dueFromHour || hour >= dueUntilHour {
		return
	}
	for _, admin := range adminSnapshot() {
		var fresh []dueUser
		var keys []string
		for _, d := range dues {
			news := false
			for _, key := range d.keys {
				if key = fmt.Sprintf("%s:%d", key, admin); !notificationSent(key) {
					keys, news = append(keys, key), true
				}
			}
			if news {
				fresh = append(fresh, d)
			}
		}
		if len(fresh) > 0 {
			t.deliverAdminNotice(ctx, admin, keys, dueNotice(fresh), now)
		}
	}
}

func hasKeyPrefix(keys []string, prefix string) bool {
	for _, key := range keys {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func dueNotice(dues []dueUser) botView {
	var b strings.Builder
	b.WriteString("⏳ <b>有用户快到期或流量将尽</b>")
	var kb [][]telego.InlineKeyboardButton
	for i, d := range dues {
		fmt.Fprintf(&b, "\n• <b>%s</b>：%s", esc(d.row.rec.Email), strings.Join(d.parts, "；"))
		if i < dueButtonsMax {
			kb = append(kb, []telego.InlineKeyboardButton{button(d.row.rec.Email, fmt.Sprintf("pg:a:c:%d", d.row.rec.Id))})
		}
	}
	b.WriteString("\n\n点名字打开用户卡片，可以续期或清零流量。")
	kb = append(kb, []telego.InlineKeyboardButton{button("⏳ 需要续期的用户", "pg:a:x")})
	return botView{b.String(), keyboard(kb...)}
}

// deliverAdminNotice sends one message and, once it is out, records every key it
// carries, so each is sent once however the users group into messages.
func (t *Tgbot) deliverAdminNotice(ctx context.Context, chatID int64, keys []string, view botView, now time.Time) {
	sendCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	params := &telego.SendMessageParams{ChatID: tu.ID(chatID), Text: view.text, ParseMode: telego.ModeHTML, ReplyMarkup: view.keyboard}
	if _, err := bot.SendMessage(sendCtx, params); err != nil {
		logger.Warning("admin notice delivery failed")
		return
	}
	rows := make([]model.AccountNotification, len(keys))
	for i, key := range keys {
		rows[i] = model.AccountNotification{Key: key, SentAt: now.UnixMilli()}
	}
	if err := database.GetDB().Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error; err != nil {
		logger.Warning("admin notice receipt failed:", err)
	}
}
