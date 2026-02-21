package main

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// ============================================================================
// Route Handlers
// ============================================================================

func handleRegister(c *gin.Context) {
	if !allowRegistration {
		c.JSON(http.StatusForbidden, gin.H{"error": "registration is currently disabled"})
		return
	}

	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if username already exists
	var existing User
	if db.Where("username = ?", req.Username).First(&existing).Error == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "username already taken"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	user := User{
		Username:     req.Username,
		PasswordHash: string(hash),
	}
	if err := db.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	token, _ := generateToken(user.ID, user.Username)
	c.JSON(http.StatusCreated, gin.H{
		"message":  "registration successful",
		"token":    token,
		"user_id":  user.ID,
		"username": user.Username,
	})
}

func handleLogin(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user User
	if db.Where("username = ?", req.Username).First(&user).Error != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		return
	}

	token, _ := generateToken(user.ID, user.Username)
	c.JSON(http.StatusOK, gin.H{
		"message":  "login successful",
		"token":    token,
		"user_id":  user.ID,
		"username": user.Username,
	})
}

func handlePushClip(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	var req ClipRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	devName := req.DeviceName
	if devName == "" {
		devName = "Web 浏览器"
	}
	entry := ClipEntry{
		UserID:     userID,
		Content:    req.Content,
		DeviceName: devName,
	}
	if err := db.Create(&entry).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save clipboard"})
		return
	}

	// Enforce FIFO limit
	enforceHistoryLimit(userID)

	// Broadcast to all connected devices of this user
	hub.broadcast(userID, gin.H{
		"type":        "clip",
		"content":     entry.Content,
		"id":          entry.ID,
		"device_name": entry.DeviceName,
		"created_at":  entry.CreatedAt,
	}, nil)

	c.JSON(http.StatusOK, gin.H{
		"message": "clipboard pushed",
		"id":      entry.ID,
	})
}

func handleGetHistory(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	var entries []ClipEntry
	db.Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(50).
		Find(&entries)

	c.JSON(http.StatusOK, gin.H{
		"entries": entries,
		"count":   len(entries),
	})
}

func handleDeleteClip(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	id := c.Param("id")

	result := db.Where("id = ? AND user_id = ?", id, userID).Delete(&ClipEntry{})
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "entry not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func handleClearHistory(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	db.Where("user_id = ?", userID).Delete(&ClipEntry{})
	c.JSON(http.StatusOK, gin.H{"message": "history cleared"})
}

func handleGetDevices(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	val, ok := hub.clients.Load(userID)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"devices": []gin.H{}, "count": 0})
		return
	}
	uc := val.(*userClients)
	uc.mu.RLock()
	devices := make([]gin.H, 0, len(uc.list))
	for _, cl := range uc.list {
		devices = append(devices, gin.H{
			"id":           cl.id,
			"device_name":  cl.deviceName,
			"connected_at": cl.connectedAt,
		})
	}
	uc.mu.RUnlock()
	c.JSON(http.StatusOK, gin.H{"devices": devices, "count": len(devices)})
}

func handleRenameDevice(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	clientID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid device id"})
		return
	}

	var req struct {
		DeviceName string `json:"device_name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	client := hub.findClient(userID, clientID)
	if client == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}

	client.deviceName = req.DeviceName
	log.Printf("[ws] user %d renamed device %d to '%s'", userID, clientID, req.DeviceName)
	hub.broadcastDeviceList(userID)
	c.JSON(http.StatusOK, gin.H{"message": "device renamed"})
}

func handleRemoveDevice(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	clientID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid device id"})
		return
	}

	client := hub.findClient(userID, clientID)
	if client == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}

	// Send force_disconnect so the client knows not to auto-reconnect
	client.writeJSON(gin.H{"type": "force_disconnect", "reason": "removed by user"})
	// Close the connection — the defer in handleWebSocket will unregister and broadcast
	client.conn.Close()
	log.Printf("[ws] user %d removed device %d '%s'", userID, clientID, client.deviceName)
	c.JSON(http.StatusOK, gin.H{"message": "device removed"})
}

func handleChangePassword(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user User
	if err := db.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "incorrect old password"})
		return
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	user.PasswordHash = string(hash)
	db.Save(&user)

	c.JSON(http.StatusOK, gin.H{"message": "password updated successfully"})
}
