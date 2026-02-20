package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
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
	jwtSecret  = []byte(getEnv("JWT_SECRET", "clipboard-sync-secret-key-change-me"))
	listenAddr = getEnv("LISTEN_ADDR", ":8080")
	dbPath     = getEnv("DB_PATH", "./data/clip.db")
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
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index;not null" json:"user_id"`
	Content   string    `gorm:"type:text;not null" json:"content"`
	CreatedAt time.Time `json:"created_at"`
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
	Content string `json:"content" binding:"required"`
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

type Client struct {
	conn   *websocket.Conn
	userID uint
	mu     sync.Mutex
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
	db.AutoMigrate(&User{}, &ClipEntry{})
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

	entry := ClipEntry{
		UserID:  userID,
		Content: req.Content,
	}
	if err := db.Create(&entry).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save clipboard"})
		return
	}

	// Enforce FIFO limit
	enforceHistoryLimit(userID)

	// Broadcast to all connected devices of this user
	hub.broadcast(userID, gin.H{
		"type":       "clip",
		"content":    entry.Content,
		"id":         entry.ID,
		"created_at": entry.CreatedAt,
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

// ============================================================================
// WebSocket handler
// ============================================================================

func handleWebSocket(c *gin.Context) {
	tokenStr := c.Query("token")
	if tokenStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
		return
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

	client := &Client{conn: conn, userID: userID}
	hub.register(client)
	log.Printf("[ws] user %d connected (total devices: %d)", userID, countUserClients(userID))

	defer func() {
		hub.unregister(client)
		conn.Close()
		log.Printf("[ws] user %d disconnected", userID)
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
				UserID:  userID,
				Content: content,
			}
			if err := db.Create(&entry).Error; err != nil {
				log.Printf("[ws] db error for user %d: %v", userID, err)
				continue
			}
			enforceHistoryLimit(userID)

			// Broadcast to OTHER devices of this user
			hub.broadcast(userID, gin.H{
				"type":       "clip",
				"content":    entry.Content,
				"id":         entry.ID,
				"created_at": entry.CreatedAt,
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
<title>ClipSync - 剪贴板同步</title>
<style>
  :root {
    --bg: #0f1117;
    --surface: #1a1d27;
    --surface2: #242836;
    --border: #2e3348;
    --primary: #6c5ce7;
    --primary-hover: #7c6ff7;
    --accent: #00cec9;
    --text: #e4e6ef;
    --text-dim: #8b8fa3;
    --danger: #e17055;
    --success: #00b894;
    --radius: 12px;
    --shadow: 0 4px 24px rgba(0,0,0,.3);
  }
  * { margin:0; padding:0; box-sizing:border-box; }
  body {
    font-family: 'Segoe UI', system-ui, -apple-system, sans-serif;
    background: var(--bg);
    color: var(--text);
    min-height: 100vh;
    display: flex;
    justify-content: center;
    align-items: flex-start;
    padding: 20px;
  }
  .container { max-width: 560px; width: 100%; margin-top: 30px; }
  .logo {
    text-align: center;
    margin-bottom: 32px;
  }
  .logo h1 {
    font-size: 2em;
    background: linear-gradient(135deg, var(--primary), var(--accent));
    -webkit-background-clip: text;
    -webkit-text-fill-color: transparent;
    font-weight: 800;
    letter-spacing: -1px;
  }
  .logo p { color: var(--text-dim); font-size: .9em; margin-top: 4px; }
  .card {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 24px;
    margin-bottom: 16px;
    box-shadow: var(--shadow);
  }
  .card h2 { font-size: 1.1em; margin-bottom: 16px; color: var(--accent); }
  input[type="text"], input[type="password"], textarea {
    width: 100%;
    padding: 12px 14px;
    background: var(--surface2);
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text);
    font-size: .95em;
    outline: none;
    transition: border-color .2s;
    margin-bottom: 12px;
  }
  input:focus, textarea:focus { border-color: var(--primary); }
  textarea { resize: vertical; min-height: 80px; font-family: inherit; }
  .btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 10px 20px;
    border: none;
    border-radius: 8px;
    font-size: .9em;
    font-weight: 600;
    cursor: pointer;
    transition: all .2s;
    gap: 6px;
  }
  .btn-primary { background: var(--primary); color: #fff; }
  .btn-primary:hover { background: var(--primary-hover); transform: translateY(-1px); }
  .btn-accent { background: var(--accent); color: var(--bg); }
  .btn-accent:hover { opacity: .9; transform: translateY(-1px); }
  .btn-danger { background: var(--danger); color: #fff; font-size: .8em; padding: 6px 12px; }
  .btn-danger:hover { opacity: .85; }
  .btn-small { padding: 6px 14px; font-size: .8em; }
  .btn-group { display: flex; gap: 8px; }
  .status-bar {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 16px;
    background: var(--surface2);
    border-radius: 8px;
    margin-bottom: 16px;
    font-size: .85em;
  }
  .status-dot {
    width: 8px; height: 8px;
    border-radius: 50%;
    background: var(--danger);
    flex-shrink: 0;
  }
  .status-dot.online { background: var(--success); animation: pulse 2s infinite; }
  @keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: .5; }
  }
  .clip-item {
    background: var(--surface2);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 12px 14px;
    margin-bottom: 8px;
    position: relative;
    transition: border-color .2s;
  }
  .clip-item:hover { border-color: var(--primary); }
  .clip-content {
    font-size: .9em;
    white-space: pre-wrap;
    word-break: break-all;
    max-height: 120px;
    overflow: hidden;
    line-height: 1.5;
    color: var(--text);
  }
  .clip-meta {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-top: 8px;
    font-size: .75em;
    color: var(--text-dim);
  }
  .clip-actions { display: flex; gap: 6px; }
  .toast {
    position: fixed;
    top: 20px;
    right: 20px;
    padding: 12px 20px;
    border-radius: 8px;
    font-size: .9em;
    font-weight: 500;
    z-index: 9999;
    animation: slideIn .3s ease;
    color: #fff;
  }
  .toast.success { background: var(--success); }
  .toast.error { background: var(--danger); }
  @keyframes slideIn {
    from { transform: translateX(100%); opacity: 0; }
    to { transform: translateX(0); opacity: 1; }
  }
  .hidden { display: none !important; }
  .user-info {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 16px;
  }
  .user-info span { color: var(--accent); font-weight: 600; }
  #clip-list { max-height: 60vh; overflow-y: auto; }
  #clip-list::-webkit-scrollbar { width: 4px; }
  #clip-list::-webkit-scrollbar-track { background: transparent; }
  #clip-list::-webkit-scrollbar-thumb { background: var(--border); border-radius: 4px; }
</style>
</head>
<body>
<div class="container">
  <div class="logo">
    <h1>📋 ClipSync</h1>
    <p>跨设备剪贴板实时同步</p>
  </div>

  <!-- Auth Section -->
  <div id="auth-section">
    <div class="card">
      <h2 id="auth-title">🔑 登录</h2>
      <input type="text" id="username" placeholder="用户名 (至少 2 位)">
      <input type="password" id="password" placeholder="密码 (至少 6 位)">
      <div class="btn-group">
        <button class="btn btn-primary" onclick="login()">登录</button>
        <button class="btn btn-accent" onclick="register()">注册</button>
      </div>
    </div>
  </div>

  <!-- Main Section -->
  <div id="main-section" class="hidden">
    <div class="user-info">
      <span>👤 <span id="display-user"></span></span>
      <button class="btn btn-danger btn-small" onclick="logout()">退出登录</button>
    </div>

    <div class="status-bar">
      <div class="status-dot" id="ws-dot"></div>
      <span id="ws-status">未连接</span>
    </div>

    <!-- Push Clipboard -->
    <div class="card">
      <h2>📤 推送剪贴板</h2>
      <textarea id="clip-input" placeholder="输入要同步的内容..."></textarea>
      <button class="btn btn-primary" onclick="pushClip()">发送到所有设备</button>
    </div>

    <!-- History -->
    <div class="card">
      <h2>📜 历史记录</h2>
      <div id="clip-list"></div>
    </div>
  </div>
</div>

<script>
const API = window.location.origin;
let token = localStorage.getItem('token');
let username = localStorage.getItem('username');
let ws = null;

// Init
if (token) showMain();

function showToast(msg, type='success') {
  const t = document.createElement('div');
  t.className = 'toast ' + type;
  t.textContent = msg;
  document.body.appendChild(t);
  setTimeout(() => t.remove(), 3000);
}

async function apiFetch(path, opts={}) {
  const headers = { 'Content-Type': 'application/json', ...opts.headers };
  if (token) headers['Authorization'] = 'Bearer ' + token;
  const res = await fetch(API + path, { ...opts, headers });
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || 'request failed');
  return data;
}

async function register() {
  const u = document.getElementById('username').value.trim();
  const p = document.getElementById('password').value;
  if (!u || !p) return showToast('请填写用户名和密码', 'error');
  try {
    const data = await apiFetch('/api/register', {
      method: 'POST',
      body: JSON.stringify({ username: u, password: p })
    });
    token = data.token;
    username = data.username;
    localStorage.setItem('token', token);
    localStorage.setItem('username', username);
    showToast('注册成功！');
    showMain();
  } catch (e) {
    showToast(e.message, 'error');
  }
}

async function login() {
  const u = document.getElementById('username').value.trim();
  const p = document.getElementById('password').value;
  if (!u || !p) return showToast('请填写用户名和密码', 'error');
  try {
    const data = await apiFetch('/api/login', {
      method: 'POST',
      body: JSON.stringify({ username: u, password: p })
    });
    token = data.token;
    username = data.username;
    localStorage.setItem('token', token);
    localStorage.setItem('username', username);
    showToast('登录成功！');
    showMain();
  } catch (e) {
    showToast(e.message, 'error');
  }
}

function logout() {
  token = null;
  username = null;
  localStorage.removeItem('token');
  localStorage.removeItem('username');
  if (ws) ws.close();
  document.getElementById('auth-section').classList.remove('hidden');
  document.getElementById('main-section').classList.add('hidden');
}

function showMain() {
  document.getElementById('auth-section').classList.add('hidden');
  document.getElementById('main-section').classList.remove('hidden');
  document.getElementById('display-user').textContent = username;
  loadHistory();
  connectWS();
}

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
    list.innerHTML = '<p style="color:var(--text-dim);text-align:center;padding:20px">暂无记录</p>';
    return;
  }
  list.innerHTML = entries.map(e => {
    const time = new Date(e.created_at).toLocaleString('zh-CN');
    const preview = e.content.length > 200 ? e.content.substring(0, 200) + '...' : e.content;
    return '<div class="clip-item">' +
      '<div class="clip-content">' + escapeHtml(preview) + '</div>' +
      '<div class="clip-meta"><span>' + time + '</span>' +
      '<div class="clip-actions">' +
      '<button class="btn btn-primary btn-small" onclick="copyClip(\'' + escapeJs(e.content) + '\')">📋 复制</button>' +
      '<button class="btn btn-danger btn-small" onclick="deleteClip(' + e.id + ')">🗑️</button>' +
      '</div></div></div>';
  }).join('');
}

