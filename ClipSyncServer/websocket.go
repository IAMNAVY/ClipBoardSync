package main

import (
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

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
