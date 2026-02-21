# ClipSync Server - Cross-device Clipboard Real-time Sync

A lightweight, high-performance clipboard synchronization backend written in Go. As the server component of the ClipSync system, it is optimized for 1GB VPS, featuring real-time WebSocket broadcasting, JWT authentication, and SQLite history storage.

> 💡 **Note**: The desktop client codebase can be found in the `../ClipSyncClient` directory.

## 🌟 Features

- **Extreme Performance**: Idle memory usage < 30MB, perfect for low-end VPS.
- **Modular Codebase**: Clean architecture decoupling routes, WebSocket Hub, Database, and embedded frontend HTML.
- **Real-time Sync**: WebSocket-based Pub/Sub mechanism ensures clipboard content is pushed to all connected devices in sub-seconds.
- **Secure**: Stateless authentication with JWT, passwords hashed with Bcrypt.
- **History Management**: Up to 50 items per user with automatic FIFO pruning.
- **Single Binary**: Frontend and backend are integrated into one binary for easy deployment.
- **Modern UI**: Includes a responsive, dark-themed Web interface.
- **Docker Ready**: Multi-stage build provided for minimal image size.

## 🛠️ Tech Stack

- **Backend**: Go (1.23+), Gin (Web Framework), GORM (SQLite ORM)
- **Auth**: JWT (JSON Web Token)
- **Communication**: Gorilla WebSocket
- **Database**: SQLite 3 (optimized with WAL mode)
- **Frontend**: Vanilla HTML/JS/CSS (Embedded)

## 🚀 Quick Start

### Method 1: Docker (Recommended)

```bash
# Build image
docker build -t clipsync-server .

# Run container
docker run -d \
  --name clipsync-server \
  -p 8080:8080 \
  -v clipsync-data:/app/data \
  -e JWT_SECRET="your-strong-secret-key" \
  clipsync-server
```

### Method 2: Native Build

1. **Prerequisites**: Install Go 1.23+ and a C compiler (for SQLite CGO).
2. **Install Dependencies**:
   ```bash
   go mod tidy
   ```
3. **Build and Run**:
   ```bash
   go build -o clipsyncd .
   ./clipsyncd
   ```

## ⚙️ Environment Variables

| Variable | Default | Description |
|---|---|---|
| `JWT_SECRET` | `clipboard-sync-secret-key-change-me` | JWT Secret Key (Change this in production) |
| `LISTEN_ADDR` | `:8080` | Service listen address |
| `DB_PATH` | `./data/clip.db` | SQLite database file path |

## 🧪 Testing (curl)

1. **Register**:
   ```bash
   curl -X POST http://localhost:8080/api/register -d '{"username":"test","password":"password123"}' -H "Content-Type: application/json"
   ```
2. **Login**:
   ```bash
   curl -X POST http://localhost:8080/api/login -d '{"username":"test","password":"password123"}' -H "Content-Type: application/json"
   ```

## 📄 License

MIT
