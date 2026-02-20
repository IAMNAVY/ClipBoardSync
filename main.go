package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ============================================================================
// Configuration
// ============================================================================

var (
	jwtSecret         = []byte(getEnv("JWT_SECRET", "clipboard-sync-secret-key-change-me"))
	listenAddr        = getEnv("LISTEN_ADDR", ":8080")
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
	DeviceName string    `gorm:"size:128;default:''" json:"device_name"`
	CreatedAt  time.Time `json:"created_at"`
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

type AdminPasswordReset struct {
	NewPassword string `json:"new_password" binding:"required,min=6,max=128"`
}

type AdminConfigUpdate struct {
	AllowRegistration bool `json:"allow_registration"`
}

// ============================================================================
// JWT helpers
// ============================================================================

func generateToken(userID uint, username string) (string, error) {
	claims := jwt.MapClaims{
		"user_id":  userID,
		"username": username,
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
		"iat":      time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

func parseToken(tokenStr string) (uint, string, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return 0, "", fmt.Errorf("invalid token")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return 0, "", fmt.Errorf("invalid claims")
	}
	userIDFloat, ok := claims["user_id"].(float64)
	if !ok {
		return 0, "", fmt.Errorf("invalid user_id")
	}
	username, _ := claims["username"].(string)
	return uint(userIDFloat), username, nil
}

// ============================================================================
// JWT Middleware
// ============================================================================

func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing or invalid authorization header"})
			c.Abort()
			return
		}
		tokenStr := strings.TrimPrefix(auth, "Bearer ")
		userID, username, err := parseToken(tokenStr)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}
		c.Set("user_id", userID)
		c.Set("username", username)
		c.Next()
	}
}

// ============================================================================
// WebSocket Hub — lightweight, memory-safe
// ============================================================================

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

var clientIDCounter uint64

type Client struct {
	id          uint64
	conn        *websocket.Conn
	userID      uint
	deviceName  string
	connectedAt time.Time
	mu          sync.Mutex
}

func (c *Client) writeJSON(v interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return c.conn.WriteJSON(v)
}

// Hub stores all active connections keyed by user ID
type Hub struct {
	// map[uint][]*Client
	clients sync.Map
}

var hub = &Hub{}

func (h *Hub) register(client *Client) {
	val, _ := h.clients.LoadOrStore(client.userID, &[]*Client{})
	list := val.(*[]*Client)
	*list = append(*list, client)
}

func (h *Hub) unregister(client *Client) {
	val, ok := h.clients.Load(client.userID)
	if !ok {
		return
	}
	list := val.(*[]*Client)
	newList := make([]*Client, 0, len(*list))
	for _, c := range *list {
		if c != client {
			newList = append(newList, c)
		}
	}
	if len(newList) == 0 {
		h.clients.Delete(client.userID)
	} else {
		*list = newList
	}
}

// broadcast sends to all connections of a user EXCEPT the sender (if provided)
func (h *Hub) broadcast(userID uint, msg interface{}, sender *Client) {
	val, ok := h.clients.Load(userID)
	if !ok {
		return
	}
	list := val.(*[]*Client)
	for _, c := range *list {
		if c == sender {
			continue
		}
		if err := c.writeJSON(msg); err != nil {
			log.Printf("[ws] write error for user %d: %v", userID, err)
		}
	}
}

// broadcastDeviceList sends the current device list to ALL connections of a user
func (h *Hub) broadcastDeviceList(userID uint) {
	val, ok := h.clients.Load(userID)
	if !ok {
		return
	}
	list := val.(*[]*Client)
	devices := make([]gin.H, 0, len(*list))
	for _, c := range *list {
		devices = append(devices, gin.H{
			"id":           c.id,
			"device_name":  c.deviceName,
			"connected_at": c.connectedAt,
		})
	}
	msg := gin.H{
		"type":    "devices_update",
		"devices": devices,
	}
	for _, c := range *list {
		if err := c.writeJSON(msg); err != nil {
			log.Printf("[ws] write error for user %d: %v", userID, err)
		}
	}
}

// findClient finds a client by its unique ID for a given user
func (h *Hub) findClient(userID uint, clientID uint64) *Client {
	val, ok := h.clients.Load(userID)
	if !ok {
		return nil
	}
	list := val.(*[]*Client)
	for _, c := range *list {
		if c.id == clientID {
			return c
		}
	}
	return nil
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
		devName = "Web \u6d4f\u89c8\u5668"
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

func handleGetDevices(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	val, ok := hub.clients.Load(userID)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"devices": []gin.H{}, "count": 0})
		return
	}
	list := val.(*[]*Client)
	devices := make([]gin.H, 0, len(*list))
	for _, cl := range *list {
		devices = append(devices, gin.H{
			"id":           cl.id,
			"device_name":  cl.deviceName,
			"connected_at": cl.connectedAt,
		})
	}
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

// ============================================================================
// WebSocket handler
// ============================================================================

func handleWebSocket(c *gin.Context) {
	tokenStr := c.Query("token")
	if tokenStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
		return
	}

	deviceName := c.Query("device_name")
	if deviceName == "" {
		deviceName = "Web 浏览器"
	}

	userID, _, err := parseToken(tokenStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[ws] upgrade error: %v", err)
		return
	}

	client := &Client{id: atomic.AddUint64(&clientIDCounter, 1), conn: conn, userID: userID, deviceName: deviceName, connectedAt: time.Now()}
	hub.register(client)
	log.Printf("[ws] user %d device '%s' connected (total: %d)", userID, deviceName, countUserClients(userID))
	hub.broadcastDeviceList(userID)

	// Send welcome message with client ID so the client can identify itself
	client.writeJSON(gin.H{"type": "welcome", "client_id": client.id})

	defer func() {
		hub.unregister(client)
		conn.Close()
		log.Printf("[ws] user %d device '%s' disconnected (total: %d)", userID, deviceName, countUserClients(userID))
		hub.broadcastDeviceList(userID)
	}()

	// Configure ping/pong
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Start ping ticker
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			client.mu.Lock()
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			err := conn.WriteMessage(websocket.PingMessage, nil)
			client.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()

	// Read loop — handle incoming clipboard pushes from WebSocket
	for {
		var msg map[string]interface{}
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("[ws] read error for user %d: %v", userID, err)
			}
			break
		}

		// Handle clipboard push via WebSocket
		if msgType, ok := msg["type"].(string); ok && msgType == "clip" {
			content, ok := msg["content"].(string)
			if !ok || content == "" {
				continue
			}

			entry := ClipEntry{
				UserID:     userID,
				Content:    content,
				DeviceName: client.deviceName,
			}
			if err := db.Create(&entry).Error; err != nil {
				log.Printf("[ws] db error for user %d: %v", userID, err)
				continue
			}
			enforceHistoryLimit(userID)

			// Broadcast to OTHER devices of this user
			hub.broadcast(userID, gin.H{
				"type":        "clip",
				"content":     entry.Content,
				"id":          entry.ID,
				"device_name": entry.DeviceName,
				"created_at":  entry.CreatedAt,
			}, client)
		}
	}
}

