<p align="center">
  <h1 align="center">📋 ClipSync</h1>
  <p align="center">
    <strong>Cross-device clipboard synchronization — instant, secure, self-hosted.</strong>
  </p>
  <p align="center">
    <a href="#-features">Features</a> •
    <a href="#-architecture">Architecture</a> •
    <a href="#-quick-start">Quick Start</a> •
    <a href="#-components">Components</a> •
    <a href="./README_CN.md">中文文档</a>
  </p>
</p>

---

## ✨ Features

- 🔄 **Real-time Sync** — Sub-second clipboard push via WebSocket; copy on one device, paste on another instantly.
- 🖥️ **Multi-platform** — Windows desktop tray app, Android app, and a responsive Web dashboard, all connected to one server.
- 🔐 **Secure by Design** — Stateless JWT authentication, Bcrypt-hashed passwords, per-user data isolation.
- 🪶 **Ultra-lightweight Server** — Idle memory &lt; 30 MB, single binary with embedded frontend; perfect for 1 GB VPS.
- 📱 **Android Deep Integration** — AccessibilityService-based clipboard monitoring for Android 10+, foreground service keep-alive, battery optimization bypass.
- 🛡️ **Smart Anti-loop** — Built-in mechanism prevents infinite sync storms between devices.
- 📡 **Device Management** — View online devices, remote rename, and force-disconnect from the Web UI.
- 🗂️ **Clipboard History** — Up to 50 entries per user with automatic FIFO pruning, viewable and deletable via Web dashboard.
- 🔀 **Flexible Sync Modes** — Bidirectional, upload-only, download-only, or off — configurable per client.
- 🐳 **Docker Ready** — Multi-stage Dockerfile included; deploy with `docker-compose up -d`.

---

## 🏗️ Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                         ClipSync Server                          │
│          Go · Gin · GORM · SQLite · Gorilla WebSocket            │
│                                                                  │
│   ┌──────────┐   ┌──────────────┐   ┌────────────────────────┐   │
│   │ REST API │   │ WebSocket Hub│   │ Embedded Web Dashboard │   │
│   └────┬─────┘   └──────┬───────┘   └────────────────────────┘   │
│        │                │                                        │
│        └────────┬───────┘                                        │
│                 │                                                │
│          ┌──────┴──────┐                                         │
│          │  SQLite DB  │                                         │
│          └─────────────┘                                         │
└──────────────────────────────────────────────────────────────────┘
         ▲  WebSocket + REST           ▲  WebSocket + REST
         │                             │
   ┌─────┴──────────┐          ┌──────┴──────────────┐
   │  ClipSync       │          │  ClipSync Android    │
   │  Windows Client │          │  Kotlin · Compose    │
   │  Go · Tray App  │          │  AccessibilityService│
   └─────────────────┘          └─────────────────────┘
```

---

## 🚀 Quick Start

> [Default] Username: admin Password: admin123, Before performing any operation, you should log in to the administrator account and change the password.

### 1. Deploy the Server

The fastest way is with Docker:

```bash
cd ClipSyncServer

docker build -t clipsync-server .

docker run -d \
  --name clipsync-server \
  -p 8080:8080 \
  -v clipsync-data:/app/data \
  -e JWT_SECRET="your-strong-secret-key" \
  clipsync-server
```

> 📖 For a production deployment with **Cloudflare Tunnel + Docker Compose**, see [`ClipSyncServer/deploy.md`](./ClipSyncServer/deploy.md).

### 2. Connect the Windows Client

1. Download or compile `ClipSyncClient.exe`.
2. Launch it — a configuration window will appear on first run.
3. Enter your server URL, username, and token (from the Web dashboard).
4. The client hides to the system tray and syncs silently.

### 3. Connect the Android Client

1. Install the APK and log in with your server URL and credentials.
2. Enable the **AccessibilityService** and **battery optimization bypass** as guided.
3. The service runs in the background — your clipboard is now synced.

---

## 📦 Components

| Component | Directory | Language | Description |
|---|---|---|---|
| **Server** | [`ClipSyncServer/`](./ClipSyncServer/) | Go | Backend API, WebSocket hub, embedded Web UI, SQLite storage |
| **Windows Client** | [`ClipSyncClient/`](./ClipSyncClient/) | Go | System tray app with GUI configuration, auto-start support |
| **Android Client** | [`ClipSyncAndroid/`](./ClipSyncAndroid/) | Kotlin | Jetpack Compose UI, AccessibilityService, foreground service |

Each sub-project has its own detailed README with build instructions and usage guides.

---

## 🛠️ Tech Stack

| Layer | Technology |
|---|---|
| Server | Go 1.23+, Gin, GORM, SQLite (WAL), Gorilla WebSocket |
| Auth | JWT (HS256), Bcrypt |
| Desktop Client | Go, Win32 System Tray, CGO |
| Android Client | Kotlin, Jetpack Compose, OkHttp, AccessibilityService |
| Deployment | Docker, Docker Compose, Cloudflare Tunnel |

---

## 📁 Project Structure

```
ClipBoardSync/
├── ClipSyncServer/         # Server (Go)
│   ├── main.go             # Entry point & route registration
│   ├── handlers.go         # REST API handlers
│   ├── websocket.go        # WebSocket hub & real-time broadcast
│   ├── auth.go             # JWT middleware
│   ├── database.go         # GORM + SQLite setup
│   ├── admin.go            # Admin route handlers
│   ├── models.go           # Data models & DTOs
│   ├── frontend.go         # Embedded HTML/CSS/JS dashboard
│   ├── Dockerfile          # Multi-stage Docker build
│   └── deploy.md           # Production deployment guide
├── ClipSyncClient/         # Windows Client (Go)
│   ├── main.go             # Entry point, sync lifecycle
│   ├── ws.go               # WebSocket client with reconnect
│   ├── clipboard.go        # Clipboard monitoring & anti-loop
│   ├── tray.go             # System tray icon & menu
│   ├── gui.go              # Native configuration window
│   ├── config.go           # Persistent config management
│   ├── api.go              # HTTP API calls
│   └── autostart.go        # Windows registry auto-start
├── ClipSyncAndroid/        # Android Client (Kotlin)
│   └── app/src/main/java/com/clipsync/android/
│       ├── MainActivity.kt
│       ├── clipboard/      # Clipboard monitoring (Accessibility, Logcat)
│       ├── data/           # AppConfig, preferences
│       ├── network/        # API client, WebSocket client
│       ├── service/        # Foreground service, boot receiver
│       └── ui/             # Compose screens & theme
└── README.md               # ← You are here
```

---

## ⚙️ Environment Variables (Server)

| Variable | Default | Description |
|---|---|---|
| `JWT_SECRET` | `clipboard-sync-secret-key-change-me` | JWT signing key (**change in production**) |
| `LISTEN_ADDR` | `:8080` | Listen address |
| `DB_PATH` | `./data/clip.db` | SQLite database path |

---

## 🧪 API Quick Test

```bash
# Register
curl -X POST http://localhost:8080/api/register \
  -H "Content-Type: application/json" \
  -d '{"username":"demo","password":"password123"}'

# Login
curl -X POST http://localhost:8080/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"demo","password":"password123"}'
```

---

## 🤖 AI Acknowledgements

This project was developed with the assistance of advanced AI models to optimize architecture, refine code quality, and accelerate cross-platform implementation:

* **Claude 4.6 Opus** (via Antigravity): Assisted in core logic design, Android AccessibilityService integration, and Windows system tray implementation.
* **Gemini 3.1 Pro**: Assisted in WebSocket synchronization strategy, documentation, and Docker optimization.

---

## 📄 License

MIT

