package database

import (
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// The nodes table as shipped before agent nodes existed.
const legacyNodesNoAgentDDL = "CREATE TABLE `nodes` (`id` integer PRIMARY KEY AUTOINCREMENT,`name` text,`remark` text,`scheme` text,`address` text,`port` integer,`base_path` text,`api_token` text,`enable` numeric DEFAULT true,`allow_private_address` numeric DEFAULT false,`tls_verify_mode` text DEFAULT \"verify\",`pinned_cert_sha256` text,`inbound_sync_mode` text DEFAULT \"all\",`inbound_tags` text,`outbound_tag` text,`guid` text,`status` text DEFAULT \"unknown\",`last_heartbeat` integer,`latency_ms` integer,`xray_version` text,`panel_version` text,`cpu_pct` real,`mem_pct` real,`uptime_secs` integer,`net_up` integer,`net_down` integer,`last_error` text,`xray_state` text,`xray_error` text,`config_dirty` numeric DEFAULT false,`config_dirty_at` integer,`inbounds_adopted_at` integer DEFAULT 0,`created_at` integer,`updated_at` integer)"

// Nodes added before agents existed are 3x-ui panels; reading them as anything
// else would stop the master from calling their API after the upgrade.
func TestMigrateNodeAgentColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "x-ui.db")
	legacy, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	if err := legacy.Exec(legacyNodesNoAgentDDL).Error; err != nil {
		t.Fatalf("create legacy nodes: %v", err)
	}
	if err := legacy.Exec(
		`INSERT INTO nodes (name, scheme, address, port, base_path, api_token, enable)
		 VALUES ('vmiss', 'http', '127.0.0.1', 22601, '/b0h1/', 'token', 1)`,
	).Error; err != nil {
		t.Fatalf("seed legacy node: %v", err)
	}
	sqlDB, err := legacy.DB()
	if err != nil {
		t.Fatalf("legacy db handle: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB over legacy schema: %v", err)
	}
	t.Cleanup(func() { _ = CloseDB() })

	var row model.Node
	if err := GetDB().Where("name = ?", "vmiss").First(&row).Error; err != nil {
		t.Fatalf("preexisting node lost: %v", err)
	}
	if row.Kind != model.NodeKindPanel || row.IsAgent() {
		t.Fatalf("preexisting node kind = %q, want %q", row.Kind, model.NodeKindPanel)
	}
	if row.AgentReportSeq != 0 || row.AgentInstance != "" || row.AgentSecretHash != "" {
		t.Fatalf("agent fields must start empty, got seq=%d instance=%q hash=%q", row.AgentReportSeq, row.AgentInstance, row.AgentSecretHash)
	}
}