func countUserClients(userID uint) int {
	val, ok := hub.clients.Load(userID)
	if !ok {
		return 0
	}
	return len(*val.(*[]*Client))
}

// ============================================================================
// Embedded Frontend HTML
// ============================================================================

const indexHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>ClipSync</title>
<style>
  :root {
    /* Light Mode */
    --bg: #F9FAFB;
    --surface: #FFFFFF;
    --surface-hover: #F3F4F6;
    --border: #E5E7EB;
    /* Primary deep blue */
    --primary: #1E3A8A;
    --primary-hover: #1E40AF;
    --accent: #3B82F6;
    --text: #111827;
    --text-dim: #6B7280;
    --danger: #EF4444;
    --success: #10B981;
    --radius: 8px;
    --shadow: 0 1px 3px rgba(0,0,0,0.1), 0 1px 2px rgba(0,0,0,0.06);
    --shadow-hover: 0 4px 6px -1px rgba(0,0,0,0.1), 0 2px 4px -1px rgba(0,0,0,0.06);
  }

  @media (prefers-color-scheme: dark) {
    :root {
      /* Dark Mode */
      --bg: #111827;
      --surface: #1F2937;
      --surface-hover: #374151;
      --border: #374151;
      --primary: #3B82F6;
      --primary-hover: #60A5FA;
      --accent: #60A5FA;
      --text: #F9FAFB;
      --text-dim: #9CA3AF;
      --shadow: 0 4px 6px rgba(0,0,0,0.3);
      --shadow-hover: 0 10px 15px -1px rgba(0,0,0,0.4);
    }
  }

  * { margin:0; padding:0; box-sizing:border-box; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
    background: var(--bg);
    color: var(--text);
    height: 100vh;
    overflow: hidden;
    display: flex;
    flex-direction: column;
    letter-spacing: 0.01em;
  }

  /* Auth Layout (Centered) */
  .auth-wrapper {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100%;
    overflow-y: auto;
    padding: 20px;
  }
  .auth-card {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 32px;
    width: 100%;
    max-width: 400px;
    box-shadow: var(--shadow);
  }
  .auth-card h1 {
    font-size: 1.5em;
    font-weight: 700;
    text-align: center;
    margin-bottom: 8px;
    color: var(--primary);
  }
  .auth-card p.subtitle {
    text-align: center;
    color: var(--text-dim);
    margin-bottom: 24px;
    font-size: 0.9em;
  }
  .auth-tabs {
    display: flex;
    margin-bottom: 24px;
    border-bottom: 1px solid var(--border);
  }
  .auth-tab {
    flex: 1;
    text-align: center;
    padding: 12px;
    cursor: pointer;
    font-weight: 600;
    color: var(--text-dim);
    border-bottom: 2px solid transparent;
    transition: all 0.2s;
  }
  .auth-tab.active {
    color: var(--primary);
    border-bottom: 2px solid var(--primary);
  }

  /* Main Layout (Sidebar + Content) */
  .app-container {
    display: flex;
    flex: 1;
    overflow: hidden;
  }
  .sidebar {
    width: 260px;
    background: var(--surface);
    border-right: 1px solid var(--border);
    padding: 24px 16px;
    display: flex;
    flex-direction: column;
    flex-shrink: 0;
    overflow-y: auto;
  }
  .main-content {
    flex: 1;
    padding: 32px 40px;
    width: 100%;
    display: flex;
    justify-content: center;
    overflow-y: auto;
  }
  .content-wrapper {
    width: 100%;
    max-width: 900px;
    display: flex;
    flex-direction: column;
    gap: 24px;
  }

  @media (max-width: 768px) {
    .app-container { flex-direction: column; }
    .sidebar { width: 100%; border-right: none; border-bottom: 1px solid var(--border); padding: 16px; flex-direction: column; justify-content: flex-start; align-items: stretch; gap: 16px; }
    .main-content { padding: 20px 16px; }
    .sidebar-logo { margin-bottom: 0 !important; }
    .sidebar-user { margin-top: 0 !important; }
    .sidebar-nav { display: flex; flex-direction: row; gap: 8px; justify-content: center; }
  }

  .sidebar-logo {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 40px;
  }
  .sidebar-logo-icon {
    width: 32px; height: 32px;
    background: var(--primary);
    border-radius: 8px;
    display: flex; align-items: center; justify-content: center;
    color: white; font-weight: bold; font-size: 1.2em;
  }
  .sidebar-logo-text { font-size: 1.25em; font-weight: 700; color: var(--text); }
  
  .sidebar-nav {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .nav-item {
    display: flex; align-items: center; gap: 12px;
    padding: 10px 14px;
    border-radius: var(--radius);
    color: var(--text);
    font-weight: 500;
    cursor: pointer;
    transition: background 0.2s;
  }
  .nav-item.active { background: var(--surface-hover); color: var(--primary); }
  .nav-item:hover:not(.active) { background: var(--surface-hover); }

  .sidebar-user {
    margin-top: auto;
    padding: 16px 14px;
    background: var(--surface-hover);
    border-radius: var(--radius);
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  .user-info { display: flex; align-items: center; gap: 10px; }
  .user-avatar {
    width: 32px; height: 32px;
    border-radius: 50%; background: var(--border);
    display: flex; align-items: center; justify-content: center; font-size: 14px;
    font-weight: bold; color: var(--text);
  }
  .user-name { font-size: 0.9em; font-weight: 600; }

  /* Common Components */
  input[type="text"], input[type="password"], textarea {
    width: 100%;
    padding: 12px 14px;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    color: var(--text);
    font-size: 0.95em;
    outline: none;
    transition: border-color 0.2s, box-shadow 0.2s;
    margin-bottom: 16px;
    font-family: inherit;
  }
  input:focus, textarea:focus { 
    border-color: var(--primary); 
    box-shadow: 0 0 0 3px rgba(30, 58, 138, 0.1); 
  }
  @media (prefers-color-scheme: dark) {
    input:focus, textarea:focus { box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.2); }
  }
  textarea { resize: vertical; min-height: 100px; }
  
  .btn {
    display: inline-flex; align-items: center; justify-content: center;
    padding: 10px 20px;
    border: none; border-radius: var(--radius);
    font-size: 0.9em; font-weight: 600;
    cursor: pointer; transition: all 0.2s; gap: 8px;
    width: 100%;
  }
  .btn-primary { background: var(--primary); color: #fff; }
  .btn-primary:hover { background: var(--primary-hover); }
  .btn-outline { background: transparent; color: var(--text); border: 1px solid var(--border); }
  .btn-outline:hover { background: var(--surface-hover); }
  .btn-small { padding: 6px 12px; font-size: 0.85em; width: auto; }
  .btn-icon { 
    padding: 6px; width: auto; border: 1px solid transparent; border-radius: 6px; 
    background: transparent; color: var(--text-dim); transition: all 0.2s; 
    cursor: pointer; display: flex; align-items: center; justify-content: center;
  }
  .btn-icon:hover { background: var(--surface-hover); color: var(--text); border-color: var(--border); }
  .btn-icon.danger:hover { color: var(--danger); background: rgba(239, 68, 68, 0.1); border-color: transparent; }

  .card {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    box-shadow: var(--shadow);
    overflow: hidden;
  }
  .card-body { padding: 20px; }

  .status-badge {
    display: inline-flex; align-items: center; gap: 6px;
    padding: 4px 10px; border-radius: 99px;
    font-size: 0.75em; font-weight: 600;
    background: var(--surface-hover); color: var(--text-dim);
  }
  .status-badge.online { color: var(--success); background: rgba(16, 185, 129, 0.1); }
  .status-dot { width: 6px; height: 6px; border-radius: 50%; background: currentColor; }
  .status-badge.online .status-dot { animation: pulse 2s infinite; }
  @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: .5; } }

  /* Clip List */
  #clip-list { display: flex; flex-direction: column; }
  .clip-item {
    padding: 20px;
    border-bottom: 1px solid var(--border);
    transition: background 0.2s;
    display: flex; flex-direction: column; gap: 12px;
  }
  .clip-item:last-child { border-bottom: none; }
  .clip-item:hover { background: var(--surface-hover); }
  
  .clip-content {
    font-size: 0.95em; line-height: 1.6;
    white-space: pre-wrap; word-break: break-all;
    max-height: 200px; overflow-y: auto;
    color: var(--text);
  }
  .clip-content::-webkit-scrollbar { width: 4px; }
  .clip-content::-webkit-scrollbar-track { background: transparent; }
  .clip-content::-webkit-scrollbar-thumb { background: var(--border); border-radius: 4px; }
  
  /* Input Custom Scrollbar styling so it only shows when expanded */
  #clip-input::-webkit-scrollbar { width: 6px; }
  #clip-input::-webkit-scrollbar-track { background: transparent; }
  #clip-input::-webkit-scrollbar-thumb { background: var(--border); border-radius: 4px; }
  
  .clip-meta {
    display: flex; justify-content: space-between; align-items: center;
    font-size: 0.8em; color: var(--text-dim);
  }
  .clip-actions { display: flex; gap: 8px; opacity: 0; transition: opacity 0.2s; }
  .clip-item:hover .clip-actions { opacity: 1; }
  @media (max-width: 768px) { .clip-actions { opacity: 1; } }

  .toast {
    position: fixed; bottom: 24px; right: 24px;
    padding: 12px 20px; border-radius: var(--radius);
    font-size: 0.9em; font-weight: 500;
    z-index: 9999; animation: slideUp 0.3s cubic-bezier(0.16, 1, 0.3, 1);
    color: #fff; box-shadow: 0 10px 15px -3px rgba(0,0,0,0.2);
  }
  .toast.success { background: var(--text); color: var(--bg); }
  .toast.error { background: var(--danger); }
  @keyframes slideUp { from { transform: translateY(100%); opacity: 0; } to { transform: translateY(0); opacity: 1; } }
  .hidden { display: none !important; }

  /* Modals */
  .modal-overlay {
    position: fixed; top: 0; left: 0; right: 0; bottom: 0;
    background: rgba(0,0,0,0.5); z-index: 1000;
    display: flex; align-items: center; justify-content: center;
    backdrop-filter: blur(2px);
  }
  .modal-card {
    background: var(--surface); padding: 24px; border-radius: var(--radius);
    width: 100%; max-width: 400px; box-shadow: var(--shadow-hover);
  }
  .modal-card h3 { margin-bottom: 16px; font-size: 1.25em; }
  .modal-actions { display: flex; gap: 12px; justify-content: flex-end; margin-top: 24px; }

  /* Icons SVG */
  .icon { width: 1.2em; height: 1.2em; fill: currentColor; }

  /* Device List */
  .device-item {
    display: flex; align-items: center; gap: 12px;
    padding: 16px 20px;
    border-bottom: 1px solid var(--border);
    transition: background 0.2s;
  }
  .device-item:last-child { border-bottom: none; }
  .device-item:hover { background: var(--surface-hover); }
  .device-icon {
    width: 40px; height: 40px;
    border-radius: 10px;
    background: rgba(59, 130, 246, 0.1);
    display: flex; align-items: center; justify-content: center;
    color: var(--accent); flex-shrink: 0;
  }
  .device-info { flex: 1; min-width: 0; }
  .device-name { font-weight: 600; font-size: 0.95em; margin-bottom: 2px; }
  .device-time { font-size: 0.8em; color: var(--text-dim); }
  .device-status {
    width: 8px; height: 8px; border-radius: 50%;
    background: var(--success); flex-shrink: 0;
    animation: pulse 2s infinite;
  }
