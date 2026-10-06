package model

// AiUsageDevice is a computer whose Claude Code and Codex usage Pigger Switch
// reports; DeviceKey is the random id the app keeps, so a renamed computer keeps its rows.
type AiUsageDevice struct {
	Id         int    `json:"id" gorm:"primaryKey;autoIncrement"`
	DeviceKey  string `json:"-" gorm:"column:device_key;uniqueIndex;size:64;not null"`
	Name       string `json:"name" gorm:"size:64;not null"`
	AppVersion string `json:"appVersion" gorm:"size:32;not null;default:''"`
	LastSyncAt int64  `json:"lastSyncAt" gorm:"not null;default:0"`
}

func (AiUsageDevice) TableName() string { return "ai_usage_devices" }

// AiUsageDaily is one device's usage of one model in one project on one day of
// the device's time zone; input tokens exclude cache reads, cost is micro-USD.
type AiUsageDaily struct {
	DeviceId         int    `gorm:"primaryKey;autoIncrement:false;column:device_id"`
	Day              int    `gorm:"primaryKey;autoIncrement:false;column:day;index"`
	App              string `gorm:"primaryKey;column:app;size:16"`
	Project          string `gorm:"primaryKey;column:project;size:400"`
	Model            string `gorm:"primaryKey;column:model;size:128"`
	Requests         int64  `gorm:"column:requests;not null"`
	InputTokens      int64  `gorm:"column:input_tokens;not null"`
	OutputTokens     int64  `gorm:"column:output_tokens;not null"`
	CacheReadTokens  int64  `gorm:"column:cache_read_tokens;not null"`
	CacheWriteTokens int64  `gorm:"column:cache_write_tokens;not null"`
	CostMicros       int64  `gorm:"column:cost_micros;not null"`
}

func (AiUsageDaily) TableName() string { return "ai_usage_dailies" }

// AiUsageSession is one Claude Code or Codex session as its device last reported
// it; Tokens excludes cache reads, which CacheReadTokens carries.
type AiUsageSession struct {
	DeviceId        int    `gorm:"primaryKey;autoIncrement:false;column:device_id"`
	App             string `gorm:"primaryKey;column:app;size:16"`
	SessionId       string `gorm:"primaryKey;column:session_id;size:128"`
	Title           string `gorm:"column:title;size:200;not null;default:''"`
	Project         string `gorm:"column:project;size:400;not null;default:''"`
	Model           string `gorm:"column:model;size:128;not null;default:''"`
	Requests        int64  `gorm:"column:requests;not null"`
	Tokens          int64  `gorm:"column:tokens;not null"`
	CacheReadTokens int64  `gorm:"column:cache_read_tokens;not null"`
	CostMicros      int64  `gorm:"column:cost_micros;not null"`
	FirstAt         int64  `gorm:"column:first_at;not null"`
	LastAt          int64  `gorm:"column:last_at;not null;index"`
}

func (AiUsageSession) TableName() string { return "ai_usage_sessions" }

// AiUsageQuota is the last plan-limit reading a device made for one tool; Tiers
// holds the windows as JSON, exactly as the device read them.
type AiUsageQuota struct {
	DeviceId    int    `gorm:"primaryKey;autoIncrement:false;column:device_id"`
	Tool        string `gorm:"primaryKey;column:tool;size:16"`
	Success     bool   `gorm:"column:success;not null"`
	PlanLabel   string `gorm:"column:plan_label;size:64;not null;default:''"`
	ActiveUntil string `gorm:"column:active_until;size:40;not null;default:''"`
	Tiers       string `gorm:"column:tiers;type:text;not null"`
	Error       string `gorm:"column:error;size:500;not null;default:''"`
	QueriedAt   int64  `gorm:"column:queried_at;not null"`
	// Estimates is the tool's window estimates as JSON; empty when the app sent none.
	Estimates string `gorm:"column:estimates;type:text;default:''"`
}

func (AiUsageQuota) TableName() string { return "ai_usage_quotas" }
