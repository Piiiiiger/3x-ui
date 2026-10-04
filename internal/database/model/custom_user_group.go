package model

// CustomUserGroup is an administrative label independent of a subscription plan.
type CustomUserGroup struct {
	Id   int    `json:"id" gorm:"primaryKey"`
	Name string `json:"name" gorm:"uniqueIndex;not null"`
}

// One custom group per client; client IDs keep membership stable across renames.
type CustomUserGroupMember struct {
	ClientId int `gorm:"primaryKey;autoIncrement:false"`
	GroupId  int `gorm:"index;not null"`
}