</style>
</head>
<body>

  <!-- Auth View -->
  <div id="auth-section" class="auth-wrapper">
    <div class="auth-card">
      <div style="display: flex; justify-content: center; margin-bottom: 16px;">
        <div class="sidebar-logo-icon" style="width: 48px; height: 48px; font-size: 1.5em; border-radius: 12px;">CS</div>
      </div>
      <h1>ClipSync</h1>
      <p class="subtitle">跨设备剪贴板实时同步</p>
      
      <div class="auth-tabs">
        <div class="auth-tab active" id="tab-login" onclick="switchAuthTab('login')">登录</div>
        <div class="auth-tab" id="tab-register" onclick="switchAuthTab('register')">注册</div>
      </div>

      <div id="view-login">
        <input type="text" id="login-user" placeholder="用户名" onkeypress="handleEnter(event, 'login')">
        <input type="password" id="login-pass" placeholder="密码" onkeypress="handleEnter(event, 'login')">
        <button class="btn btn-primary" onclick="login()">登录</button>
      </div>

      <div id="view-register" class="hidden">
        <input type="text" id="reg-user" placeholder="设置用户名 (至少 2 位)" onkeypress="handleEnter(event, 'register')">
        <input type="password" id="reg-pass" placeholder="设置密码 (至少 6 位)" onkeypress="handleEnter(event, 'register')">
        <button class="btn btn-primary" onclick="register()">创建账号</button>
      </div>
    </div>
  </div>

  <!-- Main View -->
  <div id="main-section" class="app-container hidden">
    <!-- Sidebar -->
    <aside class="sidebar">
      <div class="sidebar-logo">
        <div class="sidebar-logo-icon">CS</div>
        <div class="sidebar-logo-text">ClipSync</div>
      </div>
      
      <div class="sidebar-nav">
        <!-- User items -->
        <div class="nav-item active user-nav" id="nav-clipboard" onclick="switchView('clipboard')">
          <svg class="icon" viewBox="0 0 24 24"><path d="M19 3h-4.18C14.4 1.84 13.3 1 12 1c-1.3 0-2.4.84-2.82 2H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2zm-7 0c.55 0 1 .45 1 1s-.45 1-1 1-1-.45-1-1 .45-1 1-1zm2 14H7v-2h7v2zm3-4H7v-2h10v2zm0-4H7V7h10v2z"/></svg>
          剪贴板
        </div>
        <div class="nav-item user-nav" id="nav-devices" onclick="switchView('devices')">
          <svg class="icon" viewBox="0 0 24 24"><path d="M4 6h18V4H4c-1.1 0-2 .9-2 2v11H0v3h14v-3H4V6zm19 2h-6c-.55 0-1 .45-1 1v10c0 .55.45 1 1 1h6c.55 0 1-.45 1-1V9c0-.55-.45-1-1-1zm-1 9h-4v-7h4v7z"/></svg>
          在线设备 <span id="device-count-badge" style="margin-left:auto; background:var(--accent); color:#fff; border-radius:99px; padding:1px 8px; font-size:0.75em; font-weight:700;">0</span>
        </div>

        <!-- Admin items -->
        <div class="nav-item admin-nav hidden" id="nav-users" onclick="switchView('users')">
          <svg class="icon" viewBox="0 0 24 24"><path d="M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z"/></svg>
          用户管理
        </div>
        <div class="nav-item admin-nav hidden" id="nav-settings" onclick="switchView('settings')">
          <svg class="icon" viewBox="0 0 24 24"><path d="M19.14,12.94c0.04-0.3,0.06-0.61,0.06-0.94c0-0.32-0.02-0.64-0.06-0.94l2.03-1.58c0.18-0.14,0.23-0.41,0.12-0.61 l-1.92-3.32c-0.12-0.22-0.37-0.29-0.59-0.22l-2.39,0.96c-0.5-0.38-1.03-0.7-1.62-0.94L14.4,2.81c-0.04-0.24-0.24-0.41-0.48-0.41 h-3.84c-0.24,0-0.43,0.17-0.47,0.41L9.25,5.35C8.66,5.59,8.12,5.92,7.63,6.29L5.24,5.33c-0.22-0.08-0.47,0-0.59,0.22L2.73,8.87 C2.62,9.08,2.66,9.34,2.86,9.48l2.03,1.58C4.84,11.36,4.8,11.69,4.8,12s0.02,0.64,0.06,0.94l-2.03,1.58 c-0.18,0.14-0.23,0.41-0.12,0.61l1.92,3.32c0.12,0.22,0.37,0.29,0.59,0.22l2.39-0.96c0.5,0.38,1.03,0.7,1.62,0.94l0.36,2.54 c0.05,0.24,0.24,0.41,0.48,0.41h3.84c0.24,0,0.44-0.17,0.47-0.41l0.36-2.54c0.59-0.24,1.13-0.56,1.62-0.94l2.39,0.96 c0.22,0.08,0.47,0,0.59-0.22l1.92-3.32c0.12-0.22,0.07-0.49-0.12-0.61L19.14,12.94z M12,15.6c-1.98,0-3.6-1.62-3.6-3.6 s1.62-3.6,3.6-3.6s3.6,1.62,3.6,3.6S13.98,15.6,12,15.6z"/></svg>
          系统设置
        </div>
      </div>

      <div class="sidebar-user">
        <div class="user-info">
          <div class="user-avatar" id="avatar-letter">U</div>
          <div class="user-name" id="display-user"></div>
        </div>
        <div style="display:flex; gap:8px;">
          <button class="btn-icon" onclick="openPwdModal()" title="修改密码">
            <svg class="icon" viewBox="0 0 24 24"><path d="M12.65 10A5.99 5.99 0 0 0 7 6c-3.31 0-6 2.69-6 6s2.69 6 6 6a5.99 5.99 0 0 0 5.65-4h2.35v4h4v-4h2v-4h-8.35zM7 14c-1.1 0-2-.9-2-2s.9-2 2-2 2 .9 2 2-.9 2-2 2z"/></svg>
          </button>
          <button class="btn-icon danger" onclick="logout()" title="退出登录">
            <svg class="icon" viewBox="0 0 24 24"><path d="M17 7l-1.41 1.41L18.17 11H8v2h10.17l-2.58 2.58L17 17l5-5zM4 5h8V3H4c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h8v-2H4V5z"/></svg>
          </button>
        </div>
      </div>
    </aside>

    <!-- Content -->
    <main class="main-content">
      <div class="content-wrapper">
        <!-- Clipboard View -->
        <div id="view-clipboard" class="view-panel">
          <div style="display: flex; justify-content: space-between; align-items: center;">
            <div style="display:flex; align-items:center; gap:16px;">
              <h2 style="font-size: 1.5em; font-weight: 700;">我的剪贴板</h2>
              <button class="btn-icon danger" style="padding:4px;" title="清空所有记录" onclick="clearAllHistory()">
                <svg class="icon" viewBox="0 0 24 24"><path d="M6 19c0 1.1.9 2 2 2h8c1.1 0 2-.9 2-2V7H6v12zM19 4h-3.5l-1-1h-5l-1 1H5v2h14V4z"/></svg>
              </button>
            </div>
            <div class="status-badge" id="ws-badge">
              <div class="status-dot" id="ws-dot"></div>
              <span id="ws-status">未连接</span>
            </div>
          </div>

          <div class="card" style="margin-bottom: 24px;">
            <div class="card-body">
              <textarea id="clip-input" placeholder="输入你想同步的文本... (支持多行)" style="border:none; padding:0; margin-bottom: 16px; background: transparent; height: 80px; min-height: 80px; resize: none; box-shadow:none; overflow-y: hidden;"></textarea>
              <div style="display: flex; justify-content: flex-end; gap: 8px;">
                <button id="expand-btn" class="btn-icon hidden" title="展开" onclick="toggleExpandInput()">
                  <svg class="icon" viewBox="0 0 24 24"><path d="M16.59 8.59L12 13.17 7.41 8.59 6 10l6 6 6-6z"/></svg>
                </button>
                <button class="btn btn-primary btn-small" onclick="pushClip()">推送 (Ctrl+Enter)</button>
              </div>
            </div>
          </div>

          <div class="card">
            <div id="clip-list">
              <!-- Inject clips here -->
            </div>
          </div>
        </div>

        <!-- Devices View -->
        <div id="view-devices" class="view-panel hidden">
          <div style="display: flex; justify-content: space-between; align-items: center;">
            <h2 style="font-size: 1.5em; font-weight: 700;">在线设备</h2>
            <div class="status-badge online">
              <div class="status-dot"></div>
              <span id="device-total">0 台设备</span>
            </div>
          </div>
          <div class="card">
            <div id="device-list">
              <div style="padding: 40px; text-align: center; color: var(--text-dim);">暂无在线设备</div>
            </div>
          </div>
        </div>
        
        <!-- Admin Users View -->
        <div id="view-users" class="view-panel hidden">
          <h2 style="font-size: 1.5em; font-weight: 700;">用户管理</h2>
          <div class="card" style="margin-top:24px;">
            <div id="admin-user-list">
              <!-- Admin users table injected here -->
            </div>
          </div>
        </div>

        <!-- Admin Settings View -->
        <div id="view-settings" class="view-panel hidden">
          <h2 style="font-size: 1.5em; font-weight: 700;">系统设置</h2>
          <div class="card" style="margin-top:24px;">
            <div class="card-body" style="display:flex; justify-content:space-between; align-items:center;">
              <div>
                <h3 style="font-size:1.1em; margin-bottom:4px;">开放注册</h3>
                <p style="color:var(--text-dim); font-size:0.9em;">允许新用户从 Web 界面或客户端注册账号</p>
              </div>
              <div>
                <button id="toggle-reg-btn" class="btn btn-outline" onclick="toggleRegistration()">加载中...</button>
              </div>
            </div>
          </div>
        </div>

      </div>
    </main>
  </div>

  <!-- Password Modal -->
  <div id="pwd-modal" class="modal-overlay hidden">
    <div class="modal-card">
      <h3 id="pwd-modal-title">修改密码</h3>
      <input type="password" id="pwd-old" placeholder="旧密码 (管理员重置他人密码时忽略)">
      <input type="password" id="pwd-new" placeholder="新密码 (至少 6 位)">
      <div class="modal-actions">
        <button class="btn btn-outline btn-small" onclick="closePwdModal()">取消</button>
        <button class="btn btn-primary btn-small" onclick="submitPwdModal()">确认修改</button>
      </div>
    </div>
  </div>

