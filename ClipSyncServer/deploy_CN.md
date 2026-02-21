# ClipSync Server — 生产环境部署指南

> 在 Linux 服务器上使用 **Docker Compose** 部署 ClipSync Server，并通过 **Cloudflare Tunnel** 安全暴露服务（零信任，无需开放端口）。

[English](./deploy.md)

---

## 📋 前置要求

| 项目 | 要求 |
|---|---|
| Linux 服务器 | Ubuntu 20.04+ / Debian 11+ / 任何支持 Docker 的发行版 |
| Docker | 20.10+ |
| Docker Compose | v2（Docker Desktop 自带 或 `docker compose` 插件） |
| 域名 | 已托管在 Cloudflare DNS |
| Cloudflare 账号 | 免费版即可 |

---

## 🏗️ 架构图

```
互联网
   │
   │  HTTPS (443)
   ▼
┌──────────────────────┐
│   Cloudflare 边缘    │  ← SSL 终结、DDoS 防护
│  (Zero Trust Tunnel) │
└──────────┬───────────┘
           │  加密隧道（从你的服务器向外发起的出站连接）
           ▼
┌──────────────────────┐
│   cloudflared        │  ← 隧道守护进程容器
│  (Docker 容器)       │
└──────────┬───────────┘
           │  http://clipsync-server:8080（Docker 内部网络）
           ▼
┌──────────────────────┐
│   ClipSync Server    │  ← 应用容器
│  (Docker 容器)       │
│   端口 8080（内部）  │
└──────────┬───────────┘
           │
           ▼
┌──────────────────────┐
│   SQLite 数据库      │  ← 持久化卷
│   /app/data/clip.db  │
└──────────────────────┘
```

**核心要点**：服务器不向公网暴露任何端口。Cloudflare Tunnel 从你的 VPS 向 Cloudflare 边缘节点发起出站加密连接，再由 Cloudflare 将流量转发给用户。

---

## 📝 分步部署（生产环境）

### 第 1 步：准备服务器

```bash
# 更新系统
sudo apt update && sudo apt upgrade -y

# 安装 Docker（如未安装）
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER

# 注销并重新登录以使用户组变更生效
# 验证 Docker
docker --version
docker compose version
```

### 第 2 步：创建 Cloudflare Tunnel

