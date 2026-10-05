package tgbot

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mymmrac/telego"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/global"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// botCall is one request the bot made to the Telegram API, with the parts the
// person sees: the text and every button's label, data and link.
type botCall struct {
	Method  string
	ChatID  int64
	Text    string
	Labels  []string
	Data    []string
	URLs    []string
	Payload map[string]any
}

type botRecorder struct {
	mu    sync.Mutex
	calls []botCall
}

func (r *botRecorder) all() []botCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]botCall(nil), r.calls...)
}

// shown returns what the person ended up looking at: each new message and each
// edit, in order; callback toasts and keyboard-only edits are left out.
func (r *botRecorder) shown() []botCall {
	var out []botCall
	for _, c := range r.all() {
		if c.Method == "sendMessage" || c.Method == "editMessageText" {
			out = append(out, c)
		}
	}
	return out
}

func (r *botRecorder) last(t *testing.T) botCall {
	t.Helper()
	shown := r.shown()
	if len(shown) == 0 {
		t.Fatal("the bot showed nothing")
	}
	return shown[len(shown)-1]
}

func (r *botRecorder) count(method string) int {
	n := 0
	for _, c := range r.all() {
		if c.Method == method {
			n++
		}
	}
	return n
}

func (r *botRecorder) reset() {
	r.mu.Lock()
	r.calls = nil
	r.mu.Unlock()
}

func (c botCall) hasButton(label string) bool {
	for _, l := range c.Labels {
		if l == label {
			return true
		}
	}
	return false
}

// dataFor returns the callback data behind the button whose label holds label.
func (c botCall) dataFor(t *testing.T, label string) string {
	t.Helper()
	for i, l := range c.Labels {
		if strings.Contains(l, label) && c.Data[i] != "" {
			return c.Data[i]
		}
	}
	t.Fatalf("no button %q among %q", label, c.Labels)
	return ""
}

func recordingTelegram(t *testing.T) *botRecorder {
	t.Helper()
	rec := &botRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		call := botCall{Method: method, Payload: payload}
		if id, ok := payload["chat_id"].(float64); ok {
			call.ChatID = int64(id)
		}
		call.Text, _ = payload["text"].(string)
		if markup, ok := payload["reply_markup"].(map[string]any); ok {
			rows, _ := markup["inline_keyboard"].([]any)
			for _, row := range rows {
				buttons, _ := row.([]any)
				for _, b := range buttons {
					button, _ := b.(map[string]any)
					label, _ := button["text"].(string)
					data, _ := button["callback_data"].(string)
					link, _ := button["url"].(string)
					call.Labels = append(call.Labels, label)
					call.Data = append(call.Data, data)
					call.URLs = append(call.URLs, link)
				}
			}
		}
		rec.mu.Lock()
		rec.calls = append(rec.calls, call)
		rec.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		result := any(true)
		switch method {
		case "sendMessage", "editMessageText":
			result = map[string]any{"message_id": 7, "date": 0, "chat": map[string]any{"id": call.ChatID, "type": "private"}}
		case "getMe":
			result = map[string]any{"id": 1, "is_bot": true, "first_name": "bot", "username": "pigger_test_bot"}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
	}))
	t.Cleanup(srv.Close)
	swapTestBot(t, srv.URL)
	return rec
}

// newPiggerBot gives each test a migrated DB, a recording Telegram API and a
// running bot whose only admin is adminTgID.
func newPiggerBot(t *testing.T) (*Tgbot, *botRecorder) {
	t.Helper()
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	rec := recordingTelegram(t)
	origRunning, origStorage := isRunning, hashStorage
	isRunning, hashStorage = true, global.NewHashStorage(20*time.Minute)
	t.Cleanup(func() { isRunning, hashStorage = origRunning, origStorage })
	withAdmins(t, adminTgID)
	userStateMgr.reset()
	t.Cleanup(userStateMgr.reset)
	resetAccountPacing()
	t.Cleanup(resetAccountPacing)
	inviteAttemptsMu.Lock()
	inviteAttemptsBy = map[int64]*inviteAttempts{}
	inviteAttemptsMu.Unlock()
	// A scan's snapshot outlives its test; an empty one starts this test clean.
	if _, err := (&service.IpLimitService{}).Enforce(time.Now(), nil); err != nil {
		t.Fatalf("clear online snapshot: %v", err)
	}
	return &Tgbot{}, rec
}

func resetAccountPacing() {
	accountPacing.Lock()
	accountPacing.serversAt, accountPacing.servers, accountPacing.prunedOn = time.Time{}, nil, ""
	accountPacing.Unlock()
}

