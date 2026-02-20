# ClipSync - 跨设备剪贴板实时同步工具

一个基于 Go 语言编写的轻量级、高性能剪贴板同步服务。专为 1GB 内存 VPS 优化，支持实时 WebSocket 广播、JWT 认证和 SQLite 历史存储。

## 🌟 核心特性

- **极致轻量**：空载内存占用 < 30MB，非常适合低端 VPS。
- **实时同步**：基于 WebSocket 的 Pub/Sub 机制，剪贴板内容在秒级内推送到所有连接设备。
- **安全保障**：使用 JWT 进行身份验证，密码经过 Bcrypt 加密存储。
- **历史记录**：支持每个用户最多 50 条剪贴板历史，采用 FIFO（先进先出）自动清理机制。
- **开箱即用**：前后端完全集成在单个二进制文件中，包含一个现代感的响应式深色主题 Web 界面。
- **Docker 支持**：提供多阶段构建的 Docker 镜像，体积更小。

## 🛠️ 技术栈

- **后端**: Go (1.23+), Gin (Web 框架), GORM (SQLite ORM)
- **认证**: JWT (JSON Web Token)
- **通讯**: Gorilla WebSocket
- **数据库**: SQLite 3 (WAL 模式优化)
- **前端**: 原生 HTML/JS/CSS (嵌入在 Go 二进制中)

## 🚀 快速开始

### 方式 1：使用 Docker (推荐)

```bash
# 构建镜像
docker build -t clipsync .

# 运行容器
docker run -d \
  --name clipsync \
  -p 8080:8080 \
  -v clipsync-data:/app/data \
  -e JWT_SECRET="你的强随机密钥" \
  clipsync
```

### 方式 2：本地编译运行

1. **环境准备**：安装 Go 1.23+ 和 C 编译器（用于 SQLite CGO）。
2. **下载依赖**：
   ```bash
   go mod tidy
   ```
3. **编译并运行**：
   ```bash
   go build -o clipsyncd .
   ./clipsyncd
   ```

## ⚙️ 环境变量

| 变量名 | 默认值 | 说明 |
|---|---|---|
| `JWT_SECRET` | `clipboard-sync-secret-key-change-me` | JWT 签名密钥（生产环境务必更改） |
| `LISTEN_ADDR` | `:8080` | 服务监听地址 |
| `DB_PATH` | `./data/clip.db` | SQLite 数据库文件路径 |

## 🧪 接口测试 (curl)

1. **注册**：
   ```bash
   curl -X POST http://localhost:8080/api/register -d '{"username":"test","password":"password123"}' -H "Content-Type: application/json"
   ```
2. **登录并获取 Token**：
   ```bash
   curl -X POST http://localhost:8080/api/login -d '{"username":"test","password":"password123"}' -H "Content-Type: application/json"
   ```

## 📄 开源协议

MIT
