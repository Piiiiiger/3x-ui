package service

import (
	"errors"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

type CustomUserGroupSummary struct {
	model.CustomUserGroup
	Emails []string `json:"emails"`
}

func ListCustomUserGroups() ([]CustomUserGroupSummary, error) {
	db := database.GetDB()
	groups := []model.CustomUserGroup{}
	if err := db.Order("id").Find(&groups).Error; err != nil {
		return nil, err
	}
	result := make([]CustomUserGroupSummary, 0, len(groups))
	for _, g := range groups {
		emails := []string{}
		if err := db.Table("clients c").Joins("JOIN custom_user_group_members gm ON gm.client_id=c.id").Where("gm.group_id=?", g.Id).Order("c.email").Pluck("c.email", &emails).Error; err != nil {
			return nil, err
		}
		result = append(result, CustomUserGroupSummary{g, emails})
	}
	return result, nil
}

func SaveCustomUserGroup(id int, name string) error {
	name = strings.TrimSpace(name)
	if id < 0 || name == "" || utf8.RuneCountInString(name) > 64 {
		return errors.New("group name must contain 1–64 characters")
	}
	for _, r := range name {
		if r < ' ' {
			return errors.New("invalid group name")
		}
	}
	db := database.GetDB()
	if id == 0 {
		return db.Create(&model.CustomUserGroup{Name: name}).Error
	}
	result := db.Model(&model.CustomUserGroup{}).Where("id=?", id).Update("name", name)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("group not found")
	}
	return nil
}

func DeleteCustomUserGroup(id int) error {
	if id <= 0 {
		return errors.New("invalid group")
	}
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id=?", id).Delete(&model.CustomUserGroupMember{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.CustomUserGroup{}, id).Error
	})
}

func AssignCustomUserGroup(id int, emails []string) error {
	if id < 0 || len(emails) == 0 || len(emails) > 10000 {
		return errors.New("invalid group assignment")
	}
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		if id > 0 {
			var group model.CustomUserGroup
			if err := tx.First(&group, id).Error; err != nil {
				return err
			}
		}
		unique := map[string]bool{}
		for _, email := range emails {
			unique[email] = true
		}
		var clients []model.ClientRecord
		if err := tx.Select("id").Where("email IN ?", emails).Find(&clients).Error; err != nil {
			return err
		}
		if len(clients) != len(unique) {
			return errors.New("some users no longer exist; refresh and retry")
		}
		for _, c := range clients {
			if id == 0 {
				if err := tx.Where("client_id=?", c.Id).Delete(&model.CustomUserGroupMember{}).Error; err != nil {
					return err
				}
			} else {
				member := model.CustomUserGroupMember{ClientId: c.Id, GroupId: id}
				if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "client_id"}}, DoUpdates: clause.AssignmentColumns([]string{"group_id"})}).Create(&member).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}
