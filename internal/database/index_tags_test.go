package database

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// AutoMigrate must create the hot-path indexes for traffic lookups; gorm creates
// missing indexes on migrate, so this also covers existing DBs after an upgrade.
func TestAutoMigrateCreatesHotPathIndexes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.ClientRecord{}, &xray.ClientTraffic{}, &model.ClientGlobalTraffic{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	cases := []struct {
		model any
		index string
	}{
		{&xray.ClientTraffic{}, "idx_client_traffics_inbound"},
		{&xray.ClientTraffic{}, "idx_client_traffics_renew"},
		{&model.ClientGlobalTraffic{}, "idx_client_global_email"},
	}
	for _, c := range cases {
		if !db.Migrator().HasIndex(c.model, c.index) {
			t.Errorf("expected index %q to exist after AutoMigrate", c.index)
		}
	}
}