<script>
const API = window.location.origin;
let token = localStorage.getItem('token');
let username = localStorage.getItem('username');
let ws = null;
let intentionalClose = false;
let currentDevices = [];
let currentDeviceID = null;
let currentDeviceName = 'Web 浏览器';
let pwdContext = { type: 'self', targetId: null };

// Init
initApp();

async function initApp() {
  try {
    const res = await fetch(API + '/api/config');
    const cfg = await res.json();
    if (!cfg.allow_registration) {
      document.getElementById('tab-register').classList.add('hidden');
    }
  } catch (e) {
    console.error('Failed to load global config', e);
  }
  if (token) showMain();
}

function switchAuthTab(tab) {
  document.getElementById('tab-login').classList.remove('active');
  document.getElementById('tab-register').classList.remove('active');
  document.getElementById('view-login').classList.add('hidden');
  document.getElementById('view-register').classList.add('hidden');
  
  document.getElementById('tab-' + tab).classList.add('active');
  document.getElementById('view-' + tab).classList.remove('hidden');
}

function handleEnter(e, action) {
  if (e.key === 'Enter') {
    if (action === 'login') login();
    if (action === 'register') register();
  }
}

document.getElementById('clip-input').addEventListener('keydown', function(e) {
  if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
    pushClip();
  }
});

