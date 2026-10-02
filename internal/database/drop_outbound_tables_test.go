package database

import (
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// The outbound pages kept per-outbound traffic and outbound subscriptions in their
// own tables; once the pages are gone an upgrade drops both tables.
func TestInitDB_DropsOutboundTables(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "x-ui.db")
	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	legacy := []string{
		"CREATE TABLE IF NOT EXISTS outbound_traffics (id integer PRIMARY KEY AUTOINCREMENT, tag text UNIQUE, up integer DEFAULT 0, down integer DEFAULT 0, total integer DEFAULT 0)",
		"INSERT INTO outbound_traffics (tag, up, down, total) VALUES ('warp', 1, 2, 3)",
		"CREATE TABLE IF NOT EXISTS outbound_subscriptions (id integer PRIMARY KEY AUTOINCREMENT, name text, url text)",
		"INSERT INTO outbound_subscriptions (name, url) VALUES ('sub', 'https://sub.example.com/list')",
	}
	for _, stmt := range legacy {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("seed legacy table %q: %v", stmt, err)
		}
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}

	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB on the legacy database: %v", err)
	}
	t.Cleanup(func() { _ = CloseDB() })

	for _, table := range []string{"outbound_traffics", "outbound_subscriptions"} {
		if db.Migrator().HasTable(table) {
			t.Errorf("%s survived the upgrade", table)
		}
	}
}

// The WARP, NordVPN and PIA dialogs kept their account credentials as settings;
// nothing reads them once the dialogs are gone, so an upgrade deletes them.
func TestInitDB_DeletesOutboundIntegrationSettings(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "x-ui.db")
	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	retired := []string{"warp", "warpUpdateInterval", "warpLastUpdate", "nord", "pia"}
	for _, key := range append(retired, "webPort") {
		if err := db.Create(&model.Setting{Key: key, Value: "secret-" + key}).Error; err != nil {
			t.Fatalf("seed setting %s: %v", key, err)
		}
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}

	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB on the legacy database: %v", err)
	}
	t.Cleanup(func() { _ = CloseDB() })

	var left []string
	if err := db.Model(&model.Setting{}).Where("key IN ?", retired).Pluck("key", &left).Error; err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Errorf("retired settings survived the upgrade: %v", left)
	}
	var kept model.Setting
	if err := db.Where("key = ?", "webPort").First(&kept).Error; err != nil || kept.Value != "secret-webPort" {
		t.Errorf("webPort = %q (err %v), want the stored value kept", kept.Value, err)
	}
}
