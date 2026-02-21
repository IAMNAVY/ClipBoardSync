# ClipSync Server

> ClipSync 生态的自部署后端 — 轻量、实时、开箱即用。

[English](./README.md)

---

## 🌟 核心特性

- **极致轻量** — 空载内存 < 30 MB，在 1 GB 内存 VPS 上流畅运行。
- **单一二进制** — 前端（响应式深色主题 Web 面板）与后端编译为一个可执行文件，无需额外部署。
- **实时同步** — WebSocket Pub/Sub Hub 将剪贴板内容在毫秒内广播到所有已连接设备。
- **安全认证** — 无状态 JWT 令牌、Bcrypt 密码哈希、用户数据完全隔离。
- **历史记录** — 每用户最多保存 50 条剪贴板记录，FIFO 自动清理。
- **设备管理** — Web 端实时查看在线设备，支持远程重命名和强制下线。
- **管理后台** — 首个注册用户自动成为管理员，可管理用户、控制注册开关、重置密码。
- **Docker 支持** — 提供多阶段 Dockerfile，镜像体积精简。

## 🛠️ 技术栈

| 层级 | 技术 |
|---|---|
| Web 框架 | [Gin](https://github.com/gin-gonic/gin) |
| ORM | [GORM](https://gorm.io/) + SQLite 3 (WAL 模式) |
| WebSocket | [Gorilla WebSocket](https://github.com/gorilla/websocket) |
| 认证 | JWT (HS256) + Bcrypt |
| 前端 | 原生 HTML / CSS / JS（通过 Go 字符串字面量嵌入） |

## 📁 源码文件说明

| 文件 | 职责 |
|---|---|
| `main.go` | 入口函数、路由注册、Gin 配置 |
| `handlers.go` | REST API 处理器（注册、登录、剪贴板 CRUD、设备管理、修改密码） |
| `websocket.go` | WebSocket 升级、Hub（注册/注销/广播）、Ping/Pong 保活 |
| `auth.go` | JWT 生成、解析、认证中间件 |
| `database.go` | GORM + SQLite 初始化、历史记录数量限制 |
| `admin.go` | 管理员路由：用户列表、删除用户、重置密码、系统配置 |
| `models.go` | 数据模型（`User`、`ClipEntry`、`SystemSetting`）及请求 DTO |
| `frontend.go` | 嵌入式 HTML/CSS/JS Web 面板 |
| `Dockerfile` | 多阶段构建（golang → alpine） |

## 🚀 快速开始

### Docker（推荐）

```bash
docker build -t clipsync-server .

docker run -d \
  --name clipsync-server \
  -p 8080:8080 \
  -v clipsync-data:/app/data \
  -e JWT_SECRET="你的强随机密钥" \
  clipsync-server
```

### 本地编译

**环境要求**：Go 1.23+、C 编译器（SQLite CGO 需要，Linux 使用 GCC，Windows 使用 TDM-GCC）。

```bash
go mod tidy
CGO_ENABLED=1 go build -o clipsyncd .
./clipsyncd
```

## ⚙️ 环境变量

| 变量名 | 默认值 | 说明 |
|---|---|---|
| `JWT_SECRET` | `clipboard-sync-secret-key-change-me` | JWT 签名密钥（**生产环境务必更改**） |
| `LISTEN_ADDR` | `:8080` | 监听地址（`:8080` = 监听所有网卡的 8080 端口） |
| `DB_PATH` | `./data/clip.db` | SQLite 数据库文件路径 |

## 📡 API 接口参考

### 公开接口

| 方法 | 端点 | 说明 |
|---|---|---|
| `POST` | `/api/register` | 注册新用户 |
| `POST` | `/api/login` | 登录，返回 JWT Token |
| `GET` | `/api/config` | 公开配置（如是否允许注册） |
| `GET` | `/ws?token=...&device_name=...` | WebSocket 连接 |

### 需认证接口（Bearer Token）

| 方法 | 端点 | 说明 |
|---|---|---|
| `POST` | `/api/clipboard` | 推送一条剪贴板记录 |
| `GET` | `/api/clipboard` | 获取剪贴板历史（最新 50 条） |
| `DELETE` | `/api/clipboard/:id` | 删除一条剪贴板记录 |
| `DELETE` | `/api/clipboard/all` | 清空所有历史 |
| `GET` | `/api/devices` | 列出已连接设备 |
| `PUT` | `/api/devices/:id/rename` | 重命名设备 |
| `DELETE` | `/api/devices/:id` | 强制下线设备 |
| `PUT` | `/api/user/password` | 修改密码 |

### 管理员接口（需管理员权限）

| 方法 | 端点 | 说明 |
|---|---|---|
| `GET` | `/api/admin/users` | 列出所有用户 |
| `DELETE` | `/api/admin/users/:id` | 删除用户 |
| `PUT` | `/api/admin/users/:id/password` | 重置用户密码 |
| `PUT` | `/api/admin/config` | 更新系统配置（如注册开关） |

## 🧪 接口测试 (curl)

```bash
# 注册
curl -X POST http://localhost:8080/api/register \
  -H "Content-Type: application/json" \
  -d '{"username":"test","password":"password123"}'

# 登录
curl -X POST http://localhost:8080/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"test","password":"password123"}'

# 推送剪贴板（将 <TOKEN> 替换为实际令牌）
curl -X POST http://localhost:8080/api/clipboard \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <TOKEN>" \
  -d '{"content":"Hello from curl!","device_name":"Terminal"}'

# 获取历史
curl -H "Authorization: Bearer <TOKEN>" \
  http://localhost:8080/api/clipboard
```

## 🐳 部署指南

- **生产环境部署**（Cloudflare Tunnel + Docker Compose）：见 [`deploy_CN.md`](./deploy_CN.md)
- **Production (English)**: see [`deploy.md`](./deploy.md)

## 📄 开源协议

MIT