1. 打开 [Cloudflare Zero Trust 控制面板](https://one.dash.cloudflare.com/)。
2. 导航至 **Networks** → **Tunnels** → **Create a tunnel**。
3. 选择 **Cloudflared** 作为连接器类型。
4. 为隧道命名（例如 `clipsync-tunnel`）。
5. **复制 Tunnel Token** — 下一步需要用到。格式类似：

   ```
   eyJhIjoiZmQ0M...很长的一串字符串...
   ```

6. 在 **Public Hostnames** 中添加一条路由：
   - **Subdomain（子域名）**：`clip`（或你喜欢的名称）
   - **Domain（域名）**：`yourdomain.com`
   - **Service（服务）**：`http://clipsync-server:8080`

   > ⚠️ 服务 URL 必须使用 Docker Compose 的服务名（`clipsync-server`），因为两个容器共享同一个 Docker 网络。

7. 在 **Additional application settings** → **HTTP Settings** 中：
   - 启用 **WebSockets** — 这是实时同步的 **关键**。
   - **HTTP Host Header** 保持默认。

8. 保存隧道配置。

### 第 3 步：创建项目目录

```bash
mkdir -p ~/clipsync && cd ~/clipsync
```

### 第 4 步：上传源代码

将 `ClipSyncServer` 目录传输到服务器：

```bash
# 方式 A：Git 克隆（如果仓库在 GitHub 上）
git clone https://github.com/youruser/ClipBoardSync.git
cd ClipBoardSync/ClipSyncServer

# 方式 B：从本地 SCP 上传
scp -r ./ClipSyncServer user@your-server:~/clipsync/ClipSyncServer
```

### 第 5 步：创建 `docker-compose.yml`

在 `~/clipsync/` 目录下创建 `docker-compose.yml`：

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
    # 不向宿主机暴露端口 — 流量仅通过 Cloudflare Tunnel
    # 如需直接调试访问，取消注释以下行：
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

### 第 6 步：配置密钥

**生成一个强 JWT 密钥**：

```bash
openssl rand -base64 32
```

**创建 `.env` 文件**（生产环境推荐）：

```bash
cat > .env << 'EOF'
JWT_SECRET=你生成的密钥放在这里
TUNNEL_TOKEN=eyJhIjoiZmQ0M...你的隧道令牌...
EOF

# 限制文件权限
chmod 600 .env
```

> 💡 Docker Compose 会自动读取同目录下的 `.env` 文件，并替换 `${VAR}` 引用。

### 第 7 步：构建并启动

```bash
cd ~/clipsync

# 后台构建并启动
docker compose up -d --build

# 检查两个容器是否都在运行
docker compose ps
```

预期输出：
```
NAME               STATUS          PORTS
clipsync-server    Up X minutes
cloudflared        Up X minutes
```

### 第 8 步：验证

1. **查看容器日志**：
   ```bash
   # 服务端日志
   docker compose logs -f clipsync-server

   # 隧道日志
   docker compose logs -f cloudflared
   ```

2. **测试公网 URL**：
   ```bash
   curl https://clip.yourdomain.com/api/config
   ```
   应返回类似以下的 JSON 响应：
   ```json
   {"allow_registration": true}
   ```

3. **注册第一个用户**（首个用户自动成为管理员）：
   ```bash
   curl -X POST https://clip.yourdomain.com/api/register \
     -H "Content-Type: application/json" \
     -d '{"username":"admin","password":"你的强密码"}'
   ```

4. **打开 Web 面板**：  
   在浏览器中访问 `https://clip.yourdomain.com`。

---

## 🧑‍💻 开发环境部署

如果你想在 **开发/测试** 环境下部署（例如本地 Linux 机器或开发 VPS），不需要 Cloudflare Tunnel。以下是需要修改的内容：

### 生产 vs 开发 差异对照

| 项目 | 生产环境 | 开发环境 |
|---|---|---|
| Cloudflare Tunnel | ✅ 需要 | ❌ 不需要 |
| 端口暴露 | 不暴露（通过隧道） | 直接暴露 `8080` 端口 |
| JWT_SECRET | 强随机密钥 | 可使用默认值或简单密钥 |
| 域名 / HTTPS | 需要 | 不需要（使用 `http://localhost:8080`） |
| Gin 模式 | `gin.ReleaseMode` | `gin.DebugMode`（代码中已是默认值） |
| `.env` 安全性 | `chmod 600` | 无需特别关注 |

### 方式 A：Docker Compose（开发版）

创建简化版的 `docker-compose.dev.yml`：

```yaml
services:
  clipsync-server:
    build:
      context: ./ClipSyncServer
      dockerfile: Dockerfile
    container_name: clipsync-server-dev
    restart: unless-stopped
    ports:
      - "8080:8080"        # 直接暴露端口用于开发调试
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

运行命令：

```bash
docker compose -f docker-compose.dev.yml up -d --build
```

访问地址：`http://你的服务器IP:8080`。

### 方式 B：直接运行二进制（不用 Docker）

如果你只想在 Linux 机器上直接运行：

```bash
# 1. 安装 Go 1.23+ 和 GCC
sudo apt install -y golang gcc

# 2. 克隆仓库
git clone https://github.com/youruser/ClipBoardSync.git
cd ClipBoardSync/ClipSyncServer

# 3. 编译
go mod tidy
CGO_ENABLED=1 go build -o clipsyncd .

# 4. 运行（可选覆盖环境变量）
JWT_SECRET="dev-secret" LISTEN_ADDR=":8080" ./clipsyncd
```

### 方式 C：二进制 + Cloudflare 快速隧道（开发环境临时公网访问）

如果开发时需要公网访问（例如用手机测试）：

```bash
# 安装 cloudflared
curl -fsSL https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64 -o /usr/local/bin/cloudflared
chmod +x /usr/local/bin/cloudflared

# 快速隧道（无需配置，给你一个临时公网 URL）
cloudflared tunnel --url http://localhost:8080
```

这会输出一个临时 URL（类似 `https://xxx-yyy-zzz.trycloudflare.com`），可用于测试。

### 开发环境需要修改的关键文件

如果你需要为开发调整行为：

| 修改内容 | 文件 | 说明 |
|---|---|---|
| 启用调试日志 | `main.go` 第 22 行 | 已默认设置为 `gin.DebugMode` |
| 修改监听端口 | 环境变量 `LISTEN_ADDR` | 设为 `:3000` 或任意端口 |
| 使用不同数据库 | 环境变量 `DB_PATH` | 例如 `./data/dev.db` |
| 测试时禁用认证 | `auth.go` | 在 `main.go` 中注释掉 `authMiddleware()`（不推荐） |
| 修改 JWT 密钥 | 环境变量 `JWT_SECRET` | 开发环境可使用任意字符串 |

---

## 🔧 运维操作

### 查看日志

```bash
docker compose logs -f              # 所有服务
docker compose logs -f clipsync-server  # 仅服务端
docker compose logs -f cloudflared     # 仅隧道
```

### 重启服务

```bash
docker compose restart              # 重启所有
docker compose restart clipsync-server  # 仅重启服务端
```

### 更新 / 重新构建

```bash
cd ~/clipsync

# 拉取最新代码
git pull  # 如果使用 git

# 重新构建并启动
docker compose up -d --build
```

### 备份数据

SQLite 数据库存储在 Docker 卷中。备份方法：

```bash
# 查看卷位置
docker volume inspect clipsync-data

# 或从运行中的容器复制
docker cp clipsync-server:/app/data/clip.db ./clip-backup-$(date +%Y%m%d).db
```

### 停止所有服务

```bash
docker compose down         # 停止并移除容器（数据卷保留）
docker compose down -v      # 停止、移除容器并删除数据卷 ⚠️
```

---

## 🛡️ 安全检查清单（生产环境）

- [ ] 已将 `JWT_SECRET` 更改为强随机字符串（≥ 32 个字符）。
- [ ] Cloudflare Tunnel 的 WebSocket 支持已 **启用**。
- [ ] `docker-compose.yml` 中未暴露任何端口（仅通过隧道访问）。
- [ ] Cloudflare 的 SSL/TLS 模式设为 **Full** 或 **Full (Strict)**。
- [ ] 第一个注册用户即管理员 — 设置完成后通过管理员 API 关闭开放注册：
  ```bash
  curl -X PUT https://clip.yourdomain.com/api/admin/config \
    -H "Authorization: Bearer <管理员TOKEN>" \
    -H "Content-Type: application/json" \
    -d '{"allow_registration": false}'
  ```
- [ ] `.env` 文件已设置限制权限：`chmod 600 .env`
- [ ] 考虑启用 Cloudflare **Access**（零信任）为 Web 面板添加额外认证层。

---

## ❓ 常见问题排查

| 现象 | 原因 | 解决方法 |
|---|---|---|
| 域名访问时出现 `502 Bad Gateway` | 服务端容器未运行或不健康 | `docker compose logs clipsync-server` — 检查启动错误 |
| WebSocket 频繁断开 | Cloudflare 未启用 WebSocket | 隧道配置 → HTTP Settings → 启用 WebSockets |
| cloudflared 日志中出现 `connection refused` | 服务名不匹配 | 确保隧道指向 `http://clipsync-server:8080`（Docker 服务名） |
| 数据库锁定错误 | 多个实例同时写入 SQLite | 确保只有一个 `clipsync-server` 容器在运行 |
| 首次构建缓慢 | Docker 中下载 Go 模块 | 首次构建属正常现象，后续构建使用缓存 |
| 隧道无法连接 | Token 无效 | 从 Cloudflare 控制面板重新复制隧道 Token |

---

## 📁 最终目录结构

```
~/clipsync/
├── docker-compose.yml      # 生产环境编排
├── docker-compose.dev.yml  # 开发环境（可选）
├── .env                    # 密钥（JWT_SECRET、TUNNEL_TOKEN）
└── ClipSyncServer/         # 源代码
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