function showToast(msg, type='success') {
  const t = document.createElement('div');
  t.className = 'toast ' + type;
  t.textContent = msg;
  document.body.appendChild(t);
  setTimeout(() => {
    t.style.opacity = '0';
    t.style.transform = 'translateY(100%)';
    t.style.transition = 'all 0.3s';
    setTimeout(() => t.remove(), 300);
  }, 3000);
}

async function apiFetch(path, opts={}) {
  const headers = { 'Content-Type': 'application/json', ...opts.headers };
  if (token) headers['Authorization'] = 'Bearer ' + token;
  const res = await fetch(API + path, { ...opts, headers });
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || '请求失败');
  return data;
}

async function register() {
  const u = document.getElementById('reg-user').value.trim();
  const p = document.getElementById('reg-pass').value;
  if (!u || !p) return showToast('请输入用户名和密码', 'error');
  try {
    const data = await apiFetch('/api/register', {
      method: 'POST',
      body: JSON.stringify({ username: u, password: p })
    });
    setAuth(data);
    showToast('注册成功！');
    showMain();
  } catch (e) {
    showToast(e.message, 'error');
  }
}

async function login() {
  const u = document.getElementById('login-user').value.trim();
  const p = document.getElementById('login-pass').value;
  if (!u || !p) return showToast('请输入用户名和密码', 'error');
  try {
    const data = await apiFetch('/api/login', {
      method: 'POST',
      body: JSON.stringify({ username: u, password: p })
    });
    setAuth(data);
    showToast('登录成功！');
    showMain();
  } catch (e) {
    showToast(e.message, 'error');
  }
}

