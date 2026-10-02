package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestExternalSubscriptionHwidIsStableAndPersisted(t *testing.T) {
	setupSettingTestDB(t)

	first := ExternalSubscriptionHwid()
	if first == "" {
		t.Fatal("ExternalSubscriptionHwid returned empty")
	}
	if second := ExternalSubscriptionHwid(); second != first {
		t.Fatalf("hwid not stable: %q vs %q", first, second)
	}
	var row model.Setting
	if err := database.GetDB().Where("key = ?", "externalSubHwid").First(&row).Error; err != nil {
		t.Fatalf("hwid not persisted: %v", err)
	}
	if row.Value != first {
		t.Fatalf("persisted hwid %q != returned %q", row.Value, first)
	}
}
