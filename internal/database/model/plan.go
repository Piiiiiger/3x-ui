package model

// Plan is a reusable set of limits (quota, validity, reset schedule, IP limit and the
// inbounds it grants) that the panel stamps onto every client assigned to it.
type Plan struct {
	Id              int    `json:"id" gorm:"primaryKey;autoIncrement" example:"1"`
	Name            string `json:"name" gorm:"uniqueIndex;not null" example:"Monthly 100G"`
	TotalGB         int64  `json:"totalGB" gorm:"column:total_gb;default:0" example:"107374182400"` // bytes, 0 = unlimited
	DurationDays    int    `json:"durationDays" gorm:"column:duration_days;default:0" example:"30"` // 0 = never expires
	TrafficReset    string `json:"trafficReset" gorm:"column:traffic_reset;default:never" example:"monthly"`
	TrafficResetDay int    `json:"trafficResetDay" gorm:"column:traffic_reset_day;default:1" example:"1"`
	LimitIP         int    `json:"limitIp" gorm:"column:limit_ip;default:0" example:"0"`
	Remark          string `json:"remark" example:"Hong Kong and Singapore"`
	// ClashRules replaces the global Clash rules for members: inline rules/YAML or an
	// HTTPS URL. Empty inherits the global rules.
	ClashRules string `json:"clashRules" gorm:"column:clash_rules;default:''" example:"DOMAIN-SUFFIX,example.com,DIRECT"`
	SortIndex  int    `json:"sortIndex" gorm:"column:sort_index;default:0" example:"0"`
	CreatedAt  int64  `json:"createdAt" gorm:"autoCreateTime:milli" example:"1735689600000"`
	UpdatedAt  int64  `json:"updatedAt" gorm:"autoUpdateTime:milli" example:"1735689600000"`
}

// PlanInbound is the plan-to-inbound join: the servers a plan grants its members.
type PlanInbound struct {
	PlanId    int `json:"planId" gorm:"primaryKey;column:plan_id"`
	InboundId int `json:"inboundId" gorm:"primaryKey;column:inbound_id;index"`
}
