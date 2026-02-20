package main

import (
	"encoding/json"
	"log"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WSClient manages a persistent WebSocket connection with auto-reconnect.
type WSClient struct {
	serverURL string
	token     string

	conn *websocket.Conn
	mu   sync.Mutex

	onClip   func(content string) // called when a remote clip is received
	onStatus func(connected bool) // called on connection status change

	stopCh chan struct{}
	done   chan struct{}
}

// NewWSClient creates a new WebSocket client.
func NewWSClient(serverURL, token string, onClip func(string), onStatus func(bool)) *WSClient {
	return &WSClient{
		serverURL: serverURL,
		token:     token,
		onClip:    onClip,
		onStatus:  onStatus,
		stopCh:    make(chan struct{}),
		done:      make(chan struct{}),
	}
}

// Start begins the connection loop in a goroutine.
func (w *WSClient) Start() {
	go w.connectLoop()
}

// Stop gracefully closes the WebSocket connection.
func (w *WSClient) Stop() {
	close(w.stopCh)
	w.mu.Lock()
	if w.conn != nil {
		w.conn.Close()
	}
	w.mu.Unlock()
	<-w.done
}

// UpdateToken updates the JWT token for reconnection.
func (w *WSClient) UpdateToken(token string) {
	w.mu.Lock()
	w.token = token
	w.mu.Unlock()
}

// buildWSURL converts http(s)://host to ws(s)://host/ws?token=xxx
func (w *WSClient) buildWSURL() string {
	w.mu.Lock()
	serverURL := w.serverURL
	token := w.token
	w.mu.Unlock()

	wsURL := strings.Replace(serverURL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL = strings.TrimRight(wsURL, "/")

	u, _ := url.Parse(wsURL + "/ws")
	q := u.Query()
	q.Set("token", token)
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "Desktop Client"
	}
	q.Set("device_name", hostname)
	u.RawQuery = q.Encode()
	return u.String()
}

func (w *WSClient) connectLoop() {
	defer close(w.done)

	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-w.stopCh:
			return
		default:
		}

		err := w.connectAndListen()
		if err != nil {
			log.Printf("[ws] 连接断开: %v", err)
		}
		if w.onStatus != nil {
			w.onStatus(false)
		}

		// Wait before reconnecting
		select {
		case <-w.stopCh:
			return
		case <-time.After(backoff):
		}

		// Exponential backoff
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (w *WSClient) connectAndListen() error {
	wsURL := w.buildWSURL()
	log.Printf("[ws] 正在连接 %s", wsURL)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return err
	}

	w.mu.Lock()
	w.conn = conn
	w.mu.Unlock()

	log.Println("[ws] 已连接")
	if w.onStatus != nil {
		w.onStatus(true)
	}

	// Configure pong handler
	conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		return nil
	})

	// Ping ticker
	pingDone := make(chan struct{})
	go func() {
		defer close(pingDone)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				w.mu.Lock()
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				err := conn.WriteMessage(websocket.PingMessage, nil)
				w.mu.Unlock()
				if err != nil {
					return
				}
			case <-w.stopCh:
				return
			}
		}
	}()

	defer func() {
		conn.Close()
		<-pingDone
	}()

	// Read loop
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		var msg map[string]interface{}
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		if msgType, ok := msg["type"].(string); ok && msgType == "clip" {
			if content, ok := msg["content"].(string); ok && content != "" {
				log.Printf("[ws] 收到远程剪贴板内容 (%d 字符)", len(content))
				if w.onClip != nil {
					w.onClip(content)
				}
			}
		}
	}
}
