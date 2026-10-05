package database

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func migrateProxyChainRoutes() error {
	return db.Transaction(func(tx *gorm.DB) error {
		if !tx.Migrator().HasIndex(&model.ProxyChain{}, "idx_proxy_chains_route") {
			if err := tx.Migrator().CreateIndex(&model.ProxyChain{}, "idx_proxy_chains_route"); err != nil {
				return err
			}
		}
		var chains []model.ProxyChain
		if err := tx.Order("id").Find(&chains).Error; err != nil {
			return err
		}
		remap := map[string]string{}
		for _, chain := range chains {
			key := model.PlanNodeKey(chain.TargetInboundId, true)
			if _, exists := remap[key]; !exists {
				remap[key] = model.PlanChainKey(chain.TargetInboundId, chain.Id)
			}
		}
		if err := RemapPlanChainKeys(tx, remap); err != nil {
			return err
		}
		if tx.Migrator().HasIndex(&model.ProxyChain{}, "idx_proxy_chains_target") {
			return tx.Migrator().DropIndex(&model.ProxyChain{}, "idx_proxy_chains_target")
		}
		return nil
	})
}

// RemapPlanChainKeys preserves explicit route choices during migration and edits.
// Unknown group fields survive; grants are never added by this operation.
func RemapPlanChainKeys(tx *gorm.DB, remap map[string]string) error {
	var plans []model.Plan
	if err := tx.Find(&plans).Error; err != nil {
		return err
	}
	rewrite := func(keys []string) bool {
		changed := false
		for i, key := range keys {
			if next, ok := remap[key]; ok && next != key {
				keys[i] = next
				changed = true
			}
		}
		return changed
	}
	for _, plan := range plans {
		updates := map[string]any{}
		if strings.TrimSpace(plan.NodeKeys) != "" {
			var keys []string
			if err := json.Unmarshal([]byte(plan.NodeKeys), &keys); err != nil {
				return fmt.Errorf("plan %d node keys: %w", plan.Id, err)
			}
			if rewrite(keys) {
				raw, _ := json.Marshal(keys)
				updates["node_keys"] = string(raw)
			}
		}
		if strings.TrimSpace(plan.ProxyGroups) != "" {
			var groups []map[string]json.RawMessage
			if err := json.Unmarshal([]byte(plan.ProxyGroups), &groups); err != nil {
				return err
			}
			changed := false
			for _, group := range groups {
				raw, ok := group["nodeKeys"]
				if !ok {
					continue
				}
				var keys []string
				if err := json.Unmarshal(raw, &keys); err != nil {
					return err
				}
				if !rewrite(keys) {
					continue
				}
				changed = true
				group["nodeKeys"], _ = json.Marshal(keys)
				ids := []int{}
				for _, key := range keys {
					id, _, err := model.ParsePlanNodeKey(key)
					if err != nil {
						return err
					}
					if !slices.Contains(ids, id) {
						ids = append(ids, id)
					}
				}
				slices.Sort(ids)
				group["inboundIds"], _ = json.Marshal(ids)
			}
			if changed {
				raw, _ := json.Marshal(groups)
				updates["proxy_groups"] = string(raw)
			}
		}
		if len(updates) > 0 {
			if err := tx.Model(&model.Plan{}).Where("id = ?", plan.Id).Updates(updates).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
