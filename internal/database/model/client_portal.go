package model

// ClientPortalLogin is a client's password for the subscription server's portal.
// It is bound to the client row it was set for, so a reused id never inherits it.
type ClientPortalLogin struct {
	ClientId        int    `json:"clientId" gorm:"primaryKey;autoIncrement:false;column:client_id"`
	ClientCreatedAt int64  `json:"clientCreatedAt" gorm:"column:client_created_at"`
	PasswordHash    string `json:"-" gorm:"column:password_hash"`
	UpdatedAt       int64  `json:"updatedAt" gorm:"autoUpdateTime:milli"`
}

func (ClientPortalLogin) TableName() string { return "client_portal_logins" }
