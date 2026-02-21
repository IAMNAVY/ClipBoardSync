# ClipSync Server — Production Deployment Guide

> Deploy ClipSync Server on a Linux VPS with **Docker Compose** and expose it securely via **Cloudflare Tunnel** (zero-trust, no open ports needed).

[中文部署指南](./deploy_CN.md)

---

## 📋 Prerequisites

| Item | Requirement |
|---|---|
| Linux VPS | Ubuntu 20.04+ / Debian 11+ / any distro with Docker support |
| Docker | 20.10+ |
| Docker Compose | v2 (bundled with Docker Desktop or `docker compose` plugin) |
| Domain | Managed in Cloudflare DNS |
| Cloudflare Account | Free tier is sufficient |

---

## 🏗️ Architecture

```
Internet
   │
   │  HTTPS (443)
   ▼
┌──────────────────────┐
│   Cloudflare Edge    │  ← SSL termination, DDoS protection
│   (Zero Trust Tunnel)│
└──────────┬───────────┘
           │  Encrypted tunnel (outbound-only from your server)
           ▼
┌──────────────────────┐
│   cloudflared        │  ← Tunnel daemon container
│   (Docker container) │
└──────────┬───────────┘
           │  http://clipsync-server:8080  (Docker internal network)
           ▼
┌──────────────────────┐
│   ClipSync Server    │  ← Application container
│   (Docker container) │
│   Port 8080 (internal)│
└──────────┬───────────┘
           │
           ▼
┌──────────────────────┐
│   SQLite Database    │  ← Persistent volume
│   /app/data/clip.db  │
└──────────────────────┘
```

**Key point:** The server does NOT expose any port to the public internet. Cloudflare Tunnel creates an outbound-only encrypted connection from your VPS to Cloudflare's edge, which then serves traffic to your users.

---

## 📝 Step-by-Step (Production)

### Step 1: Prepare the Server

```bash
# Update system
sudo apt update && sudo apt upgrade -y

# Install Docker (if not installed)
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER

# Log out and back in for group changes to take effect
# Verify Docker
docker --version
docker compose version
```

### Step 2: Create a Cloudflare Tunnel

