package database

import (
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestInitDBMigratesSingleTargetChainsWithoutChangingPlanSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chains.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	db := GetDB()
	if db.Migrator().HasIndex(&model.ProxyChain{}, "idx_proxy_chains_route") {
		if err := db.Migrator().DropIndex(&model.ProxyChain{}, "idx_proxy_chains_route"); err != nil {
			t.Fatal(err)
		}
	}
	if !db.Migrator().HasIndex(&model.ProxyChain{}, "idx_proxy_chains_target") {
		if err := db.Exec("CREATE UNIQUE INDEX idx_proxy_chains_target ON proxy_chains(target_inbound_id)").Error; err != nil {
			t.Fatal(err)
		}
	}
	chain := model.ProxyChain{Id: 7, TargetInboundId: 10, RelayInboundId: 20, Enabled: true}
	if err := db.Create(&chain).Error; err != nil {
		t.Fatal(err)
	}
	plan := model.Plan{Name: "Existing", NodeKeys: `["10:relay","20:direct"]`, ProxyGroups: `[{"name":"AI","inboundIds":[10],"nodeKeys":["10:relay"]}]`}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := CloseDB(); err != nil {
			t.Fatal(err)
		}
		if err := InitDB(path); err != nil {
			t.Fatal(err)
		}
		db = GetDB()
		if db.Migrator().HasIndex(&model.ProxyChain{}, "idx_proxy_chains_target") {
			t.Fatal("old target uniqueness survived")
		}
		if err := db.First(&plan, plan.Id).Error; err != nil {
			t.Fatal(err)
		}
		if plan.NodeKeys != `["10:relay:7","20:direct"]` {
			t.Fatalf("selection changed: %s", plan.NodeKeys)
		}
		if i == 0 {
			if err := db.Create(&model.ProxyChain{TargetInboundId: 10, RelayInboundId: 30, Enabled: true}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	var n int64
	db.Model(&model.ProxyChain{}).Count(&n)
	if n != 2 {
		t.Fatalf("chain count = %d", n)
	}
}
