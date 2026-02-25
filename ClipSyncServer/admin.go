package main

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// ============================================================================
// Admin & Config API
// ============================================================================

func handleGetConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"allow_registration": allowRegistration})
}

func adminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		username := c.MustGet("username").(string)
		if username != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "admin access required"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func handleUpdateConfig(c *gin.Context) {
	var req AdminConfigUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	allowRegistration = req.AllowRegistration
	valStr := "false"
	if allowRegistration {
		valStr = "true"
	}
	db.Model(&SystemSetting{}).Where("key = ?", "AllowRegistration").Update("value", valStr)

	c.JSON(http.StatusOK, gin.H{"message": "config updated", "allow_registration": allowRegistration})
}

func handleAdminGetUsers(c *gin.Context) {
	type userResponse struct {
		ID        uint      `json:"id"`
		Username  string    `json:"username"`
		CreatedAt time.Time `json:"created_at"`
	}
	var users []User
	db.Order("id ASC").Find(&users)
	
	var res []userResponse
	for _, u := range users {
		res = append(res, userResponse{ID: u.ID, Username: u.Username, CreatedAt: u.CreatedAt})
	}
	c.JSON(http.StatusOK, gin.H{"users": res})
}

func handleAdminDeleteUser(c *gin.Context) {
	targetID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	if targetID == 1 { // Protect admin
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot delete default admin"})
		return
	}

	// Delete user and their clips
	db.Where("user_id = ?", targetID).Delete(&ClipEntry{})
	db.Delete(&User{}, targetID)

	// Disconnect all sessions belonging to this user
	val, exists := hub.clients.Load(uint(targetID))
	if exists {
		uc := val.(*userClients)
		uc.mu.RLock()
		snapshot := make([]*Client, len(uc.list))
		copy(snapshot, uc.list)
		uc.mu.RUnlock()
		for _, client := range snapshot {
			client.writeJSON(gin.H{"type": "force_disconnect", "reason": "account deleted by admin"})
			client.conn.Close()
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "user deleted"})
}

func handleAdminResetPassword(c *gin.Context) {
	targetID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	var req AdminPasswordReset
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user User
	if err := db.First(&user, targetID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	user.PasswordHash = string(hash)
	db.Save(&user)

	c.JSON(http.StatusOK, gin.H{"message": "user password reset successfully"})
}