1. Go to [Cloudflare Zero Trust Dashboard](https://one.dash.cloudflare.com/).
2. Navigate to **Networks** → **Tunnels** → **Create a tunnel**.
3. Choose **Cloudflared** as the connector type.
4. Name your tunnel (e.g. `clipsync-tunnel`).
5. **Copy the tunnel token** — you'll need it in the next step. It looks like:

   ```
   eyJhIjoiZmQ0M...very-long-string...
   ```

6. In **Public Hostnames**, add a route:
   - **Subdomain**: `clip` (or your preference)
   - **Domain**: `yourdomain.com`
   - **Service**: `http://clipsync-server:8080`

   > ⚠️ The service URL must use the Docker Compose service name (`clipsync-server`), as both containers share the same Docker network.

7. Under **Additional application settings** → **HTTP Settings**:
   - Enable **WebSockets** — this is **critical** for real-time sync.
   - Set **HTTP Host Header** to leave as default.

8. Save the tunnel configuration.

### Step 3: Create Project Directory

```bash
mkdir -p ~/clipsync && cd ~/clipsync
```

### Step 4: Transfer Source Code

Transfer the `ClipSyncServer` directory to your server:

```bash
# Option A: Git clone (if repo is on GitHub)
git clone https://github.com/youruser/ClipBoardSync.git
cd ClipBoardSync/ClipSyncServer

# Option B: SCP from local machine
scp -r ./ClipSyncServer user@your-server:~/clipsync/ClipSyncServer
```

### Step 5: Create `docker-compose.yml`

Create `docker-compose.yml` in `~/clipsync/`:

```yaml
services:
  clipsync-server:
    build:
      context: ./ClipSyncServer
      dockerfile: Dockerfile
    container_name: clipsync-server
    restart: unless-stopped
    environment:
      - JWT_SECRET=${JWT_SECRET}
      - LISTEN_ADDR=:8080
      - DB_PATH=/app/data/clip.db
    volumes:
      - clipsync-data:/app/data
    networks:
      - clipsync-net
    # No ports exposed to host — traffic goes through Cloudflare Tunnel only
    # Uncomment the following line if you need direct access for debugging:
    # ports:
    #   - "8080:8080"

  cloudflared:
    image: cloudflare/cloudflared:latest
    container_name: cloudflared
    restart: unless-stopped
    command: tunnel --no-autoupdate run
    environment:
      - TUNNEL_TOKEN=${TUNNEL_TOKEN}
    networks:
      - clipsync-net
    depends_on:
      - clipsync-server

volumes:
  clipsync-data:
    driver: local

networks:
  clipsync-net:
    driver: bridge
```

### Step 6: Configure Secrets

**Generate a strong JWT secret:**

```bash
openssl rand -base64 32
```

**Create `.env` file** (recommended for production):

```bash
cat > .env << 'EOF'
JWT_SECRET=your-generated-secret-key-here
TUNNEL_TOKEN=eyJhIjoiZmQ0M...your-tunnel-token...
EOF

# Restrict permissions
chmod 600 .env
```

> 💡 Docker Compose automatically reads `.env` in the same directory and substitutes `${VAR}` references.

### Step 7: Build & Launch

```bash
cd ~/clipsync

# Build and start in detached mode
docker compose up -d --build

# Check that both containers are running
docker compose ps
```

Expected output:
```
NAME               STATUS          PORTS
clipsync-server    Up X minutes
cloudflared        Up X minutes
```

### Step 8: Verify

1. **Check container logs:**
   ```bash
   # Server logs
   docker compose logs -f clipsync-server

   # Tunnel logs
   docker compose logs -f cloudflared
   ```

2. **Test the public URL:**
   ```bash
   curl https://clip.yourdomain.com/api/config
   ```
   You should get a JSON response like:
   ```json
   {"allow_registration": true}
   ```

3. **Register your first user:** (First user automatically becomes admin)
   ```bash
   curl -X POST https://clip.yourdomain.com/api/register \
     -H "Content-Type: application/json" \
     -d '{"username":"admin","password":"your-strong-password"}'
   ```

4. **Open the Web dashboard:**  
   Visit `https://clip.yourdomain.com` in your browser.

---

## 🧑‍💻 Development Environment Deployment

If you want to deploy the server for **development/testing** purposes (e.g. on your local Linux machine or a dev VPS), you don't need Cloudflare Tunnel. Here's what needs to change:

### What's Different from Production

| Item | Production | Development |
|---|---|---|
| Cloudflare Tunnel | ✅ Required | ❌ Not needed |
| Port exposed | No (through tunnel only) | Yes (`8080` exposed directly) |
| JWT_SECRET | Strong random key | Can use default or simple key |
| Domain / HTTPS | Required | Not needed (use `http://localhost:8080`) |
| Gin mode | `gin.ReleaseMode` | `gin.DebugMode` (already default in code) |
| `.env` security | `chmod 600` | Not critical |

### Option A: Docker Compose (Dev)

Create a simplified `docker-compose.dev.yml`:

```yaml
services:
  clipsync-server:
    build:
      context: ./ClipSyncServer
      dockerfile: Dockerfile
    container_name: clipsync-server-dev
    restart: unless-stopped
    ports:
      - "8080:8080"        # Expose port directly for dev access
    environment:
      - JWT_SECRET=dev-secret-key-for-testing
      - LISTEN_ADDR=:8080
      - DB_PATH=/app/data/clip.db
    volumes:
      - clipsync-dev-data:/app/data

volumes:
  clipsync-dev-data:
    driver: local
```

Run with:

```bash
docker compose -f docker-compose.dev.yml up -d --build
```

Access at `http://your-server-ip:8080`.

### Option B: Direct Binary (No Docker)

If you just want to run the binary directly on a Linux machine:

```bash
# 1. Install Go 1.23+ and GCC
sudo apt install -y golang gcc

# 2. Clone the repo
git clone https://github.com/youruser/ClipBoardSync.git
cd ClipBoardSync/ClipSyncServer

# 3. Build
go mod tidy
CGO_ENABLED=1 go build -o clipsyncd .

# 4. Run (with optional environment overrides)
JWT_SECRET="dev-secret" LISTEN_ADDR=":8080" ./clipsyncd
```

### Option C: Dev Binary + Cloudflare Tunnel (Public Access for Testing)

If you need public access during development (e.g., testing from your phone):

```bash
# Install cloudflared locally
curl -fsSL https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64 -o /usr/local/bin/cloudflared
chmod +x /usr/local/bin/cloudflared

# Quick tunnel (no configuration needed, gives you a temporary public URL)
cloudflared tunnel --url http://localhost:8080
```

This will output a temporary URL like `https://xxx-yyy-zzz.trycloudflare.com` that you can use to test.

### Key Files to Modify for Dev

If you want to tweak behavior for development:

| What | File | Change |
|---|---|---|
| Enable debug logging | `main.go` line 22 | Already set to `gin.DebugMode` |
| Change listen port | env `LISTEN_ADDR` | Set to `:3000` or any port |
| Use different DB | env `DB_PATH` | e.g. `./data/dev.db` |
| Disable auth for testing | `auth.go` | Comment out `authMiddleware()` in `main.go` (not recommended) |
| Change JWT key | env `JWT_SECRET` | Any string for dev |

---

## 🔧 Operations

### View Logs

```bash
docker compose logs -f              # All services
docker compose logs -f clipsync-server  # Server only
docker compose logs -f cloudflared     # Tunnel only
```

### Restart Services

```bash
docker compose restart              # Restart all
docker compose restart clipsync-server  # Restart server only
```

### Update / Rebuild

```bash
cd ~/clipsync

# Pull latest code changes
git pull  # if using git

# Rebuild and restart
docker compose up -d --build
```

### Backup Data

The SQLite database is stored in a Docker volume. To back up:

```bash
# Find the volume location
docker volume inspect clipsync-data

# Or copy from the running container
docker cp clipsync-server:/app/data/clip.db ./clip-backup-$(date +%Y%m%d).db
```

### Stop Everything

```bash
docker compose down         # Stop and remove containers (data preserved in volume)
docker compose down -v      # Stop, remove containers AND delete data volume ⚠️
```

---

## 🛡️ Security Checklist (Production)

- [ ] Changed `JWT_SECRET` to a strong random string (≥ 32 characters).
- [ ] Cloudflare Tunnel WebSocket support is **enabled**.
- [ ] No ports are exposed in `docker-compose.yml` (except through tunnel).
- [ ] Cloudflare SSL/TLS mode is set to **Full** or **Full (Strict)**.
- [ ] First user registered is the admin — disable open registration after setup via admin API:
  ```bash
  curl -X PUT https://clip.yourdomain.com/api/admin/config \
    -H "Authorization: Bearer <ADMIN_TOKEN>" \
    -H "Content-Type: application/json" \
    -d '{"allow_registration": false}'
  ```
- [ ] `.env` file has restrictive permissions: `chmod 600 .env`
- [ ] Consider enabling Cloudflare **Access** (Zero Trust) for additional auth layer on the Web UI.

---

## ❓ Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `502 Bad Gateway` on domain | Server container not running or not healthy | `docker compose logs clipsync-server` — check for startup errors |
| WebSocket disconnects | Cloudflare WebSocket not enabled | Tunnel config → HTTP Settings → Enable WebSockets |
| `connection refused` in cloudflared logs | Service name mismatch | Ensure tunnel points to `http://clipsync-server:8080` (Docker service name) |
| Database locked errors | Multiple instances writing to SQLite | Ensure only one `clipsync-server` container is running |
| Slow first build | Downloading Go modules in Docker | Normal for first build; subsequent builds use cache |
| Tunnel not connecting | Invalid token | Re-copy the tunnel token from Cloudflare dashboard |

---

## 📁 Final Directory Layout

```
~/clipsync/
├── docker-compose.yml      # Production service orchestration
├── docker-compose.dev.yml  # Development (optional)
├── .env                    # Secrets (JWT_SECRET, TUNNEL_TOKEN)
└── ClipSyncServer/         # Source code
    ├── Dockerfile
    ├── main.go
    ├── handlers.go
    ├── websocket.go
    ├── auth.go
    ├── database.go
    ├── admin.go
    ├── models.go
    ├── frontend.go
    ├── go.mod
    └── go.sum
```
