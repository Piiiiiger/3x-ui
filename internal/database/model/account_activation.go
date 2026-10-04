package model

// AccountActivation is a permanent one-account credential, independent of grants.
// Telegram settings survive renewals, renames and process restarts.
type AccountActivation struct {
	ClientId     int    `gorm:"primaryKey;autoIncrement:false" json:"-"`
	Code         string `gorm:"uniqueIndex;not null" json:"code"`
	DailyEnabled bool   `json:"dailyEnabled"`
	DailyTime    string `json:"dailyTime"`
}

// AccountNotification deduplicates successful deliveries by recipient and the
// actual expiry date, so renewing invalidates the previous reminder sequence.
type AccountNotification struct {
	Key    string `gorm:"primaryKey"`
	SentAt int64
}