function setAuth(data) {
  token = data.token;
  username = data.username;
  localStorage.setItem('token', token);
  localStorage.setItem('username', username);
}

function logout() {
  token = null; username = null;
  localStorage.removeItem('token');
  localStorage.removeItem('username');
  intentionalClose = true;
  if (ws) { ws.close(); ws = null; }
  intentionalClose = false;
  
  // Reset fields
  document.getElementById('login-pass').value = '';
  document.getElementById('reg-pass').value = '';
  document.getElementById('clip-input').value = '';
  
  // Re-fetch config to apply registration visibility state
  document.getElementById('tab-register').classList.remove('hidden');
  initApp();
  switchAuthTab('login');
  
  document.getElementById('auth-section').classList.remove('hidden');
  document.getElementById('main-section').classList.add('hidden');
}

function switchView(view) {
  document.querySelectorAll('.nav-item').forEach(el => el.classList.remove('active'));
  document.querySelectorAll('.view-panel').forEach(el => el.classList.add('hidden'));

  const nav = document.getElementById('nav-' + view);
  const v = document.getElementById('view-' + view);
  if (nav) nav.classList.add('active');
  if (v) v.classList.remove('hidden');

  if (view === 'devices') loadDevices();
  if (view === 'users') loadUsers();
  if (view === 'settings') loadSettings();
}

function toggleExpandInput() {
  const input = document.getElementById('clip-input');
  const btn = document.getElementById('expand-btn');
  if (input.classList.contains('expanded')) {
    input.classList.remove('expanded');
    input.style.height = '80px';
    input.style.overflowY = 'hidden';
    btn.innerHTML = '<svg class="icon" viewBox="0 0 24 24"><path d="M16.59 8.59L12 13.17 7.41 8.59 6 10l6 6 6-6z"/></svg>';
    btn.title = '展开';
  } else {
    input.classList.add('expanded');
    input.style.height = input.scrollHeight + 'px';
    input.style.overflowY = 'auto'; // allow inner scroll if huge
    btn.innerHTML = '<svg class="icon" viewBox="0 0 24 24"><path d="M12 8l-6 6 1.41 1.41L12 10.83l4.59 4.58L18 14z"/></svg>';
    btn.title = '收起';
  }
}

document.getElementById('clip-input').addEventListener('input', function() {
  const btn = document.getElementById('expand-btn');
  if (this.classList.contains('expanded')) {
    this.style.height = '80px'; // Temporarily shrink to get real scrollHeight
    const sh = this.scrollHeight;
    this.style.height = sh + 'px';
    if (sh <= 80) {
      this.classList.remove('expanded');
      this.style.height = '80px';
      this.style.overflowY = 'hidden';
      btn.classList.add('hidden');
      btn.innerHTML = '<svg class="icon" viewBox="0 0 24 24"><path d="M16.59 8.59L12 13.17 7.41 8.59 6 10l6 6 6-6z"/></svg>';
      btn.title = '展开';
    }
  } else {
    if (this.scrollHeight > 80) {
      btn.classList.remove('hidden');
    } else {
      btn.classList.add('hidden');
    }
  }
});

function showMain() {
  document.getElementById('auth-section').classList.add('hidden');
  document.getElementById('main-section').classList.remove('hidden');
  document.getElementById('display-user').textContent = username;
  document.getElementById('avatar-letter').textContent = username.charAt(0).toUpperCase();

  if (username === 'admin') {
    document.querySelectorAll('.user-nav').forEach(el => el.classList.add('hidden'));
    document.querySelectorAll('.admin-nav').forEach(el => el.classList.remove('hidden'));
    switchView('users');
  } else {
    document.querySelectorAll('.user-nav').forEach(el => el.classList.remove('hidden'));
    document.querySelectorAll('.admin-nav').forEach(el => el.classList.add('hidden'));
    switchView('clipboard');
    loadHistory();
    connectWS();
  }
}

function copyIcon() { return '<svg class="icon" viewBox="0 0 24 24"><path d="M16 1H4c-1.1 0-2 .9-2 2v14h2V3h12V1zm3 4H8c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h11c1.1 0 2-.9 2-2V7c0-1.1-.9-2-2-2zm0 16H8V7h11v14z"/></svg>'; }
function trashIcon() { return '<svg class="icon" viewBox="0 0 24 24"><path d="M6 19c0 1.1.9 2 2 2h8c1.1 0 2-.9 2-2V7H6v12zM19 4h-3.5l-1-1h-5l-1 1H5v2h14V4z"/></svg>'; }
function editIcon() { return '<svg class="icon" viewBox="0 0 24 24"><path d="M3 17.25V21h3.75L17.81 9.94l-3.75-3.75L3 17.25zM20.71 7.04c.39-.39.39-1.02 0-1.41l-2.34-2.34c-.39-.39-1.02-.39-1.41 0l-1.83 1.83 3.75 3.75 1.83-1.83z"/></svg>'; }
function closeIcon() { return '<svg class="icon" viewBox="0 0 24 24"><path d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z"/></svg>'; }

async function loadHistory() {
  try {
    const data = await apiFetch('/api/clipboard');
    renderClips(data.entries || []);
  } catch (e) {
    showToast('加载历史失败: ' + e.message, 'error');
  }
}

