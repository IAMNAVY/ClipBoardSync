package main

import (
	"log"
	"os"
	"strconv"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ============================================================================
// Configuration
// ============================================================================

var (
	dbPath            = getEnv("DB_PATH", "./data/clip.db")
	allowRegistration = true // Memory cache for global setting
	retentionCount    = 50   // Max entries per user (0 = unlimited)
	retentionDays     = 0    // Max age in days (0 = unlimited)
)

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ============================================================================
// Database initialization
// ============================================================================

var db *gorm.DB

func initDB() {
	// Ensure data directory exists
	if err := os.MkdirAll("./data", 0755); err != nil {
		log.Fatal("failed to create data directory:", err)
	}

	var err error
	db, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		log.Fatal("failed to connect database:", err)
	}

	// Connection pool — keep it lean for SQLite
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// Enable WAL mode for better concurrent read performance
	sqlDB.Exec("PRAGMA journal_mode=WAL")
	sqlDB.Exec("PRAGMA synchronous=NORMAL")

	// Auto migrate
	db.AutoMigrate(&User{}, &ClipEntry{}, &SystemSetting{})

	// Initialize SystemSettings
	initSetting("AllowRegistration", "true")
	initSetting("RetentionCount", "50")
	initSetting("RetentionDays", "0")

	// Load settings into memory
	allowRegistration = (loadSetting("AllowRegistration") == "true")
	if v, err := strconv.Atoi(loadSetting("RetentionCount")); err == nil {
		retentionCount = v
	}
	if v, err := strconv.Atoi(loadSetting("RetentionDays")); err == nil {
		retentionDays = v
	}

	// Initialize Admin
	var adminUser User
	if err := db.Where("username = ?", "admin").First(&adminUser).Error; err != nil {
		hash, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
		adminUser = User{
			Username:     "admin",
			PasswordHash: string(hash),
		}
		db.Create(&adminUser)
		log.Println("[init] Default admin account created: admin / admin123")
	}
}

// initSetting creates a SystemSetting if it doesn't exist yet.
func initSetting(key, defaultValue string) {
	var s SystemSetting
	if err := db.Where("key = ?", key).First(&s).Error; err != nil {
		db.Create(&SystemSetting{Key: key, Value: defaultValue})
	}
}

// loadSetting reads a SystemSetting value from DB.
func loadSetting(key string) string {
	var s SystemSetting
	if err := db.Where("key = ?", key).First(&s).Error; err != nil {
		return ""
	}
	return s.Value
}

// saveSetting writes a SystemSetting to DB.
func saveSetting(key, value string) {
	db.Model(&SystemSetting{}).Where("key = ?", key).Update("value", value)
}

// ============================================================================
// Retention enforcement
// ============================================================================

// enforceHistoryLimit cleans up old, unpinned entries based on retention policy.
// Pinned entries are never deleted by this function.
func enforceHistoryLimit(userID uint) {
	// Strategy 1: Enforce count limit (if configured)
	if retentionCount > 0 {
		var count int64
		db.Model(&ClipEntry{}).Where("user_id = ? AND is_pinned = ?", userID, false).Count(&count)
		if count > int64(retentionCount) {
			excess := count - int64(retentionCount)
			var oldest []ClipEntry
			db.Where("user_id = ? AND is_pinned = ?", userID, false).
				Order("created_at ASC").
				Limit(int(excess)).
				Find(&oldest)
			for _, entry := range oldest {
				db.Delete(&entry)
			}
		}
	}

	// Strategy 2: Enforce day limit (if configured)
	if retentionDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -retentionDays)
		db.Where("user_id = ? AND is_pinned = ? AND created_at < ?", userID, false, cutoff).
			Delete(&ClipEntry{})
	}
}
