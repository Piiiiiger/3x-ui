package model

// ProbeLink says which server of the Lite monitor is which host of this panel.
// NodeId 0 is the panel's own host, so NodeId cannot be the primary key.
type ProbeLink struct {
	Id        int    `json:"id" gorm:"primaryKey;autoIncrement"`
	NodeId    int    `json:"nodeId" gorm:"column:node_id;uniqueIndex;not null"`
	ServerId  string `json:"serverId" gorm:"column:server_id;uniqueIndex;not null"`
	UpdatedAt int64  `json:"updatedAt" gorm:"autoUpdateTime:milli"`
}

func (ProbeLink) TableName() string { return "probe_links" }
