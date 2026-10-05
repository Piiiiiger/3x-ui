package database

import (
	"encoding/json"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func tgIdInSettings(t *testing.T, inboundId int, email string) any {
	t.Helper()
	clients, _ := reloadInboundSettings(t, inboundId)["clients"].([]any)
	for _, raw := range clients {
		if c, ok := raw.(map[string]any); ok && c["email"] == email {
			return c["tgId"]
		}
	}
	t.Fatalf("%s is not in inbound %d", email, inboundId)
	return nil
}

// Regression: the account bot bound only the clients row, so every inbound kept
// tgId 0 and the next save of one wiped the binding; the repair copies it over.
func TestCopyClientTgIdsToInboundsRepairsARowOnlyBinding(t *testing.T) {
	initMtprotoMigrationDB(t)
	raw, _ := json.Marshal(map[string]any{"clients": []any{
		map[string]any{"email": "bound", "tgId": 0, "enable": true},
		map[string]any{"email": "never", "tgId": 0, "enable": true},
	}})
	in := &model.Inbound{UserId: 1, Remark: "in", Port: 47201, Protocol: model.VLESS, Settings: string(raw), Tag: "in-47201"}
	if err := db.Create(in).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	for _, rec := range []*model.ClientRecord{{Email: "bound", TgID: 5150, Enable: true}, {Email: "never", Enable: true}} {
		if err := db.Create(rec).Error; err != nil {
			t.Fatalf("create %s: %v", rec.Email, err)
		}
		if err := db.Create(&model.ClientInbound{ClientId: rec.Id, InboundId: in.Id}).Error; err != nil {
			t.Fatalf("link %s: %v", rec.Email, err)
		}
	}
	clearSeederHistory(t, "ClientTgIdToInbounds")

	if err := runSeeders(false); err != nil {
		t.Fatalf("runSeeders: %v", err)
	}

	if got := tgIdInSettings(t, in.Id, "bound"); got != float64(5150) {
		t.Errorf("bound tgId = %v, want 5150 from its record", got)
	}
	if got := tgIdInSettings(t, in.Id, "never"); got != float64(0) {
		t.Errorf("never tgId = %v, want it left at 0", got)
	}
	var runs int64
	db.Model(&model.HistoryOfSeeders{}).Where("seeder_name = ?", "ClientTgIdToInbounds").Count(&runs)
	if runs != 1 {
		t.Errorf("seeder history rows = %d, want 1 so the repair runs once", runs)
	}
}
