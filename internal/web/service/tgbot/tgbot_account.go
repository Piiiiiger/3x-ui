package tgbot

import (
	"context"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
	"gorm.io/gorm/clause"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

var accountLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

const accountBotHelp = "将用户页面的激活码发给我，即可绑定账号。\n\n/bind 激活码 — 绑定账号\n/report — 查询用量\n/daily — 查看日报设置\n/daily off — 关闭日报\n/daily on — 开启日报\n/daily 20:00 — 修改发送时间（北京时间）\n\n账号到期前 7、3、1 天自动提醒；续费后按新日期提醒。"

// Account credentials and reports are accepted only in a private conversation.
func (t *Tgbot) handleAccountMessage(m *telego.Message) bool {
	if m.From == nil {
		return false
	}
	fields := strings.Fields(m.Text)
	if len(fields) == 0 {
		return false
	}
	cmd := strings.Split(fields[0], "@")[0]
	isCommand := cmd == "/bind" || cmd == "/daily" || cmd == "/report"
	isCode := !strings.HasPrefix(cmd, "/") && len(fields) == 1 && len(fields[0]) == 19
	if !isCommand && !isCode && cmd != "/start" && cmd != "/help" {
		return false
	}
	if m.Chat.Type != "private" || m.Chat.ID != m.From.ID {
		if isCommand || isCode {
			t.SendMsgToTgbot(m.Chat.ID, "请在与机器人的私聊中绑定或查询账号。")
		}
		return true
	}
	if cmd == "/start" || cmd == "/help" {
		t.SendMsgToTgbot(m.Chat.ID, accountBotHelp)
		return true
	}
	if cmd == "/bind" || isCode {
		code := fields[0]
		if cmd == "/bind" {
			if len(fields) != 2 {
				t.SendMsgToTgbot(m.Chat.ID, "用法：/bind 你的激活码")
				return true
			}
			code = fields[1]
		}
		if !t.allowInviteAttempt(m.From) {
			t.SendMsgToTgbot(m.Chat.ID, "尝试过于频繁，请稍后再试。")
			return true
		}
		client, err := service.BindAccountActivation(code, m.From.ID)
		if err != nil {
			t.SendMsgToTgbot(m.Chat.ID, "无法绑定：请确认激活码正确，且账号与 Telegram 均未绑定其他用户。")
			return true
		}
		prefs, err := service.EnsureAccountActivation(client)
		if err != nil {
			t.SendMsgToTgbot(m.Chat.ID, "绑定已完成，暂时无法读取日报设置，请稍后使用 /daily 查询。")
			return true
		}
		t.SendMsgToTgbot(m.Chat.ID, "已绑定账号 <b>"+html.EscapeString(client.Email)+"</b>。\n"+accountPreferences(prefs)+"\n\n"+accountBotHelp)
		return true
	}
	var client model.ClientRecord
	if err := database.GetDB().Where("tg_id = ?", m.From.ID).First(&client).Error; err != nil {
		t.SendMsgToTgbot(m.Chat.ID, "请先发送用户页面的激活码绑定账号。")
		return true
	}
	prefs, err := service.EnsureAccountActivation(&client)
	if err != nil {
		t.SendMsgToTgbot(m.Chat.ID, "暂时无法读取账号，请稍后重试。")
		return true
	}
	if cmd == "/report" {
		report, err := t.accountReport(&client, time.Now())
		if err != nil {
			t.SendMsgToTgbot(m.Chat.ID, "暂时无法读取流量，请稍后重试。")
		} else {
			t.SendMsgToTgbot(m.Chat.ID, report)
		}
		return true
	}
	if len(fields) > 1 {
		value := fields[1]
		updates := map[string]any{}
		switch value {
		case "on":
			updates["daily_enabled"] = true
		case "off":
			updates["daily_enabled"] = false
		default:
			if !validDailyTime(value) || len(fields) != 2 {
				t.SendMsgToTgbot(m.Chat.ID, "时间格式为 HH:MM，例如 /daily 20:00（北京时间）。")
				return true
			}
			updates["daily_time"] = value
			updates["daily_enabled"] = true
		}
		if err := database.GetDB().Model(prefs).Updates(updates).Error; err != nil {
			t.SendMsgToTgbot(m.Chat.ID, "保存失败，请稍后重试。")
			return true
		}
	}
	t.SendMsgToTgbot(m.Chat.ID, accountPreferences(prefs))
	return true
}

func validDailyTime(value string) bool {
	parsed, err := time.Parse("15:04", value)
	return err == nil && parsed.Format("15:04") == value
}

func accountPreferences(p *model.AccountActivation) string {
	state := "已关闭"
	if p.DailyEnabled {
		state = "已开启"
	}
	return fmt.Sprintf("日报%s，发送时间：%s（北京时间）。\n/daily off 关闭 · /daily on 开启 · /daily 09:00 修改时间", state, p.DailyTime)
}

func (t *Tgbot) accountReport(client *model.ClientRecord, now time.Time) (string, error) {
	traffic, err := t.inboundService.GetClientTrafficByEmail(client.Email)
	if err != nil {
		return "", err
	}
	if traffic == nil {
		return "", fmt.Errorf("account has no traffic record")
	}
	used := traffic.Up + traffic.Down
	quota, remaining := "不限量", "不限量"
	if client.TotalGB > 0 {
		quota = accountBytes(client.TotalGB)
		remaining = accountBytes(max(0, client.TotalGB-used))
	}
	expiry := "长期有效"
	if client.ExpiryTime > 0 {
		expiry = time.UnixMilli(client.ExpiryTime).In(accountLocation).Format("2006-01-02 15:04")
	}
	days, err := (&service.TrafficStatsService{}).ClientDaily(client.Email, now.In(accountLocation), 1)
	if err != nil {
		return "", err
	}
	var today int64
	for _, day := range days {
		today += day.Up + day.Down
	}
	return fmt.Sprintf("<b>%s · 用量报告</b>\n统计时间：%s（北京时间）\n剩余流量：%s / %s\n本期已用：%s\n上传：%s · 下载：%s\n今日已用：%s\n到期：%s", html.EscapeString(client.Email), now.In(accountLocation).Format("01-02 15:04"), remaining, quota, accountBytes(used), accountBytes(traffic.Up), accountBytes(traffic.Down), accountBytes(today), expiry), nil
}
func accountBytes(n int64) string { return fmt.Sprintf("%.2f GB", float64(n)/(1024*1024*1024)) }

// Use calendar days in the user's stated time zone. An expired timestamp must
// never produce a pre-expiry message, including on the same date.
func accountReminderDay(now time.Time, expiry int64) int {
	if expiry <= now.UnixMilli() {
		return 0
	}
	date := func(t time.Time) time.Time {
		y, m, d := t.In(accountLocation).Date()
		return time.Date(y, m, d, 0, 0, 0, 0, accountLocation)
	}
	days := int(date(time.UnixMilli(expiry)).Sub(date(now)).Hours() / 24)
	if days == 7 || days == 3 || days == 1 {
		return days
	}
	return 0
}

func (t *Tgbot) runAccountNotifications(ctx context.Context) {
	defer botWG.Done()
	defer recoverBotPanic()
	// Existing accounts acquire permanent credentials before the first bind.
	var clients []model.ClientRecord
	if err := database.GetDB().Find(&clients).Error; err == nil {
		for i := range clients {
			if _, err := service.EnsureAccountActivation(&clients[i]); err != nil {
				logger.Warning("account credential initialization failed:", err)
			}
		}
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			t.sendAccountNotifications(ctx, now)
		}
	}
}

