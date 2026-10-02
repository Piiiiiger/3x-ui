package model

// ActivationCode lets one person register, or a signed-in person renew or change
// plan: it grants a plan with their quota, validity and monthly reset day.
type ActivationCode struct {
	Id     int    `json:"id" gorm:"primaryKey;autoIncrement" example:"1"`
	Code   string `json:"code" gorm:"uniqueIndex;not null" example:"7KQ3-X9MZ-2F4H-RT8W"`
	PlanId int    `json:"planId" gorm:"column:plan_id;not null;index" example:"2"`
	// TotalGB is the quota in bytes, 0 for none; Days 0 never expires; ResetDay 0 never resets.
	TotalGB  int64  `json:"totalGB" gorm:"column:total_gb;not null;default:0" example:"107374182400"`
	Days     int    `json:"days" gorm:"not null;default:0" example:"30"`
	ResetDay int    `json:"resetDay" gorm:"column:reset_day;not null;default:0" example:"1"`
	Note     string `json:"note" gorm:"not null;default:''" example:"for alice"`
	// UsedAt is 0 until the code is used; UsedBy is the email of whoever used it.
	UsedAt int64  `json:"usedAt" gorm:"column:used_at;not null;default:0" example:"0"`
	UsedBy string `json:"usedBy" gorm:"column:used_by;not null;default:''" example:""`
	// A failed renewal retries this same expiry until every service update succeeds.
	RedeemExpiry *int64 `json:"-"`
	CreatedAt    int64  `json:"createdAt" gorm:"autoCreateTime:milli" example:"1735689600000"`
}

func (ActivationCode) TableName() string { return "activation_codes" }
