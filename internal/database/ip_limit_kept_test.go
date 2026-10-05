package database

import (
	"encoding/json"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// IP limits are enforced without fail2ban, so starting on a host without it
// must keep every configured limit (an upstream seeder used to zero them).
func TestStartupKeepsIpLimitsWithoutFail2ban(t *testing.T) {
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	if err := InitDB(config.GetDBPath()); err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	if err := db.Where("seeder_name = ?", "ResetIpLimitNoFail2ban").Delete(&model.HistoryOfSeeders{}).Error; err != nil {
		t.Fatal(err)
	}
	settings, _ := json.Marshal(map[string]any{"clients": []any{map[string]any{"email": "kept@example.test", "limitIp": 3}}})
	if err := db.Create(&model.Inbound{Remark: "kept", Tag: "kept", Settings: string(settings)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ClientRecord{Email: "kept@example.test", LimitIP: 3}).Error; err != nil {
		t.Fatal(err)
	}

	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(config.GetDBPath()); err != nil {
		t.Fatalf("restart: %v", err)
	}

	var record model.ClientRecord
	if err := db.Where("email = ?", "kept@example.test").First(&record).Error; err != nil {
		t.Fatal(err)
	}
	var inbound model.Inbound
	if err := db.Where("tag = ?", "kept").First(&inbound).Error; err != nil {
		t.Fatal(err)
	}
	var got struct {
		Clients []struct {
			LimitIp int `json:"limitIp"`
		} `json:"clients"`
	}
	if err := json.Unmarshal([]byte(inbound.Settings), &got); err != nil {
		t.Fatal(err)
	}
	if record.LimitIP != 3 || len(got.Clients) != 1 || got.Clients[0].LimitIp != 3 {
		t.Fatalf("after restart: record limit %d, inbound clients %+v; want 3 kept", record.LimitIP, got.Clients)
	}
}
