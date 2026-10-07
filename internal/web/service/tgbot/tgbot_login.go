package tgbot

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// After a network's first failed login is reported, its further failures wait this long
// and arrive as one summary; the admin's own networks fail mostly by typo.
const (
	newNetworkLoginHold   = time.Hour
	knownNetworkLoginHold = 24 * time.Hour
)

type heldLoginFailures struct {
	network   string
	known     bool
	opened    time.Time
	until     time.Time
	count     int
	usernames []string
}

var loginFailures = struct {
	sync.Mutex
	held map[string]*heldLoginFailures
}{held: map[string]*heldLoginFailures{}}

var loginFailReasons = map[string]string{
	"invalid credentials":      "用户名或密码错误",
	"invalid 2FA code":         "两步验证码错误",
	"too many failed attempts": "失败太多次，暂时不让登录",
}

func loginNetworkLabel(newIP bool) string {
	if newIP {
		return "新 IP"
	}
	return "登录成功过的 IP"
}

func loginClock(at time.Time) string {
	return at.In(accountLocation).Format("01-02 15:04")
}

// noteLoginAttempt tells the admins of a sign-in from a new network and of a network's
// first failure in a hold; Username arrives HTML-escaped from the login handler.
func (t *Tgbot) noteLoginAttempt(data *eventbus.LoginEventData, now time.Time) {
	lines := []string{
		"用户名：" + data.Username,
		"IP：" + esc(data.IP),
		"时间：" + loginClock(now) + "（北京时间）",
		"主机：" + esc(getHostname()),
	}
	if data.Status == "success" {
		if data.NewIP {
			t.SendMsgToTgbotAdmins("🆕 <b>新 IP 登录了面板</b>\n" + strings.Join(lines, "\n") + "\n以后从这个 IP 登录不再通知。")
		}
		return
	}
	network := service.PanelLoginNetwork(data.IP)
	loginFailures.Lock()
	if held, ok := loginFailures.held[network]; ok && now.Before(held.until) {
		held.count++
		if !slices.Contains(held.usernames, data.Username) {
			held.usernames = append(held.usernames, data.Username)
		}
		loginFailures.Unlock()
		return
	}
	ended := loginFailures.held[network]
	hold := newNetworkLoginHold
	if !data.NewIP {
		hold = knownNetworkLoginHold
	}
	loginFailures.held[network] = &heldLoginFailures{network: network, known: !data.NewIP, opened: now, until: now.Add(hold)}
	loginFailures.Unlock()
	if ended != nil && ended.count > 0 {
		t.SendMsgToTgbotAdmins(loginFailureSummary(ended))
	}

	reason := loginFailReasons[data.Reason]
	if reason == "" {
		reason = esc(data.Reason)
	}
	t.SendMsgToTgbotAdmins(fmt.Sprintf("❗ <b>面板登录失败</b>（%s）\n原因：%s\n%s\n这个 IP 之后 %d 小时内的失败合并成一条再报。",
		loginNetworkLabel(data.NewIP), reason, strings.Join(lines, "\n"), int(hold/time.Hour)))
}

// flushLoginFailures sends the summary of each ended hold that saw more failures.
func (t *Tgbot) flushLoginFailures(_ context.Context, now time.Time) {
	var ended []*heldLoginFailures
	loginFailures.Lock()
	for network, held := range loginFailures.held {
		if !now.Before(held.until) {
			delete(loginFailures.held, network)
			if held.count > 0 {
				ended = append(ended, held)
			}
		}
	}
	loginFailures.Unlock()
	slices.SortFunc(ended, func(a, b *heldLoginFailures) int { return a.opened.Compare(b.opened) })
	for _, held := range ended {
		t.SendMsgToTgbotAdmins(loginFailureSummary(held))
	}
}

func loginFailureSummary(held *heldLoginFailures) string {
	return fmt.Sprintf("❗ <b>面板登录又失败 %d 次</b>（%s）\nIP：%s\n用户名：%s\n时段：%s – %s（北京时间）",
		held.count, loginNetworkLabel(!held.known), esc(held.network), strings.Join(held.usernames, "、"),
		loginClock(held.opened), loginClock(held.until))
}