function renderClips(entries) {
  const list = document.getElementById('clip-list');
  if (!entries.length) {
    list.innerHTML = '<div style="padding: 40px; text-align: center; color: var(--text-dim);">暂无剪贴板记录</div>';
    return;
  }
  
  list.innerHTML = '';
  entries.forEach(e => {
    // Relative time approx
    const d = new Date(e.created_at);
    const timeStr = d.toLocaleTimeString('zh-CN', {hour: '2-digit', minute:'2-digit'});
    const dateStr = d.toLocaleDateString('zh-CN', {month: 'short', day: 'numeric'});

    const div = document.createElement('div');
    div.className = 'clip-item';
    
    // Header
    const meta = document.createElement('div');
    meta.className = 'clip-meta';
    const source = e.device_name ? ' · 来自 ' + escapeHtml(e.device_name) : '';
    meta.innerHTML = '<span>' + dateStr + ' ' + timeStr + source + '</span>';
    
    // Actions
    const actions = document.createElement('div');
    actions.className = 'clip-actions';
    
    const copyBtn = document.createElement('button');
    copyBtn.className = 'btn-icon'; copyBtn.title = '复制';
    copyBtn.innerHTML = copyIcon();
    copyBtn.onclick = () => copyClip(e.content);
    
    const delBtn = document.createElement('button');
    delBtn.className = 'btn-icon danger'; delBtn.title = '删除';
    delBtn.innerHTML = trashIcon();
    delBtn.onclick = () => deleteClip(e.id);
    
    actions.appendChild(copyBtn);
    actions.appendChild(delBtn);
    meta.appendChild(actions);

    // Content
    const content = document.createElement('div');
    content.className = 'clip-content';
    content.textContent = e.content; // textContent escapes HTML safely

    div.appendChild(content);
    div.appendChild(meta);
    list.appendChild(div);
  });
}

async function pushClip() {
  const input = document.getElementById('clip-input');
  const content = input.value.trim();
  if (!content) return showToast('内容不能为空', 'error');
  try {
    await apiFetch('/api/clipboard', {
      method: 'POST',
      body: JSON.stringify({ content, device_name: currentDeviceName })
    });
    input.value = '';
    input.dispatchEvent(new Event('input')); // Reset auto-expand state
    showToast('已推送到所有设备');
    loadHistory();
  } catch (e) {
    showToast('推送失败: ' + e.message, 'error');
  }
}

async function copyClip(text) {
  try {
    await navigator.clipboard.writeText(text);
    showToast('已复制到剪贴板');
  } catch {
    const ta = document.createElement('textarea');
    ta.value = text;
    document.body.appendChild(ta);
    ta.select();
    document.execCommand('copy');
    ta.remove();
    showToast('已复制到剪贴板');
  }
}

async function deleteClip(id) {
  try {
    await apiFetch('/api/clipboard/' + id, { method: 'DELETE' });
    loadHistory();
  } catch (e) {
    showToast('删除失败: ' + e.message, 'error');
  }
}

async function clearAllHistory() {
  if (!confirm('确定要清空所有记录吗？此操作无法恢复。')) return;
  try {
    await apiFetch('/api/clipboard/all', { method: 'DELETE' });
    showToast('记录已清空');
    loadHistory();
  } catch (e) {
    showToast('清空失败: ' + e.message, 'error');
  }
}

async function loadDevices() {
  try {
    const data = await apiFetch('/api/devices');
    renderDevices(data.devices || []);
  } catch (e) {
    showToast('加载设备列表失败: ' + e.message, 'error');
  }
}

function renderDevices(devices) {
  currentDevices = devices;
  const list = document.getElementById('device-list');
  const countBadge = document.getElementById('device-count-badge');
  const totalSpan = document.getElementById('device-total');
  countBadge.textContent = devices.length;
  totalSpan.textContent = devices.length + ' 台设备';
  if (!devices.length) {
    list.innerHTML = '<div style="padding: 40px; text-align: center; color: var(--text-dim);">暂无在线设备</div>';
    return;
  }
  list.innerHTML = '';
  devices.forEach(d => {
    const t = new Date(d.connected_at);
    const timeStr = t.toLocaleTimeString('zh-CN', {hour:'2-digit', minute:'2-digit'});
    const dateStr = t.toLocaleDateString('zh-CN', {month:'short', day:'numeric'});
    const isWeb = d.device_name.includes('浏览器') || d.device_name.includes('Web');
    const iconSvg = isWeb
      ? '<svg class="icon" viewBox="0 0 24 24" style="width:1.4em;height:1.4em;"><path d="M20 4H4c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V6c0-1.1-.9-2-2-2zm0 14H4V8h16v12z"/></svg>'
      : '<svg class="icon" viewBox="0 0 24 24" style="width:1.4em;height:1.4em;"><path d="M4 6h18V4H4c-1.1 0-2 .9-2 2v11H0v3h14v-3H4V6zm19 2h-6c-.55 0-1 .45-1 1v10c0 .55.45 1 1 1h6c.55 0 1-.45 1-1V9c0-.55-.45-1-1-1zm-1 9h-4v-7h4v7z"/></svg>';
    const isMe = currentDeviceID && (d.id === currentDeviceID);
    const meTag = isMe ? '<span style="color:var(--accent); font-size:0.8em; margin-left:8px;">(当前设备)</span>' : '';
    
    const div = document.createElement('div');
    div.className = 'device-item';
    
    let btns = '<button class="btn-icon" title="重命名" onclick="renameDevice(' + d.id + ', \'' + escapeHtml(d.device_name).replace(/'/g, "\\'") + '\')">' + editIcon() + '</button>';
    if (!isMe) {
      btns += '<button class="btn-icon danger" title="移除" onclick="removeDevice(' + d.id + ')">' + closeIcon() + '</button>';
    }

    div.innerHTML = '<div class="device-icon">' + iconSvg + '</div>'
      + '<div class="device-info"><div class="device-name">' + escapeHtml(d.device_name) + meTag + '</div>'
      + '<div class="device-time">连接于 ' + dateStr + ' ' + timeStr + '</div></div>'
      + '<div style="display:flex;gap:4px;align-items:center;">'
      + btns
      + '</div>'
      + '<div class="device-status"></div>';
    list.appendChild(div);
  });
}

async function renameDevice(id, currentName) {
  const newName = prompt('请输入新的设备名称:', currentName);
  if (!newName || newName === currentName) return;
  try {
    await apiFetch('/api/devices/' + id + '/rename', {
      method: 'PUT',
      body: JSON.stringify({ device_name: newName })
    });
    showToast('设备已重命名');
  } catch (e) {
    showToast('重命名失败: ' + e.message, 'error');
  }
}

async function removeDevice(id) {
  if (!confirm('确定要移除该设备吗？该设备的 WebSocket 连接将被断开。')) return;
  try {
    await apiFetch('/api/devices/' + id, { method: 'DELETE' });
    showToast('设备已移除');
  } catch (e) {
    showToast('移除失败: ' + e.message, 'error');
  }
}

function escapeHtml(str) {
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

function connectWS() {
  // Close old connection without triggering reconnect
  intentionalClose = true;
  if (ws) { ws.close(); ws = null; }
  intentionalClose = false;

  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  ws = new WebSocket(proto + '//' + location.host + '/ws?token=' + token + '&device_name=' + encodeURIComponent('Web 浏览器'));

  const badge = document.getElementById('ws-badge');
  const status = document.getElementById('ws-status');

  ws.onopen = () => {
    badge.classList.add('online');
    status.textContent = '已连接';
  };

  ws.onclose = () => {
    badge.classList.remove('online');
    status.textContent = '重连中...';
    if (!intentionalClose) {
      setTimeout(() => { if (token) connectWS(); }, 5000);
    }
  };

  ws.onerror = () => {
    badge.classList.remove('online');
    status.textContent = '连接错误';
  };

  ws.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data);
      if (data.type === 'clip') {
        showToast('收到新内容');
        loadHistory();
      } else if (data.type === 'welcome') {
        currentDeviceID = data.client_id;
      } else if (data.type === 'devices_update') {
        // Find our own device to update our local currentDeviceName
        if (currentDeviceID) {
          const me = (data.devices || []).find(d => d.id === currentDeviceID);
          if (me && me.device_name) currentDeviceName = me.device_name;
        }
        renderDevices(data.devices || []);
      } else if (data.type === 'force_disconnect') {
        intentionalClose = true;
        showToast('本设备已被移除，正在登出...', 'error');
        if (ws) { ws.close(); ws = null; }
        setTimeout(logout, 1500);
      }
    } catch {}
  };
}

