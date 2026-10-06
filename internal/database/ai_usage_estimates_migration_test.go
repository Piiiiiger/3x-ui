package database

import (
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// A plan reading stored before estimates existed loads with no estimates and no NULL
// left behind, whether the column is missing or an older ALTER TABLE added it as NULL.
func TestAiUsageEstimatesColumnMigration(t *testing.T) {
	for _, nullable := range []bool{false, true} {
		name := "missing column"
		if nullable {
			name = "nullable column"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "x-ui.db")
			legacy, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			column := ""
			if nullable {
				column = ", estimates TEXT"
			}
			for _, ddl := range []string{
				"CREATE TABLE ai_usage_quotas (device_id INTEGER, tool TEXT, success NUMERIC, plan_label TEXT, active_until TEXT, tiers TEXT, error TEXT, queried_at INTEGER" + column + ", PRIMARY KEY (device_id, tool))",
				"INSERT INTO ai_usage_quotas (device_id, tool, success, plan_label, active_until, tiers, error, queried_at) VALUES (1, 'claude', 1, 'Pro', '', '[]', '', 5)",
			} {
				if err := legacy.Exec(ddl).Error; err != nil {
					t.Fatal(err)
				}
			}
			handle, err := legacy.DB()
			if err != nil {
				t.Fatal(err)
			}
			if err := handle.Close(); err != nil {
				t.Fatal(err)
			}
			if err := InitDB(path); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = CloseDB() })
			var nulls int64
			if err := GetDB().Table("ai_usage_quotas").Where("estimates IS NULL").Count(&nulls).Error; err != nil || nulls != 0 {
				t.Fatalf("NULL estimates/error = %d/%v", nulls, err)
			}
			var row model.AiUsageQuota
			if err := GetDB().Where("tool = ?", "claude").First(&row).Error; err != nil || row.PlanLabel != "Pro" || row.Estimates != "" {
				t.Fatalf("reading/error = %+v/%v", row, err)
			}
		})
	}
}