function escapeHtml(str) {
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

function escapeJs(str) {
  return str.replace(/\\/g, '\\\\').replace(/'/g, "\\'").replace(/\n/g, '\\n').replace(/\r/g, '\\r');
}

async function pushClip() {
  const input = document.getElementById('clip-input');
  const content = input.value.trim();
  if (!content) return showToast('内容不能为空', 'error');
  try {
    await apiFetch('/api/clipboard', {
      method: 'POST',
      body: JSON.stringify({ content })
    });
    input.value = '';
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
    // Fallback
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
    showToast('已删除');
    loadHistory();
  } catch (e) {
    showToast('删除失败: ' + e.message, 'error');
  }
}

function connectWS() {
  if (ws) ws.close();
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  ws = new WebSocket(proto + '//' + location.host + '/ws?token=' + token);

  const dot = document.getElementById('ws-dot');
  const status = document.getElementById('ws-status');

  ws.onopen = () => {
    dot.classList.add('online');
    status.textContent = '已连接 — 实时同步中';
  };

  ws.onclose = () => {
    dot.classList.remove('online');
    status.textContent = '连接断开，5秒后重连...';
    setTimeout(() => { if (token) connectWS(); }, 5000);
  };

  ws.onerror = () => {
    dot.classList.remove('online');
    status.textContent = '连接错误';
  };

  ws.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data);
      if (data.type === 'clip') {
        showToast('收到新的剪贴板内容');
        loadHistory();
      }
    } catch {}
  };
}
</script>
</body>
</html>`

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

	// WebSocket
	r.GET("/ws", handleWebSocket)

	// Protected routes
	api := r.Group("/api", authMiddleware())
	{
		api.POST("/clipboard", handlePushClip)
		api.GET("/clipboard", handleGetHistory)
		api.DELETE("/clipboard/:id", handleDeleteClip)
	}

	log.Printf("🚀 ClipSync server starting on %s", listenAddr)
	log.Printf("📂 Database: %s", dbPath)
	log.Printf("🌐 Open http://localhost%s in your browser", listenAddr)

	if err := r.Run(listenAddr); err != nil {
		log.Fatal("server error:", err)
	}
}
