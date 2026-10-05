package service

import (
	"errors"
	"strings"
	"sync"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
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

// accountBindMu makes a bind's check and write one step: of two Telegram
// accounts claiming one code at once, exactly one must win.
var accountBindMu sync.Mutex

// BindAccountActivation binds the account a code names to one Telegram account,
// reporting whether the cores need a restart as any client edit does.
func BindAccountActivation(code string, tgID int64) (*model.ClientRecord, bool, error) {
	if tgID <= 0 {
		return nil, false, ErrActivationCode
	}
	accountBindMu.Lock()
	defer accountBindMu.Unlock()
	db := database.GetDB()
	var row model.AccountActivation
	if err := db.Where("code = ?", strings.ToUpper(strings.TrimSpace(code))).First(&row).Error; err != nil {
		return nil, false, ErrActivationCode
	}
	var client model.ClientRecord
	if err := db.First(&client, row.ClientId).Error; err != nil {
		return nil, false, ErrActivationCode
	}
	var others int64
	if err := db.Model(&model.ClientRecord{}).Where("tg_id = ? AND id != ?", tgID, client.Id).Count(&others).Error; err != nil {
		return nil, false, err
	}
	if others != 0 || (client.TgID != 0 && client.TgID != tgID) {
		return nil, false, ErrActivationCode
	}
	if client.TgID == tgID {
		return &client, false, nil
	}
	needRestart, err := (&ClientService{}).setTelegramID(&InboundService{}, &client, tgID)
	if err != nil {
		return nil, needRestart, err
	}
	client.TgID = tgID
	return &client, needRestart, nil
}
