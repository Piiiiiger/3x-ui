package service

import (
	"encoding/json"
	"strings"

	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// Enforce at render time, including subsequent inbound edits and restored rows.
// Sniffed Reality camouflage names must never replace the actual next hop.
func protectChainRelay(tx *gorm.DB, inbound *model.Inbound) error {
	var count int64
	if err := tx.Model(&model.ProxyChain{}).Where("relay_inbound_id = ? AND enabled = ?", inbound.Id, true).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 || strings.TrimSpace(inbound.Sniffing) == "" {
		return nil
	}
	var sniff map[string]any
	if err := json.Unmarshal([]byte(inbound.Sniffing), &sniff); err != nil {
		return err
	}
	if sniff == nil {
		return nil
	}
	sniff["routeOnly"] = true
	raw, err := json.Marshal(sniff)
	inbound.Sniffing = string(raw)
	return err
}
