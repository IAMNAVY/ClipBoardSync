# ClipSync Server

> The self-hosted backend for the ClipSync ecosystem — lightweight, real-time, all-in-one.

[中文文档](./README_CN.md)

---

## 🌟 Features

- **Ultra-lightweight** — Idle memory < 30 MB; runs comfortably on a 1 GB VPS.
- **Single Binary** — Frontend (responsive dark-theme Web UI) and backend are compiled into one executable.
- **Real-time Sync** — WebSocket Pub/Sub hub broadcasts clipboard content to all connected devices within milliseconds.
- **Secure Auth** — Stateless JWT tokens, Bcrypt password hashing, per-user data isolation.
- **History Retention** — Stores the latest 50 clipboard entries per user with automatic FIFO pruning.
- **Device Management** — View online devices in real-time; remote rename or force-disconnect from the Web dashboard.
- **Admin Panel** — First registered user becomes admin. Manage users, toggle registration, and reset passwords via API.
- **Docker Ready** — Multi-stage Dockerfile for minimal container images.

## 🛠️ Tech Stack

| Layer | Technology |
|---|---|
| Web Framework | [Gin](https://github.com/gin-gonic/gin) |
| ORM | [GORM](https://gorm.io/) + SQLite 3 (WAL mode) |
| WebSocket | [Gorilla WebSocket](https://github.com/gorilla/websocket) |
| Auth | JWT (HS256) + Bcrypt |
| Frontend | Vanilla HTML / CSS / JS (embedded via Go string literal) |

## 📁 Source Files

| File | Responsibility |
|---|---|
| `main.go` | Entry point, route registration, Gin configuration |
| `handlers.go` | REST API handlers (register, login, clipboard CRUD, device management, password change) |
| `websocket.go` | WebSocket upgrade, Hub (register/unregister/broadcast), ping/pong keep-alive |
| `auth.go` | JWT generation, parsing, middleware |
| `database.go` | GORM + SQLite initialization, history limit enforcement |
| `admin.go` | Admin-only routes: list users, delete users, reset passwords, system config |
| `models.go` | Data models (`User`, `ClipEntry`, `SystemSetting`) and request DTOs |
| `frontend.go` | Embedded HTML/CSS/JS for the Web dashboard |
| `Dockerfile` | Multi-stage build (golang → alpine) |

## 🚀 Quick Start

> [Default] Username: admin Password: admin123, Before performing any operation, you should log in to the administrator account and change the password.

### Docker (Recommended)

```bash
docker build -t clipsync-server .

docker run -d \
  --name clipsync-server \
  -p 8080:8080 \
  -v clipsync-data:/app/data \
  -e JWT_SECRET="your-strong-secret-key" \
  clipsync-server
```

### Native Build

**Prerequisites:** Go 1.23+, C compiler (for SQLite CGO, e.g. GCC on Linux, TDM-GCC on Windows).

```bash
go mod tidy
CGO_ENABLED=1 go build -o clipsyncd .
./clipsyncd
```

## ⚙️ Environment Variables

| Variable | Default | Description |
|---|---|---|
| `JWT_SECRET` | `clipboard-sync-secret-key-change-me` | JWT signing key (**must change in production**) |
| `LISTEN_ADDR` | `:8080` | Listen address (`:8080` = all interfaces, port 8080) |
| `DB_PATH` | `./data/clip.db` | SQLite database file path |

## 📡 API Reference

### Public

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/api/register` | Register a new user |
| `POST` | `/api/login` | Login, returns JWT token |
| `GET` | `/api/config` | Public config (e.g. is registration allowed) |
| `GET` | `/ws?token=...&device_name=...` | WebSocket connection |

### Authenticated (Bearer Token)

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/api/clipboard` | Push a clipboard entry |
| `GET` | `/api/clipboard` | Get clipboard history (latest 50) |
| `DELETE` | `/api/clipboard/:id` | Delete a clipboard entry |
| `DELETE` | `/api/clipboard/all` | Clear all history |
| `GET` | `/api/devices` | List connected devices |
| `PUT` | `/api/devices/:id/rename` | Rename a device |
| `DELETE` | `/api/devices/:id` | Force-disconnect a device |
| `PUT` | `/api/user/password` | Change password |

### Admin (Requires admin role)

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/admin/users` | List all users |
| `DELETE` | `/api/admin/users/:id` | Delete a user |
| `PUT` | `/api/admin/users/:id/password` | Reset user password |
| `PUT` | `/api/admin/config` | Update system config (e.g. registration toggle) |

## 🧪 Testing (curl)

```bash
# Register
curl -X POST http://localhost:8080/api/register \
  -H "Content-Type: application/json" \
  -d '{"username":"test","password":"password123"}'

# Login
curl -X POST http://localhost:8080/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"test","password":"password123"}'

# Push clipboard (replace <TOKEN>)
curl -X POST http://localhost:8080/api/clipboard \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <TOKEN>" \
  -d '{"content":"Hello from curl!","device_name":"Terminal"}'

# Get history
curl -H "Authorization: Bearer <TOKEN>" \
  http://localhost:8080/api/clipboard
```

## 🐳 Deployment

- **Production** (Cloudflare Tunnel + Docker Compose): see [`deploy.md`](./deploy.md)
- **Production（中文）**: 见 [`deploy_CN.md`](./deploy_CN.md)

## 📄 License

MIT
