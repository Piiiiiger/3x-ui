package service

import (
	"errors"
	"strings"
	"sync"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var accountActivationMu sync.Mutex

// EnsureAccountActivation preserves the original registration code. Accounts
// created by an administrator get a new permanent code on first access.
func EnsureAccountActivation(client *model.ClientRecord) (*model.AccountActivation, error) {
	accountActivationMu.Lock()
	defer accountActivationMu.Unlock()
	db := database.GetDB()
	var row model.AccountActivation
	err := db.First(&row, "client_id = ?", client.Id).Error
	if err == nil {
		return &row, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	var grant model.ActivationCode
	err = db.Where("used_by = ? AND used_at > 0 AND used_at >= ? AND code NOT IN (SELECT code FROM account_activations)", client.Email, client.CreatedAt-60000).Order("used_at ASC, id ASC").First(&grant).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	code := grant.Code
	if code == "" {
		code, err = newActivationCode()
		if err != nil {
			return nil, err
		}
	}
	row = model.AccountActivation{ClientId: client.Id, Code: code, DailyEnabled: true, DailyTime: "20:00"}
	if err = db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "client_id"}}, DoNothing: true}).Create(&row).Error; err != nil {
		return nil, err
	}
	err = db.First(&row, "client_id = ?", client.Id).Error
	return &row, err
}

// BindAccountActivation changes only the canonical account's notification
// identity; no Xray configuration or restart is needed for a Telegram binding.
func BindAccountActivation(code string, tgID int64) (*model.ClientRecord, error) {
	if tgID <= 0 {
		return nil, ErrActivationCode
	}
	accountActivationMu.Lock()
	defer accountActivationMu.Unlock()
	var client model.ClientRecord
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		var row model.AccountActivation
		if err := tx.Where("code = ?", strings.ToUpper(strings.TrimSpace(code))).First(&row).Error; err != nil {
			return ErrActivationCode
		}
		if err := tx.First(&client, row.ClientId).Error; err != nil {
			return ErrActivationCode
		}
		var others int64
		if err := tx.Model(&model.ClientRecord{}).Where("tg_id = ? AND id != ?", tgID, client.Id).Count(&others).Error; err != nil {
			return err
		}
		if others != 0 || (client.TgID != 0 && client.TgID != tgID) {
			return ErrActivationCode
		}
		result := tx.Model(&model.ClientRecord{}).Where("id = ? AND (tg_id = 0 OR tg_id = ?)", client.Id, tgID).Updates(map[string]any{"tg_id": tgID})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrActivationCode
		}
		client.TgID = tgID
		return nil
	})
	return &client, err
}
