package service

import (
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// resyncInbound does what an inbound save and the traffic job both do: feed the
// inbound's stored settings JSON back through SyncInbound.
func resyncInbound(t *testing.T, inboundId int) {
	t.Helper()
	var ib model.Inbound
	if err := database.GetDB().First(&ib, inboundId).Error; err != nil {
		t.Fatalf("read inbound %d: %v", inboundId, err)
	}
	clients, err := (&InboundService{}).GetClients(&ib)
	if err != nil {
		t.Fatalf("clients of inbound %d: %v", inboundId, err)
	}
	if err := (&ClientService{}).SyncInbound(nil, inboundId, clients); err != nil {
		t.Fatalf("resync inbound %d: %v", inboundId, err)
	}
}

func assertTelegramBinding(t *testing.T, email string, tgID int64, inboundIds ...int) {
	t.Helper()
	if got := planRecord(t, email).TgID; got != tgID {
		t.Errorf("%s record tgId = %d, want %d", email, got, tgID)
	}
	for _, id := range inboundIds {
		if got := inboundClientEntry(t, id, email)["tgId"]; got != float64(tgID) {
			t.Errorf("%s tgId in inbound %d settings = %v, want %d", email, id, got, tgID)
		}
	}
}

// Regression: the bot bound only the clients row, so the next save of any of
// the client's inbounds copied the settings JSON's tgId 0 back over it.
func TestAccountBindingSurvivesASaveOfTheClientsInbounds(t *testing.T) {
	a, b, _ := setupPlanDB(t)
	createPlanClient(t, "bound@tg", []int{a, b}, 0)
	act, err := EnsureAccountActivation(planRecord(t, "bound@tg"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := BindAccountActivation(act.Code, 5150); err != nil {
		t.Fatalf("bind: %v", err)
	}

	resyncInbound(t, b)
	resyncInbound(t, a)

	assertTelegramBinding(t, "bound@tg", 5150, a, b)
}

// Upstream's setter wrote the first inbound only, which loses the binding the
// same way once the client's other inbound is saved.
func TestSetClientTelegramUserIDBindsEveryInbound(t *testing.T) {
	a, b, _ := setupPlanDB(t)
	createPlanClient(t, "shared@tg", []int{a, b}, 0)
	traffic, err := (&InboundService{}).GetClientTrafficByEmail("shared@tg")
	if err != nil || traffic == nil {
		t.Fatalf("traffic row: %v", err)
	}

	if _, err := (&ClientService{}).SetClientTelegramUserID(&InboundService{}, traffic.Id, 6160); err != nil {
		t.Fatalf("set: %v", err)
	}
	resyncInbound(t, b)
	assertTelegramBinding(t, "shared@tg", 6160, a, b)

	if _, err := (&ClientService{}).SetClientTelegramUserID(&InboundService{}, traffic.Id, 0); err != nil {
		t.Fatalf("clear: %v", err)
	}
	resyncInbound(t, a)
	assertTelegramBinding(t, "shared@tg", 0, a, b)
}

// A code is bound however it was typed, the way the portal reads one at sign-up.
func TestBindAcceptsACodeTypedWithoutDashesOrCapitals(t *testing.T) {
	a, _, _ := setupPlanDB(t)
	createPlanClient(t, "typed@tg", []int{a}, 0)
	act, err := EnsureAccountActivation(planRecord(t, "typed@tg"))
	if err != nil {
		t.Fatal(err)
	}
	typed := strings.ToLower(strings.ReplaceAll(act.Code, "-", " "))

	if !LooksLikeActivationCode(typed) {
		t.Fatalf("LooksLikeActivationCode(%q) = false for a real code", typed)
	}
	if _, _, err := BindAccountActivation(typed, 7170); err != nil {
		t.Fatalf("bind %q: %v", typed, err)
	}
	assertTelegramBinding(t, "typed@tg", 7170, a)
	for _, text := range []string{"hello there", "IOIO-IOIO-IOIO-IOIO", "ABCD-EFGH-JKLM"} {
		if LooksLikeActivationCode(text) {
			t.Errorf("LooksLikeActivationCode(%q) = true for text that cannot be a code", text)
		}
	}
}