// Single scheduler goroutine; mark only successful delivery, allowing retries
// after transient failures. Durable keys survive restarts and include new expiry.
func (t *Tgbot) deliverAccountNotification(ctx context.Context, key string, chatID int64, text string, now time.Time) {
	var count int64
	if err := database.GetDB().Model(&model.AccountNotification{}).Where("key = ?", key).Count(&count).Error; err != nil || count > 0 {
		return
	}
	sendCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if _, err := bot.SendMessage(sendCtx, &telego.SendMessageParams{ChatID: tu.ID(chatID), Text: text, ParseMode: "HTML"}); err != nil {
		logger.Warning("account notification delivery failed")
		return
	}
	row := model.AccountNotification{Key: key, SentAt: now.UnixMilli()}
	if err := database.GetDB().Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		logger.Warning("account notification receipt failed:", err)
	}
}

func (t *Tgbot) sendAccountNotifications(ctx context.Context, now time.Time) {
	var clients []model.ClientRecord
	if err := database.GetDB().Where("tg_id > 0").Find(&clients).Error; err != nil {
		return
	}
	local := now.In(accountLocation)
	for i := range clients {
		if ctx.Err() != nil {
			return
		}
		client := &clients[i]
		prefs, err := service.EnsureAccountActivation(client)
		if err != nil {
			continue
		}
		// Catch up today's report after a restart, without replaying previous dates.
		if prefs.DailyEnabled && validDailyTime(prefs.DailyTime) && local.Format("15:04") >= prefs.DailyTime {
			key := fmt.Sprintf("daily:%d:%d:%s", client.Id, client.TgID, local.Format("2006-01-02"))
			if report, err := t.accountReport(client, now); err == nil {
				t.deliverAccountNotification(ctx, key, client.TgID, report, now)
			}
		}
		if day := accountReminderDay(now, client.ExpiryTime); day > 0 && local.Format("15:04") >= prefs.DailyTime {
			key := fmt.Sprintf("account:%d:%d:%d:%d", client.Id, client.TgID, client.ExpiryTime, day)
			msg := fmt.Sprintf("<b>账号续费提醒</b>\n%s 将于 %s 到期（还有 %d 天）。\n如需续费，请联系管理员。", html.EscapeString(client.Email), time.UnixMilli(client.ExpiryTime).In(accountLocation).Format("2006-01-02 15:04"), day)
			// Read expiry again immediately before delivery: a renewal suppresses old work.
			var current model.ClientRecord
			if err := database.GetDB().First(&current, client.Id).Error; err == nil && current.ExpiryTime == client.ExpiryTime && current.TgID == client.TgID {
				t.deliverAccountNotification(ctx, key, client.TgID, msg, now)
			}
		}
		if client.Email == "pigger" && local.Format("15:04") >= prefs.DailyTime {
			snap, configured, err := (&service.ProbeService{}).Snapshot(ctx)
			if err != nil || !configured || snap.Error != "" {
				continue
			}
			for _, server := range snap.Servers {
				day := accountReminderDay(now, server.ExpiryTime)
				if day == 0 {
					continue
				}
				key := fmt.Sprintf("server:%s:%d:%d:%d", server.Id, client.TgID, server.ExpiryTime, day)
				msg := fmt.Sprintf("<b>服务器续费提醒</b>\n%s 将于 %s 到期（还有 %d 天）。\n数据来源：探针；续费后请更新探针到期日期。", html.EscapeString(server.Name), time.UnixMilli(server.ExpiryTime).In(accountLocation).Format("2006-01-02 15:04"), day)
				t.deliverAccountNotification(ctx, key, client.TgID, msg, now)
			}
		}
	}
	// Bounded delivery history; reminders cannot recur after their calendar date.
	database.GetDB().Where("sent_at < ?", now.AddDate(0, 0, -45).UnixMilli()).Delete(&model.AccountNotification{})
}
