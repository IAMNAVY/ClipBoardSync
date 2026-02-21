package main

import (
	"log"
	"os"
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

	// Initialize SystemSetting
	var regSetting SystemSetting
	if err := db.Where("key = ?", "AllowRegistration").First(&regSetting).Error; err != nil {
		regSetting = SystemSetting{Key: "AllowRegistration", Value: "true"}
		db.Create(&regSetting)
	}
	allowRegistration = (regSetting.Value == "true")

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

// ============================================================================
// FIFO enforcement — keep at most 50 entries per user
// ============================================================================

func enforceHistoryLimit(userID uint) {
	var count int64
	db.Model(&ClipEntry{}).Where("user_id = ?", userID).Count(&count)
	if count > 50 {
		// Find the oldest entry that should be removed
		excess := count - 50
		var oldest []ClipEntry
		db.Where("user_id = ?", userID).
			Order("created_at ASC").
			Limit(int(excess)).
			Find(&oldest)
		for _, entry := range oldest {
			db.Delete(&entry)
		}
	}
}
