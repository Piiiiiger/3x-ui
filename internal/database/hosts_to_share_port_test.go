package database

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// Entries (the hosts table) carried what links advertise instead of the inbound's
// own address and port. The upgrade moves each inbound's first enabled row onto the
// inbound itself, so links keep pointing at the same place, and drops the table.
func TestInitDB_MovesEntriesOntoTheirInbounds(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "x-ui.db")
	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	seeded := seedLegacyEntries(t, "id integer PRIMARY KEY AUTOINCREMENT, inbound_id integer, sort_order integer DEFAULT 0, is_disabled numeric DEFAULT 0, port integer DEFAULT 0")
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}

	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB on the legacy database: %v", err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	assertEntriesMoved(t, seeded)
}

// PostgreSQL kept is_disabled as a real boolean, which a comparison with 0 or 1 would
// reject, so the move runs against it too.
func TestInitDB_MovesEntriesOntoTheirInbounds_Postgres(t *testing.T) {
	usePostgresSchema(t)
	if err := InitDB(""); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	seeded := seedLegacyEntries(t, "id bigserial PRIMARY KEY, inbound_id bigint NOT NULL, sort_order bigint DEFAULT 0, is_disabled boolean DEFAULT false, port bigint DEFAULT 0")
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}

	if err := InitDB(""); err != nil {
		t.Fatalf("InitDB on the legacy database: %v", err)
	}
	assertEntriesMoved(t, seeded)
}

type legacyEntryInbounds struct{ nat, cdn, plain *model.Inbound }

// seedLegacyEntries takes the dialect's spelling of the hosts columns whose types
// differ between SQLite and PostgreSQL.
func seedLegacyEntries(t *testing.T, typedColumns string) legacyEntryInbounds {
	t.Helper()
	inbound := func(tag string, port int, strategy, addr string) *model.Inbound {
		ib := &model.Inbound{
			UserId: 1, Tag: tag, Remark: tag, Enable: true, Port: port, Protocol: model.VLESS,
			Settings: `{"clients":[]}`, StreamSettings: `{}`, ShareAddrStrategy: strategy, ShareAddr: addr,
		}
		if err := db.Create(ib).Error; err != nil {
			t.Fatalf("seed inbound %s: %v", tag, err)
		}
		return ib
	}
	seeded := legacyEntryInbounds{
		nat:   inbound("nat", 81, "custom", "203.0.113.53"),
		cdn:   inbound("cdn", 443, "node", ""),
		plain: inbound("plain", 8443, "node", ""),
	}

	legacy := []string{
		"DROP TABLE IF EXISTS hosts",
		"CREATE TABLE hosts (" + typedColumns + ", group_id text, remark text, address text, security text DEFAULT 'same')",
		fmt.Sprintf("INSERT INTO hosts (group_id, inbound_id, sort_order, remark, address, port) VALUES ('g1', %d, 1, 'nat', '203.0.113.53', 20443)", seeded.nat.Id),
		fmt.Sprintf("INSERT INTO hosts (group_id, inbound_id, sort_order, remark, address, port) VALUES ('g1', %d, 2, 'nat backup', '198.51.100.7', 30443)", seeded.nat.Id),
		fmt.Sprintf("INSERT INTO hosts (group_id, inbound_id, remark, address, port) VALUES ('g2', %d, 'cdn', 'cdn.example.com', 0)", seeded.cdn.Id),
		fmt.Sprintf("INSERT INTO hosts (group_id, inbound_id, remark, is_disabled, address, port) VALUES ('g3', %d, 'old', true, '198.51.100.9', 30000)", seeded.plain.Id),
	}
	for _, stmt := range legacy {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("seed legacy entries %q: %v", stmt, err)
		}
	}
	return seeded
}

func assertEntriesMoved(t *testing.T, seeded legacyEntryInbounds) {
	t.Helper()
	reload := func(ib *model.Inbound) model.Inbound {
		var got model.Inbound
		if err := db.First(&got, ib.Id).Error; err != nil {
			t.Fatalf("reload %s: %v", ib.Tag, err)
		}
		return got
	}
	if got := reload(seeded.nat); got.SharePort != 20443 || got.ShareAddrStrategy != "custom" || got.ShareAddr != "203.0.113.53" {
		t.Errorf("nat inbound: port %d, %s %q; want public port 20443 from its first entry and its address kept", got.SharePort, got.ShareAddrStrategy, got.ShareAddr)
	}
	if got := reload(seeded.cdn); got.SharePort != 0 || got.ShareAddrStrategy != "custom" || got.ShareAddr != "cdn.example.com" {
		t.Errorf("cdn inbound: port %d, %s %q; want its entry's address as a custom share address and its own port", got.SharePort, got.ShareAddrStrategy, got.ShareAddr)
	}
	if got := reload(seeded.plain); got.SharePort != 0 || got.ShareAddrStrategy != "node" || got.ShareAddr != "" {
		t.Errorf("plain inbound: port %d, %s %q; a disabled entry must not move", got.SharePort, got.ShareAddrStrategy, got.ShareAddr)
	}
	if db.Migrator().HasTable("hosts") {
		t.Error("the hosts table survived the upgrade")
	}
}
