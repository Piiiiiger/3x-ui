package model

// ClientDailyTraffic is one client's traffic on one day of the panel's time zone.
type ClientDailyTraffic struct {
	Email string `json:"email" gorm:"primaryKey;column:email" example:"alice"`
	Day   int    `json:"day" gorm:"primaryKey;column:day;index" example:"20261001"` // YYYYMMDD
	Up    int64  `json:"up" gorm:"column:up;not null" example:"1048576"`
	Down  int64  `json:"down" gorm:"column:down;not null" example:"4194304"`
}

func (ClientDailyTraffic) TableName() string { return "client_daily_traffics" }

// ClientTrafficMark is the counter the daily job last saw per client, so each
// run adds only what was used since the run before.
type ClientTrafficMark struct {
	Email string `json:"email" gorm:"primaryKey;column:email" example:"alice"`
	Up    int64  `json:"up" gorm:"column:up;not null" example:"1048576"`
	Down  int64  `json:"down" gorm:"column:down;not null" example:"4194304"`
}

func (ClientTrafficMark) TableName() string { return "client_traffic_marks" }
