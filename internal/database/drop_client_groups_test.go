package database

import (
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// A database from before client groups were removed still carries their table and
// the clients column with its index; the upgrade drops them and keeps every client.
func TestInitDB_DropsClientGroupLeftovers(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "x-ui.db")
	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	seedClientGroupLeftovers(t, "integer PRIMARY KEY AUTOINCREMENT")
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}

	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB on the legacy database: %v", err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	assertClientGroupLeftoversDropped(t)
}

// PostgreSQL drops the column with a plain ALTER TABLE rather than SQLite's table
// rebuild, so the upgrade runs there too.
func TestInitDB_DropsClientGroupLeftovers_Postgres(t *testing.T) {
	usePostgresSchema(t)
	if err := InitDB(""); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	seedClientGroupLeftovers(t, "bigserial PRIMARY KEY")
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}

	if err := InitDB(""); err != nil {
		t.Fatalf("InitDB on the legacy database: %v", err)
	}
	assertClientGroupLeftoversDropped(t)
}

func seedClientGroupLeftovers(t *testing.T, idColumn string) {
	t.Helper()
	legacy := []string{
		"CREATE TABLE client_groups (id " + idColumn + ", name text NOT NULL UNIQUE, reset_up bigint DEFAULT 0, reset_down bigint DEFAULT 0, created_at bigint, updated_at bigint)",
		"INSERT INTO client_groups (name) VALUES ('staff')",
		"ALTER TABLE clients ADD COLUMN group_name text DEFAULT ''",
		"CREATE INDEX idx_client_record_group ON clients (group_name)",
		"INSERT INTO clients (email, sub_id, enable, group_name) VALUES ('alice', 'sub-alice', true, 'staff')",
	}
	for _, stmt := range legacy {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("seed legacy schema %q: %v", stmt, err)
		}
	}
}

func assertClientGroupLeftoversDropped(t *testing.T) {
	t.Helper()
	migrator := db.Migrator()
	if migrator.HasTable("client_groups") {
		t.Error("client_groups survived the upgrade")
	}
	if migrator.HasColumn(&model.ClientRecord{}, "group_name") {
		t.Error("clients.group_name survived the upgrade")
	}
	if migrator.HasIndex(&model.ClientRecord{}, "idx_client_record_group") {
		t.Error("idx_client_record_group survived the upgrade")
	}
	var kept model.ClientRecord
	if err := db.Where("email = ?", "alice").First(&kept).Error; err != nil || kept.SubID != "sub-alice" {
		t.Fatalf("client alice after the upgrade: %+v (err %v), want it kept with its sub id", kept, err)
	}
}
