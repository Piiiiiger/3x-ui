package model

// RuleSet is a list of classical rules templates use as RULE-SET,<name>,<target>.
// It follows an upstream list, and only what a review saved reaches subscriptions.
type RuleSet struct {
	Id          int    `json:"id" gorm:"primaryKey;autoIncrement" example:"1"`
	Name        string `json:"name" gorm:"uniqueIndex;not null" example:"ai"`
	Rules       string `json:"rules" gorm:"type:text;not null;default:''" example:"DOMAIN-SUFFIX,example.com"`
	UpstreamURL string `json:"upstreamUrl" gorm:"column:upstream_url;not null;default:''" example:"https://example.com/rules.yaml"`
	// Reviewed is the upstream list as the last review saw it, Latest as last fetched.
	Reviewed   string `json:"reviewed" gorm:"type:text;not null;default:''"`
	ReviewedAt int64  `json:"reviewedAt" gorm:"column:reviewed_at;not null;default:0" example:"1735689600000"`
	Latest     string `json:"latest" gorm:"type:text;not null;default:''"`
	FetchedAt  int64  `json:"fetchedAt" gorm:"column:fetched_at;not null;default:0" example:"1735689600000"`
	// PendingSince is the first fetch whose rules differed from the reviewed ones.
	PendingSince  int64  `json:"pendingSince" gorm:"column:pending_since;not null;default:0" example:"0"`
	FetchFailures int    `json:"fetchFailures" gorm:"column:fetch_failures;not null;default:0" example:"0"`
	FailingSince  int64  `json:"failingSince" gorm:"column:failing_since;not null;default:0" example:"0"`
	FetchError    string `json:"fetchError" gorm:"column:fetch_error;not null;default:''" example:""`
	UpdatedAt     int64  `json:"updatedAt" gorm:"autoUpdateTime:milli" example:"1735689600000"`
}

func (RuleSet) TableName() string { return "rule_sets" }

// RuleSetVersion is a set as one save left it: its rules and the upstream list they
// were reviewed against, so a restore brings back both.
type RuleSetVersion struct {
	Id        int    `json:"id" gorm:"primaryKey;autoIncrement" example:"1"`
	RuleSetId int    `json:"ruleSetId" gorm:"column:rule_set_id;index;not null" example:"1"`
	Rules     string `json:"rules" gorm:"type:text;not null;default:''"`
	Reviewed  string `json:"reviewed" gorm:"type:text;not null;default:''"`
	Note      string `json:"note" gorm:"not null;default:''" example:"Factory added"`
	SavedAt   int64  `json:"savedAt" gorm:"column:saved_at;autoCreateTime:milli" example:"1735689600000"`
}

func (RuleSetVersion) TableName() string { return "rule_set_versions" }
