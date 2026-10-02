package model

// RuleTemplate is a set of Clash rules plans share: rule lines, a YAML document whose
// groups list __PROXY_NODES__, an HTTPS URL, or a variant's changes to its base.
type RuleTemplate struct {
	Id        int    `json:"id" gorm:"primaryKey;autoIncrement" example:"1"`
	Name      string `json:"name" gorm:"uniqueIndex;not null" example:"alpha_v3"`
	Content   string `json:"content" gorm:"type:text;not null;default:''" example:"DOMAIN-SUFFIX,example.com,DIRECT"`
	IsDefault bool   `json:"isDefault" gorm:"column:is_default;default:false" example:"false"`
	// BaseId is the template a variant changes; 0 for a full template.
	BaseId    int   `json:"baseId" gorm:"column:base_id;not null;default:0;index" example:"0"`
	CreatedAt int64 `json:"createdAt" gorm:"autoCreateTime:milli" example:"1735689600000"`
	UpdatedAt int64 `json:"updatedAt" gorm:"autoUpdateTime:milli" example:"1735689600000"`
}

func (RuleTemplate) TableName() string { return "rule_templates" }

// RuleTemplateVersion is a template's content as one save left it; the newest few
// of each template are kept, so a save can be undone.
type RuleTemplateVersion struct {
	Id         int    `json:"id" gorm:"primaryKey;autoIncrement" example:"1"`
	TemplateId int    `json:"templateId" gorm:"column:template_id;index;not null" example:"1"`
	Content    string `json:"content" gorm:"type:text;not null;default:''" example:"DOMAIN-SUFFIX,example.com,DIRECT"`
	Size       int    `json:"size" gorm:"column:size;not null;default:0" example:"389305"`
	SavedAt    int64  `json:"savedAt" gorm:"column:saved_at;autoCreateTime:milli" example:"1735689600000"`
}

func (RuleTemplateVersion) TableName() string { return "rule_template_versions" }
