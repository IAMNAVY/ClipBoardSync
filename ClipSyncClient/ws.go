package main

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WSClient manages a persistent WebSocket connection with auto-reconnect.
type WSClient struct {
	serverURL  string
	token      string
	deviceName string

	conn      *websocket.Conn
	mu        sync.Mutex
	connected bool // current connection state

	clientID uint64 // assigned by server via welcome message

	onClip            func(content string) // called when a remote clip is received
	onStatus          func(connected bool) // called on connection status change
	onDeviceRenamed   func(newName string) // called when server renames this device
	onForceDisconnect func(reason string)  // called on server force disconnect
	onTokenExpired    func()               // called when token appears expired (401 on connect)

	forceDisconnected bool // set when server sends force_disconnect

	stopCh      chan struct{}
	done        chan struct{}
	reconnectCh chan struct{} // external trigger for immediate reconnect
}

// NewWSClient creates a new WebSocket client.
func NewWSClient(serverURL, token, deviceName string, onClip func(string), onStatus func(bool), onDeviceRenamed func(string), onForceDisconnect func(string), onTokenExpired func()) *WSClient {
	return &WSClient{
		serverURL:         serverURL,
		token:             token,
		deviceName:        deviceName,
		onClip:            onClip,
		onStatus:          onStatus,
		onDeviceRenamed:   onDeviceRenamed,
		onForceDisconnect: onForceDisconnect,
		onTokenExpired:    onTokenExpired,
		stopCh:            make(chan struct{}),
		done:              make(chan struct{}),
		reconnectCh:       make(chan struct{}, 1),
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

// Reconnect triggers an immediate reconnection attempt.
// If already connected, it closes the current connection first.
func (w *WSClient) Reconnect() {
	log.Println("[ws] 手动触发重连")
	w.mu.Lock()
	if w.conn != nil {
		w.conn.Close() // force close to break the read loop
	}
	w.mu.Unlock()

	// Signal the connect loop to skip the backoff wait
	select {
	case w.reconnectCh <- struct{}{}:
	default:
		// already signalled, no need to send again
	}
}

// IsConnected returns the current connection state.
func (w *WSClient) IsConnected() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.connected
}

// UpdateToken updates the JWT token for reconnection.
func (w *WSClient) UpdateToken(token string) {
	w.mu.Lock()
	w.token = token
	w.mu.Unlock()
}

// buildWSURL converts http(s)://host to ws(s)://host/ws?token=xxx&device_name=yyy
func (w *WSClient) buildWSURL() string {
	w.mu.Lock()
	serverURL := w.serverURL
	token := w.token
	devName := w.deviceName
	w.mu.Unlock()

	wsURL := strings.Replace(serverURL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL = strings.TrimRight(wsURL, "/")

	u, _ := url.Parse(wsURL + "/ws")
	q := u.Query()
	q.Set("token", token)
	q.Set("device_name", devName)
	u.RawQuery = q.Encode()
	return u.String()
}

func (w *WSClient) connectLoop() {
	defer close(w.done)

	const initialBackoff = time.Second
	const maxBackoff = 30 * time.Second
	backoff := initialBackoff

	for {
		select {
		case <-w.stopCh:
			return
		default:
		}

		// Drain any pending reconnect signal before connecting
		select {
		case <-w.reconnectCh:
		default:
		}

		wasConnected, err := w.connectAndListen()
		if err != nil {
			log.Printf("[ws] 连接断开: %v", err)
		}

		w.mu.Lock()
		w.connected = false
		w.mu.Unlock()
		if w.onStatus != nil {
			w.onStatus(false)
		}

		// Reset backoff if we had a successful connection (was actually online)
		if wasConnected {
			backoff = initialBackoff
		}

		// If force disconnected by server, stop reconnecting
		w.mu.Lock()
		forced := w.forceDisconnected
		w.mu.Unlock()
		if forced {
			log.Println("[ws] 服务端强制下线，停止重连")
			return
		}

		log.Printf("[ws] 将在 %v 后重连...", backoff)

		// Wait before reconnecting, but allow immediate reconnect via reconnectCh
		select {
		case <-w.stopCh:
			return
		case <-w.reconnectCh:
			log.Println("[ws] 收到重连信号，立即重连")
			backoff = initialBackoff // reset backoff on manual reconnect
		case <-time.After(backoff):
		}

		// Exponential backoff
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// connectAndListen connects to the server and processes messages.
// Returns (wasConnected, error) where wasConnected indicates if the
// connection was ever successfully established.
func (w *WSClient) connectAndListen() (bool, error) {
	wsURL := w.buildWSURL()
	log.Printf("[ws] 正在连接 %s", wsURL)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, resp, err := dialer.Dial(wsURL, nil)
	if err != nil {
		// Check for 401 Unauthorized — token likely expired
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			log.Println("[ws] 服务端返回 401，Token 可能已过期")
			if w.onTokenExpired != nil {
				w.onTokenExpired()
			}
		}
		return false, err
	}

	w.mu.Lock()
	w.conn = conn
	w.clientID = 0 // reset until we receive welcome
	w.connected = true
	w.mu.Unlock()

	log.Println("[ws] 已连接")
	if w.onStatus != nil {
		w.onStatus(true)
	}

	// Configure ping/pong handlers — tighter deadline for faster dead connection detection
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	// PingHandler: reply Pong to server pings and refresh read deadline
	conn.SetPingHandler(func(appData string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		w.mu.Lock()
		conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		err := conn.WriteMessage(websocket.PongMessage, []byte(appData))
		w.mu.Unlock()
		return err
	})

	// Ping ticker — more frequent pings for faster dead connection detection
	pingDone := make(chan struct{})
	go func() {
		defer close(pingDone)
		ticker := time.NewTicker(20 * time.Second)
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
			return true, err
		}

		var msg map[string]interface{}
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		msgType, _ := msg["type"].(string)

		switch msgType {
		case "clip":
			if content, ok := msg["content"].(string); ok && content != "" {
				log.Printf("[ws] 收到远程剪贴板内容 (%d 字符)", len(content))
				if w.onClip != nil {
					w.onClip(content)
				}
			}

		case "welcome":
			// Server tells us our client ID
			if idFloat, ok := msg["client_id"].(float64); ok {
				w.mu.Lock()
				w.clientID = uint64(idFloat)
				w.mu.Unlock()
				log.Printf("[ws] 收到 welcome, client_id=%d", uint64(idFloat))
			}

		case "devices_update":
			// Check if server renamed this device
			w.handleDevicesUpdate(msg)

		case "force_disconnect":
			reason, _ := msg["reason"].(string)
			log.Printf("[ws] 服务端强制下线: %s", reason)
			w.mu.Lock()
			w.forceDisconnected = true
			cb := w.onForceDisconnect
			w.mu.Unlock()
			if cb != nil {
				// Call asynchronously to avoid blocking the read loop exit
				go cb(reason)
			}
			return true, nil // exit read loop, connectLoop will see forceDisconnected
		}
	}
}

// handleDevicesUpdate checks if the server has renamed this client's device.
func (w *WSClient) handleDevicesUpdate(msg map[string]interface{}) {
	w.mu.Lock()
	myID := w.clientID
	myName := w.deviceName
	w.mu.Unlock()

	if myID == 0 {
		return // haven't received welcome yet
	}

	devices, ok := msg["devices"].([]interface{})
	if !ok {
		return
	}

	for _, d := range devices {
		dev, ok := d.(map[string]interface{})
		if !ok {
			continue
		}
		idFloat, _ := dev["id"].(float64)
		if uint64(idFloat) == myID {
			serverName, _ := dev["device_name"].(string)
			if serverName != "" && serverName != myName {
				log.Printf("[ws] 服务端重命名设备: '%s' -> '%s'", myName, serverName)
				w.mu.Lock()
				w.deviceName = serverName
				w.mu.Unlock()
				if w.onDeviceRenamed != nil {
					w.onDeviceRenamed(serverName)
				}
			}
			break
		}
	}
}
