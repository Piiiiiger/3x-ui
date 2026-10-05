package tgbot

import (
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
	return &Tgbot{}, rec
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

// tap presses a button the way Telegram delivers it, from a private chat.
func (tb *Tgbot) tap(from int64, data string) {
	q := &telego.CallbackQuery{
		ID:      "q",
		From:    telego.User{ID: from},
		Data:    data,
		Message: &telego.Message{MessageID: 7, Chat: telego.Chat{ID: from, Type: telego.ChatTypePrivate}},
	}
	if isAdmin, ok := tb.gateCallback(q); ok {
		tb.answerCallback(q, isAdmin)
	}
}
