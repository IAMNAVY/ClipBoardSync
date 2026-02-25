package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ============================================================================
// Configuration
// ============================================================================

var listenAddr = getEnv("LISTEN_ADDR", ":8080")

// ============================================================================
// Main
// ============================================================================

func main() {
	// Use release mode for lower memory footprint
	// gin.SetMode(gin.DebugMode)
	gin.SetMode(gin.ReleaseMode)

	initDB()

	r := gin.New()
	r.Use(gin.Recovery())

	// Serve embedded frontend
	r.GET("/", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(indexHTML))
	})

	// Auth routes
	r.POST("/api/register", handleRegister)
	r.POST("/api/login", handleLogin)
	r.POST("/api/refresh", handleRefreshToken)
	r.GET("/api/config", handleGetConfig) // Public config

	// WebSocket
	r.GET("/ws", handleWebSocket)

	// Protected routes
	api := r.Group("/api", authMiddleware())
	{
		api.POST("/clipboard", handlePushClip)
		api.GET("/clipboard", handleGetHistory)
		api.DELETE("/clipboard/:id", handleDeleteClip)
		api.DELETE("/clipboard/all", handleClearHistory) // New clear history route
		api.GET("/devices", handleGetDevices)
		api.PUT("/devices/:id/rename", handleRenameDevice)
		api.DELETE("/devices/:id", handleRemoveDevice)
		api.PUT("/user/password", handleChangePassword) // New modify password route
		api.PUT("/clipboard/:id/pin", handleTogglePin)  // Toggle pin/favorite
	}

	// Admin routes
	admin := r.Group("/api/admin", authMiddleware(), adminMiddleware())
	{
		admin.GET("/users", handleAdminGetUsers)
		admin.DELETE("/users/:id", handleAdminDeleteUser)
		admin.PUT("/users/:id/password", handleAdminResetPassword)
		admin.PUT("/config", handleUpdateConfig)
	}

	log.Printf("🚀 ClipSync server starting on %s", listenAddr)
	log.Printf("📂 Database: %s", dbPath)
	log.Printf("🌐 Open http://localhost%s in your browser", listenAddr)

	if err := r.Run(listenAddr); err != nil {
		log.Fatal("server error:", err)
	}
}
