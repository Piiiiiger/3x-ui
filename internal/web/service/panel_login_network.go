package service

import (
	"time"

	"gorm.io/gorm/clause"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// PanelLoginNetwork is the key a login address is remembered by; IPv6 privacy
// suffixes rotate, so a /64 stands for one line. Unparsable input is its own key.
func PanelLoginNetwork(ip string) string {
	if network, _, ok := ipLimitNetwork(ip); ok {
		return network
	}
	return ip
}

// PanelLoginKnown says whether the admin has signed in from ip's network before.
func PanelLoginKnown(ip string) bool {
	var count int64
	err := database.GetDB().Model(&model.PanelLoginNetwork{}).
		Where("network = ?", PanelLoginNetwork(ip)).Count(&count).Error
	return err == nil && count > 0
}

// RecordPanelLogin remembers a successful sign-in and says whether its network is new.
func RecordPanelLogin(ip string, now time.Time) (bool, error) {
	db := database.GetDB()
	row := model.PanelLoginNetwork{Network: PanelLoginNetwork(ip), LastIP: ip, FirstSeen: now.Unix(), LastSeen: now.Unix()}
	created := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if created.Error != nil {
		return false, created.Error
	}
	if created.RowsAffected == 1 {
		return true, nil
	}
	return false, db.Model(&model.PanelLoginNetwork{}).Where("network = ?", row.Network).
		Updates(map[string]any{"last_ip": ip, "last_seen": now.Unix()}).Error
}
