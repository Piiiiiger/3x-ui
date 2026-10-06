package database

import "github.com/mhsanaei/3x-ui/v3/internal/database/model"

// aiRuleSets route AI services in the templates. Each follows a VPSDance/ai-proxy-rules
// list, which a person reviews before any of it reaches subscriptions.
var aiRuleSets = []model.RuleSet{
	{Name: "ai", UpstreamURL: "https://raw.githubusercontent.com/VPSDance/ai-proxy-rules/main/rules/clash/global.yaml"},
	{Name: "ai-cn", UpstreamURL: "https://raw.githubusercontent.com/VPSDance/ai-proxy-rules/main/rules/clash/cn.yaml"},
}

func ensureAIRuleSets() error {
	for _, set := range aiRuleSets {
		if err := db.Where("name = ?", set.Name).FirstOrCreate(&set).Error; err != nil {
			return err
		}
	}
	return nil
}
