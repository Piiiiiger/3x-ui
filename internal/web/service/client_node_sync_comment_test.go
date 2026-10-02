package service

import (
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/runtime"
)

func TestSetRemoteTraffic_PreservesPanelLocalComment(t *testing.T) {
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	dbtest.InitDB(t, filepath.Join(dbDir, "x-ui.db"))

	db := database.GetDB()

	const nodeID = 1
	const email = "node-user@example.com"
	const uid = "ce8d33df-3a64-4f10-8f9b-91c3a8e0c003"
	const wantComment = "renewed manually"

	id := nodeID
	central := &model.Inbound{
		UserId:   1,
		NodeID:   &id,
		Tag:      "n1-vless",
		Enable:   true,
		Port:     20001,
		Protocol: model.VLESS,
		Settings: `{"clients":[{"email":"` + email + `","id":"` + uid + `","enable":true,"comment":"` + wantComment + `"}]}`,
	}
	if err := db.Create(central).Error; err != nil {
		t.Fatalf("create node inbound: %v", err)
	}

	if err := db.Create(&model.ClientRecord{
		Email:   email,
		UUID:    uid,
		Enable:  true,
		Comment: wantComment,
	}).Error; err != nil {
		t.Fatalf("create client record: %v", err)
	}

	snap := &runtime.TrafficSnapshot{
		Inbounds: []*model.Inbound{
			{
				Tag:      "n1-vless",
				Enable:   true,
				Port:     20001,
				Protocol: model.VLESS,
				Settings: `{"clients":[{"email":"` + email + `","id":"` + uid + `","enable":true}]}`,
			},
		},
	}

	svc := InboundService{}
	if _, err := svc.setRemoteTrafficLocked(nodeID, snap, false, false); err != nil {
		t.Fatalf("setRemoteTrafficLocked: %v", err)
	}

	var row model.ClientRecord
	if err := db.Where("email = ?", email).First(&row).Error; err != nil {
		t.Fatalf("lookup client row after sync: %v", err)
	}
	if row.Comment != wantComment {
		t.Errorf("comment was wiped by node snapshot sync: got %q, want %q", row.Comment, wantComment)
	}
}