// ==========================================
// Admin & Modals Logic
// ==========================================

function openPwdModal(targetId = null) {
  pwdContext.targetId = targetId;
  pwdContext.type = targetId ? 'admin_reset' : 'self';
  document.getElementById('pwd-modal-title').textContent = targetId ? '重置该用户密码' : '修改我的密码';
  document.getElementById('pwd-old').style.display = targetId ? 'none' : 'block';
  document.getElementById('pwd-old').value = '';
  document.getElementById('pwd-new').value = '';
  document.getElementById('pwd-modal').classList.remove('hidden');
}

function closePwdModal() {
  document.getElementById('pwd-modal').classList.add('hidden');
}

async function submitPwdModal() {
  const oldPwd = document.getElementById('pwd-old').value;
  const newPwd = document.getElementById('pwd-new').value;

  if (!newPwd || newPwd.length < 6) return showToast('新密码至少6位', 'error');

  try {
    if (pwdContext.type === 'self') {
      if (!oldPwd) return showToast('请输入旧密码', 'error');
      await apiFetch('/api/user/password', {
        method: 'PUT',
        body: JSON.stringify({ old_password: oldPwd, new_password: newPwd })
      });
      showToast('密码修改成功，请重新登录！');
      closePwdModal();
      setTimeout(logout, 1500);
    } else {
      await apiFetch('/api/admin/users/' + pwdContext.targetId + '/password', {
        method: 'PUT',
        body: JSON.stringify({ new_password: newPwd })
      });
      showToast('用户密码已重置');
      closePwdModal();
    }
  } catch (e) {
    showToast(e.message, 'error');
  }
}

async function loadUsers() {
  try {
    const data = await apiFetch('/api/admin/users');
    const list = document.getElementById('admin-user-list');
    if (!data.users || data.users.length === 0) {
      list.innerHTML = '<div style="padding: 20px; text-align: center; color: var(--text-dim);">无用户</div>';
      return;
    }
    let html = '';
    data.users.forEach(u => {
      const isMe = u.username === 'admin';
      const d = new Date(u.created_at);
      let btnHtml = '';
      if (!isMe) {
        btnHtml = '<button class="btn btn-outline btn-small" style="color:var(--danger); border-color:var(--danger);" onclick="deleteUser(' + u.id + ', \'' + escapeHtml(u.username) + '\')">删除账号</button>';
      }
      html += '<div style="display:flex; justify-content:space-between; align-items:center; padding:16px 20px; border-bottom:1px solid var(--border);">' +
                '<div>' +
                  '<div style="font-weight:600;">' + escapeHtml(u.username) + '</div>' +
                  '<div style="font-size:0.8em; color:var(--text-dim);">注册时间: ' + d.toLocaleString('zh-CN') + '</div>' +
                '</div>' +
                '<div style="display:flex; gap:8px;">' +
                  '<button class="btn btn-outline btn-small" onclick="openPwdModal(' + u.id + ')">重置密码</button>' +
                  btnHtml +
                '</div>' +
              '</div>';
    });
    list.innerHTML = html;
  } catch(e) {
    showToast('加载用户失败: ' + e.message, 'error');
  }
}

async function deleteUser(id, name) {
  if (!confirm('确定要删除用户 "' + name + '" 吗？这会清空TA所有的记录并强制下线所有该用户的设备！')) return;
  try {
    await apiFetch('/api/admin/users/' + id, { method: 'DELETE' });
    showToast('用户已删除');
    loadUsers();
  } catch (e) {
    showToast('删除失败: ' + e.message, 'error');
  }
}

let registrationEnabled = false;
async function loadSettings() {
  try {
    const data = await apiFetch('/api/config');
    registrationEnabled = data.allow_registration;
    renderSettings();
  } catch (e) {
    showToast('获取设配失败', 'error');
  }
}

function renderSettings() {
  const btn = document.getElementById('toggle-reg-btn');
  if (registrationEnabled) {
    btn.textContent = '已开启 (点击关闭)';
    btn.className = 'btn btn-primary';
  } else {
    btn.textContent = '已关闭 (点击开启)';
    btn.className = 'btn btn-outline';
  }
}

async function toggleRegistration() {
  const nextState = !registrationEnabled;
  try {
    await apiFetch('/api/admin/config', {
      method: 'PUT',
      body: JSON.stringify({ allow_registration: nextState })
    });
    registrationEnabled = nextState;
    renderSettings();
    showToast('配置已更新');
  } catch(e) {
    showToast('更新失败: ' + e.message, 'error');
  }
}
</script>
</body>
</html>`

func handleClearHistory(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	db.Where("user_id = ?", userID).Delete(&ClipEntry{})
	c.JSON(http.StatusOK, gin.H{"message": "history cleared"})
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
		for _, client := range *val.(*[]*Client) {
			// This will trigger force disconnect logic properly
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

// ============================================================================
// Main
// ============================================================================

func main() {
	// Use release mode for lower memory footprint
	gin.SetMode(gin.DebugMode)

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