// withProbe stands in for Lite, counting how often the scheduler asks it.
func withProbe(t *testing.T, servers ...service.ProbeServer) *int {
	t.Helper()
	calls := new(int)
	orig := probeSnapshot
	probeSnapshot = func(context.Context) (service.ProbeSnapshot, bool, error) {
		*calls++
		return service.ProbeSnapshot{Servers: servers}, true, nil
	}
	t.Cleanup(func() { probeSnapshot = orig })
	return calls
}

// observe runs one IP-limit scan in which each email is online from the given
// addresses, the way the 10 s job feeds it.
func observe(t *testing.T, now time.Time, email string, ips ...string) {
	t.Helper()
	seen := make([]service.IpObservation, 0, len(ips))
	for i, ip := range ips {
		seen = append(seen, service.IpObservation{Email: email, IP: ip, LastSeen: now.Unix() - int64(len(ips)-i), Server: 0})
	}
	if _, err := (&service.IpLimitService{}).Enforce(now, seen); err != nil {
		t.Fatalf("scan: %v", err)
	}
}

const adminTgID = int64(9001)

var nextTestPort = 47400

func seedVlessInbound(t *testing.T, remark string) int {
	t.Helper()
	nextTestPort++
	in := &model.Inbound{
		Tag: "in-" + remark, Remark: remark, Enable: true, Listen: "0.0.0.0", Port: nextTestPort,
		Protocol: model.VLESS, StreamSettings: `{"network":"tcp"}`, Settings: `{"clients":[]}`,
	}
	if err := database.GetDB().Create(in).Error; err != nil {
		t.Fatalf("seed inbound %s: %v", remark, err)
	}
	return in.Id
}

// seedClient makes a client the way the panel does, on the given inbounds.
func seedClient(t *testing.T, email string, inboundIds []int, edit func(*model.Client)) *model.ClientRecord {
	t.Helper()
	client := model.Client{Email: email, ID: uuid.NewString(), SubID: "sub-" + uuid.NewString()[:8], Enable: true}
	if edit != nil {
		edit(&client)
	}
	if _, err := (&service.ClientService{}).Create(&service.InboundService{}, &service.ClientCreatePayload{
		Client: client, InboundIds: inboundIds,
	}); err != nil {
		t.Fatalf("create client %s: %v", email, err)
	}
	return clientRecord(t, email)
}

func clientRecord(t *testing.T, email string) *model.ClientRecord {
	t.Helper()
	rec, err := (&service.ClientService{}).GetRecordByEmail(nil, email)
	if err != nil {
		t.Fatalf("read client %s: %v", email, err)
	}
	return rec
}

func setUsage(t *testing.T, email string, up, down int64) {
	t.Helper()
	if err := database.GetDB().Model(&xray.ClientTraffic{}).Where("email = ?", email).
		Updates(map[string]any{"up": up, "down": down}).Error; err != nil {
		t.Fatalf("set usage of %s: %v", email, err)
	}
}

func usageOf(t *testing.T, email string) int64 {
	t.Helper()
	traffic, err := (&service.InboundService{}).GetClientTrafficByEmail(email)
	if err != nil || traffic == nil {
		t.Fatalf("usage of %s: %v", email, err)
	}
	return traffic.Up + traffic.Down
}

// bindTelegram binds a client the way the account bot does.
func bindTelegram(t *testing.T, email string, tgID int64) {
	t.Helper()
	act, err := service.EnsureAccountActivation(clientRecord(t, email))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.BindAccountActivation(act.Code, tgID); err != nil {
		t.Fatalf("bind %s: %v", email, err)
	}
}

func privateMessage(from int64, text string) *telego.Message {
	return &telego.Message{
		From: &telego.User{ID: from, FirstName: "Test"},
		Chat: telego.Chat{ID: from, Type: telego.ChatTypePrivate},
		Text: text,
	}
}

// send runs a private message through the same steps the long-poll handlers do.
func (tb *Tgbot) send(from int64, text string) {
	m := privateMessage(from, text)
	if strings.HasPrefix(text, "/") {
		userStateMgr.clear(messageActor(*m))
	}
	if tb.handleAccountMessage(m) {
		return
	}
	if strings.HasPrefix(text, "/") {
		if isAdmin, ok := tb.gateCommand(m); ok {
			tb.answerCommand(m, m.Chat.ID, isAdmin)
		}
	}
}

// tap presses a button the way Telegram delivers it, from a private chat.
func (tb *Tgbot) tap(from int64, data string) {
	q := &telego.CallbackQuery{
		ID:      "q",
		From:    telego.User{ID: from},
		Data:    data,
		Message: &telego.Message{MessageID: 7, Chat: telego.Chat{ID: from, Type: telego.ChatTypePrivate}},
	}
	if tb.handleAccountCallback(q) {
		return
	}
	if isAdmin, ok := tb.gateCallback(q); ok {
		tb.answerCallback(q, isAdmin)
	}
}
