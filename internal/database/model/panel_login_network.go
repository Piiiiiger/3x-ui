package model

// PanelLoginNetwork is a network the panel admin has signed in from: an IPv4
// address or an IPv6 /64, the unit the IP limit counts too.
type PanelLoginNetwork struct {
	Id        int    `gorm:"primaryKey;autoIncrement"`
	Network   string `gorm:"not null;uniqueIndex"`
	LastIP    string
	FirstSeen int64
	LastSeen  int64
}
