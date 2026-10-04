package sub

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func subscriptionPlanNodes(subID string) (model.Plan, error) {
	var plan model.Plan
	err := database.GetDB().Table("plans AS p").
		Joins("JOIN clients AS c ON c.plan_id = p.id").
		Where("c.sub_id = ?", subID).Order("p.id").Select("p.*").Limit(1).Scan(&plan).Error
	return plan, err
}

func filterPlanNodeVariants(subID string, proxies []map[string]any) ([]map[string]any, error) {
	plan, err := subscriptionPlanNodes(subID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(plan.NodeKeys) == "" {
		return availableAutomaticChainVariants(proxies), nil
	}
	var keys []string
	if err := json.Unmarshal([]byte(plan.NodeKeys), &keys); err != nil {
		return nil, err
	}
	selected := make(map[string]bool)
	constrained := make(map[int]bool)
	for _, key := range keys {
		selected[key] = true
		id, _, err := model.ParsePlanNodeKey(key)
		if err != nil {
			return nil, err
		}
		constrained[id] = true
	}
	var planInboundIDs []int
	if err := database.GetDB().Model(&model.PlanInbound{}).Where("plan_id = ?", plan.Id).Pluck("inbound_id", &planInboundIDs).Error; err != nil {
		return nil, err
	}
	for _, id := range planInboundIDs {
		if !constrained[id] {
			selected[model.PlanNodeKey(id, false)] = true
		}
		constrained[id] = true
	}
	filtered := make([]map[string]any, 0, len(proxies))
	for _, proxy := range proxies {
		key, _ := proxy[clashPlanNodeKey].(string)
		id, _ := proxy[clashPlanInboundIDKey].(int)
		// Keep independently granted inbounds; a plan only chooses its variants.
		if !constrained[id] || selected[key] {
			filtered = append(filtered, proxy)
		}
	}
	// Unselected routes from independently granted nodes require a granted hop.
	available := make(map[int]bool)
	for _, proxy := range filtered {
		if _, chained := proxy[clashRelayInboundKey]; !chained {
			id, _ := proxy[clashPlanInboundIDKey].(int)
			available[id] = true
		}
	}
	out := filtered[:0]
	for _, proxy := range filtered {
		relayID, chained := proxy[clashRelayInboundKey].(int)
		key, _ := proxy[clashPlanNodeKey].(string)
		if !chained || available[relayID] || selected[key] {
			out = append(out, proxy)
		}
	}
	// Explicit plan order leads; hidden relay dependencies stay available at the end.
	ranks := make(map[string]int, len(keys))
	for i, key := range keys {
		if _, ok := ranks[key]; !ok {
			ranks[key] = i
		}
	}
	rank := func(p map[string]any) int {
		k, _ := p[clashPlanNodeKey].(string)
		if r, ok := ranks[k]; ok {
			return r
		}
		return len(keys)
	}
	sort.SliceStable(out, func(i, j int) bool { return rank(out[i]) < rank(out[j]) })
	return out, nil
}

func availableAutomaticChainVariants(proxies []map[string]any) []map[string]any {
	available := make(map[int]bool)
	for _, proxy := range proxies {
		if _, chained := proxy[clashRelayInboundKey]; !chained {
			id, _ := proxy[clashPlanInboundIDKey].(int)
			available[id] = true
		}
	}
	out := proxies[:0]
	for _, proxy := range proxies {
		relayID, chained := proxy[clashRelayInboundKey].(int)
		if !chained || available[relayID] {
			out = append(out, proxy)
		}
	}
	return out
}

// Resolve after name deduplication, including subscriptions without a plan.
// Stable node keys keep a renamed relay attached to the correct inbound.
func resolveChainProxyNames(proxies []map[string]any) error {
	byKey := make(map[string]string)
	for _, proxy := range proxies {
		key, _ := proxy[clashPlanNodeKey].(string)
		name, _ := proxy["name"].(string)
		if _, exists := byKey[key]; !exists {
			byKey[key] = name
		}
	}
	for _, proxy := range proxies {
		relayID, ok := proxy[clashRelayInboundKey].(int)
		if !ok {
			continue
		}
		name := byKey[model.PlanNodeKey(relayID, false)]
		if name == "" {
			return fmt.Errorf("中转节点不可用，无法生成 %v 的中转版本", proxy["name"])
		}
		proxy["dialer-proxy"] = name
	}
	return nil
}

func planGroupNodeKeys(subID string) (map[string][]string, error) {
	plan, err := subscriptionPlanNodes(subID)
	if err != nil {
		return nil, err
	}
	var groups []struct {
		Name     string   `json:"name"`
		NodeKeys []string `json:"nodeKeys"`
	}
	if strings.TrimSpace(plan.ProxyGroups) != "" {
		if err := json.Unmarshal([]byte(plan.ProxyGroups), &groups); err != nil {
			return nil, err
		}
	}
	out := make(map[string][]string)
	for _, group := range groups {
		if group.NodeKeys == nil {
			continue
		}
		out[group.Name] = group.NodeKeys
	}
	return out, nil
}

func clashProxyNamesForNodeKeys(value any, keys []string) []string {
	proxies, _ := value.([]map[string]any)
	byKey := make(map[string][]string)
	for _, proxy := range proxies {
		key, _ := proxy[clashPlanNodeKey].(string)
		name, _ := proxy["name"].(string)
		if name != "" {
			byKey[key] = append(byKey[key], name)
		}
	}
	names := []string{}
	seen := map[string]bool{}
	for _, key := range keys {
		for _, name := range byKey[key] {
			if !seen[name] {
				names = append(names, name)
				seen[name] = true
			}
		}
	}
	return names
}
