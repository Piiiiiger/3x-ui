package tgbot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// ruleSetFailuresToTell is how many failed fetches in a row the admins hear about.
const ruleSetFailuresToTell = 3

const ruleSetRemindEvery = 7 * 24 * time.Hour

var ruleRiskLabels = map[string]string{
	"keyword": "关键词，可能匹配无关域名",
	"regex":   "正则，可能匹配无关域名",
	"asn":     "IP-ASN，客户端要下载 ASN 库",
	"wide":    "IP 段过大",
	"shared":  "整个平台的域名",
	"tld":     "整个顶级域",
	"unknown": "未知规则类型",
}

// notifyRuleSets tells the admins once when upstream changes are worth a review,
// then weekly while it waits, and once when fetching keeps failing.
func (t *Tgbot) notifyRuleSets(ctx context.Context, now time.Time) {
	w, err := (&service.RuleSetService{}).Watch(now)
	if err != nil {
		return
	}
	for _, admin := range adminSnapshot() {
		if w.Due {
			first := fmt.Sprintf("rule-sets:%d:%d", w.PendingSince, admin)
			t.deliverAccountNotification(ctx, first, admin, ruleSetNotice(w, now, false), now)
			if sentAt, ok := notificationSentAt(first); ok {
				if weeks := int(now.Sub(sentAt) / ruleSetRemindEvery); weeks > 0 {
					t.deliverAccountNotification(ctx, fmt.Sprintf("%s:w%d", first, weeks), admin, ruleSetNotice(w, now, true), now)
				}
			}
		}
		if w.FetchFailures >= ruleSetFailuresToTell {
			text := fmt.Sprintf("⚠️ <b>AI 规则的上游列表连续 %d 次获取失败</b>\n最近的错误：%s\n已审核的规则照常使用，不受影响；上游恢复后每天的检查会自动继续。",
				w.FetchFailures, esc(w.FetchError))
			t.deliverAccountNotification(ctx, fmt.Sprintf("rule-sets-fetch:%d:%d", w.FailingSince, admin), admin, botView{text: text}, now)
		}
	}
}

func ruleSetNotice(w *service.RuleSetWatch, now time.Time, reminder bool) botView {
	var b strings.Builder
	score := strconv.FormatFloat(w.Score, 'f', -1, 64) + " / " + strconv.FormatFloat(w.Threshold, 'f', -1, 64)
	if reminder {
		days := int(now.Sub(time.UnixMilli(w.PendingSince)) / (24 * time.Hour))
		fmt.Fprintf(&b, "⏰ <b>AI 规则仍待审核</b>（评分 %s，已等 %d 天）\n", score, days)
	} else {
		fmt.Fprintf(&b, "🧩 <b>AI 规则建议更新</b>（评分 %s）\n", score)
	}
	var risky []service.RuleSetChange
	for _, set := range w.Sets {
		if len(set.Added)+len(set.Removed) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n<b>%s</b>：%s", esc(set.Name), ruleSetChangeText(set))
		for _, added := range set.Added {
			if added.Risk != "" {
				risky = append(risky, added)
			}
		}
	}
	if len(risky) > 0 {
		b.WriteString("\n\n需要重点审查：")
		for i, r := range risky {
			if i == 5 {
				fmt.Fprintf(&b, "\n…共 %d 条", len(risky))
				break
			}
			fmt.Fprintf(&b, "\n• %s（%s）", esc(r.Rule), ruleRiskLabels[r.Risk])
		}
	}
	b.WriteString("\n\n在 Claude Code 里运行 /pigger-ai-rules 审核并更新；审核之前，用户拿到的规则不变。")
	return botView{text: b.String()}
}

func ruleSetChangeText(set service.RuleSetChanges) string {
	since := "尚未审核过，上游现有"
	if set.ReviewedAt > 0 {
		since = "自 " + time.UnixMilli(set.ReviewedAt).In(accountLocation).Format("01-02") + " 审核以来"
	}
	var parts []string
	if n := len(set.NewProviders); n > 0 {
		parts = append(parts, fmt.Sprintf("新服务 %d 个（%s）", n, firstNames(set.NewProviders)))
	}
	if n := len(set.Added); n > 0 {
		parts = append(parts, fmt.Sprintf("新增 %d 条（%s）", n, tierTally(set.Added)))
	}
	if n := len(set.Removed); n > 0 {
		parts = append(parts, fmt.Sprintf("删除 %d 条", n))
	}
	if n := len(set.GoneProviders); n > 0 {
		parts = append(parts, fmt.Sprintf("下线服务 %d 个（%s）", n, firstNames(set.GoneProviders)))
	}
	return since + "，" + strings.Join(parts, "；")
}

func firstNames(names []string) string {
	shown := names[:min(len(names), 4)]
	out := esc(strings.Join(shown, "、"))
	if len(names) > len(shown) {
		out += " 等"
	}
	return out
}

// tierTally counts added rules the way upstream marks them; process rules apart.
func tierTally(added []service.RuleSetChange) string {
	labels := []string{"核心", "第三方", "网络", "进程", "其他"}
	counts := map[string]int{}
	for _, a := range added {
		switch {
		case strings.HasPrefix(a.Rule, "PROCESS-NAME,"):
			counts["进程"]++
		case a.Tier == "Core":
			counts["核心"]++
		case a.Tier == "Third-Party":
			counts["第三方"]++
		case a.Tier == "Network":
			counts["网络"]++
		default:
			counts["其他"]++
		}
	}
	var parts []string
	for _, label := range labels {
		if counts[label] > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", label, counts[label]))
		}
	}
	return strings.Join(parts, "、")
}
