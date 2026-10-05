package model

// ClientIpBan keeps one client off one network until ExpiresAt. Network is the
// unit the IP limit counts: an IPv4 address or an IPv6 /64.
type ClientIpBan struct {
	Id        int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Email     string `json:"email" gorm:"not null;uniqueIndex:idx_client_ip_ban,priority:1" example:"alice"`
	Network   string `json:"network" gorm:"not null;uniqueIndex:idx_client_ip_ban,priority:2" example:"198.51.100.7"`
	BannedAt  int64  `json:"bannedAt" example:"1791172800"`
	ExpiresAt int64  `json:"expiresAt" gorm:"index" example:"1791174600"`
}
