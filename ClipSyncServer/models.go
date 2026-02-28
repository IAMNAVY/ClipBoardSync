package main

import "time"

// ============================================================================
// Models
// ============================================================================

type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"uniqueIndex;size:64;not null" json:"username"`
	PasswordHash string    `gorm:"size:256;not null" json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type ClipEntry struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     uint      `gorm:"index;not null" json:"user_id"`
	Content    string    `gorm:"type:text;not null" json:"content"`
	Category   string    `gorm:"size:16;default:'text'" json:"category"`
	IsPinned   bool      `gorm:"default:false" json:"is_pinned"`
	DeviceName string    `gorm:"size:128;default:''" json:"device_name"`
	CreatedAt  time.Time `json:"created_at"`
}

// CleanRule defines a user-configurable regex-based content filter rule.
// Action: "clean" = strip matched part; "ignore" = skip entire clipboard entry.
type CleanRule struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	UserID      uint   `gorm:"index;not null" json:"user_id"`
	Pattern     string `gorm:"size:512;not null" json:"pattern"`      // regex pattern
	Action      string `gorm:"size:16;not null" json:"action"`        // "clean" or "ignore"
	Description string `gorm:"size:256;default:''" json:"description"` // human-readable note
	Enabled     bool   `gorm:"default:true" json:"enabled"`
}

type SystemSetting struct {
	Key   string `gorm:"primaryKey;size:64" json:"key"`
	Value string `gorm:"type:text" json:"value"`
}

// ============================================================================
// Auth DTOs
// ============================================================================

type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=2,max=64"`
	Password string `json:"password" binding:"required,min=6,max=128"`
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type ClipRequest struct {
	Content    string `json:"content" binding:"required"`
	DeviceName string `json:"device_name"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6,max=128"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type AdminPasswordReset struct {
	NewPassword string `json:"new_password" binding:"required,min=6,max=128"`
}

type AdminConfigUpdate struct {
	AllowRegistration bool `json:"allow_registration"`
	RetentionCount    int  `json:"retention_count"`
	RetentionDays     int  `json:"retention_days"`
}

type CleanRuleRequest struct {
	Pattern     string `json:"pattern" binding:"required"`
	Action      string `json:"action" binding:"required,oneof=clean ignore"`
	Description string `json:"description"`
	Enabled     *bool  `json:"enabled"` // pointer to distinguish missing from false
}
