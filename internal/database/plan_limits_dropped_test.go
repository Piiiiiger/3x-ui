package database

import (
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

var planLimitColumns = []string{"total_gb", "duration_days", "traffic_reset", "traffic_reset_day"}

// Plans stopped holding a quota, validity and reset schedule, which each user keeps
// on their own; a database from before still has those columns until the upgrade.
func TestInitDB_DropsPlanLimitColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "x-ui.db")
	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	seedPlanLimitColumns(t)
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}

	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB on the legacy database: %v", err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	assertPlanLimitColumnsDropped(t)
}

func TestInitDB_DropsPlanLimitColumns_Postgres(t *testing.T) {
	usePostgresSchema(t)
	if err := InitDB(""); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	seedPlanLimitColumns(t)
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(""); err != nil {
		t.Fatalf("InitDB on the legacy database: %v", err)
	}
	assertPlanLimitColumnsDropped(t)
}

func seedPlanLimitColumns(t *testing.T) {
	t.Helper()
	db := GetDB()
	for _, column := range []string{
		"total_gb bigint DEFAULT 0", "duration_days integer DEFAULT 0",
		"traffic_reset text DEFAULT 'never'", "traffic_reset_day integer DEFAULT 1",
	} {
		if err := db.Exec("ALTER TABLE plans ADD COLUMN " + column).Error; err != nil {
			t.Fatalf("add legacy column %s: %v", column, err)
		}
	}
	if err := db.Exec("INSERT INTO plans (name, limit_ip, template_id, sort_index, total_gb, duration_days, " +
		"traffic_reset, traffic_reset_day) VALUES ('Monthly', 2, 0, 0, 1073741824, 30, 'monthly', 5)").Error; err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	var plan model.Plan
	if err := db.Where("name = ?", "Monthly").First(&plan).Error; err != nil {
		t.Fatalf("read seeded plan: %v", err)
	}
	if err := db.Create(&model.ClientRecord{
		Email: "member@example.test", PlanId: plan.Id, TotalGB: 5 << 30,
		ExpiryTime: 2000000000000, TrafficReset: "monthly", TrafficResetDay: 22,
	}).Error; err != nil {
		t.Fatalf("seed member limits: %v", err)
	}
}

func assertPlanLimitColumnsDropped(t *testing.T) {
	t.Helper()
	for _, column := range planLimitColumns {
		if GetDB().Migrator().HasColumn(&model.Plan{}, column) {
			t.Errorf("plans still has column %s", column)
		}
	}
	var plan model.Plan
	if err := GetDB().Where("name = ?", "Monthly").First(&plan).Error; err != nil {
		t.Fatalf("the plan did not survive the upgrade: %v", err)
	}
	if plan.LimitIP != 2 {
		t.Fatalf("plan IP limit = %d, want 2", plan.LimitIP)
	}
	var member model.ClientRecord
	if err := GetDB().Where("email = ?", "member@example.test").First(&member).Error; err != nil {
		t.Fatalf("read member after upgrade: %v", err)
	}
	if member.PlanId != plan.Id || member.TotalGB != 5<<30 || member.ExpiryTime != 2000000000000 || member.TrafficReset != "monthly" || member.TrafficResetDay != 22 {
		t.Fatalf("member limits changed during upgrade: plan %d, quota %d, expiry %d, reset %s/%d", member.PlanId, member.TotalGB, member.ExpiryTime, member.TrafficReset, member.TrafficResetDay)
	}
}
