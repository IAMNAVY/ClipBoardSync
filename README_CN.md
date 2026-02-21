<p align="center">
  <h1 align="center">📋 ClipSync</h1>
  <p align="center">
    <strong>跨设备剪贴板同步 — 即时、安全、自部署。</strong>
  </p>
  <p align="center">
    <a href="#-特性">特性</a> •
    <a href="#-架构">架构</a> •
    <a href="#-快速开始">快速开始</a> •
    <a href="#-组件">组件</a> •
    <a href="./README.md">English</a>
  </p>
</p>

---

## ✨ 特性

- 🔄 **实时同步** — 基于 WebSocket 的亚秒级剪贴板推送，一端复制、另一端即时粘贴。
- 🖥️ **多平台** — Windows 桌面托盘程序、Android 客户端、响应式 Web 管理面板，全部连接同一服务端。
- 🔐 **安全设计** — 无状态 JWT 认证、Bcrypt 密码哈希、用户数据隔离。
- 🪶 **极致轻量** — 空载内存 < 30 MB，前后端打包为单一二进制文件，适合 1 GB 内存 VPS。
- 📱 **Android 深度集成** — 基于无障碍服务在 Android 10+ 后台监控剪贴板，前台服务保活，电池优化豁免引导。
- 🛡️ **智能防循环** — 内置 Anti-loop 机制，杜绝设备间无限同步风暴。
- 📡 **设备管理** — Web 端查看在线设备、远程重命名、强制下线。
- 🗂️ **剪贴板历史** — 每用户最多 50 条记录，FIFO 自动清理，Web 端可查看和删除。
- 🔀 **灵活同步模式** — 双向、仅上传、仅下载、关闭 — 每个客户端独立配置。
- 🐳 **Docker 就绪** — 提供多阶段 Dockerfile，`docker-compose up -d` 一键部署。

---

## 🏗️ 架构

```
┌──────────────────────────────────────────────────────────────────┐
│                         ClipSync Server                          │
│          Go · Gin · GORM · SQLite · Gorilla WebSocket            │
│                                                                  │
│   ┌──────────┐   ┌──────────────┐   ┌────────────────────────┐   │
│   │ REST API │   │ WebSocket Hub│   │  嵌入式 Web 管理面板   │   │
│   └────┬─────┘   └──────┬───────┘   └────────────────────────┘   │
│        │                │                                        │
│        └────────┬───────┘                                        │
│                 │                                                │
│          ┌──────┴──────┐                                         │
│          │  SQLite 数据库│                                        │
│          └─────────────┘                                         │
└──────────────────────────────────────────────────────────────────┘
         ▲  WebSocket + REST           ▲  WebSocket + REST
         │                             │
   ┌─────┴──────────┐          ┌──────┴──────────────┐
   │  ClipSync       │          │  ClipSync Android    │
   │  Windows 客户端 │          │  Kotlin · Compose    │
   │  Go · 托盘程序  │          │  无障碍服务           │
   └─────────────────┘          └─────────────────────┘
```

---

## 🚀 快速开始

> [默认配置] 管理员账号: admin 管理员密码: admin123，登录后请修改密码。

### 1. 部署服务端

最快方式：Docker 一键启动。

```bash
cd ClipSyncServer

docker build -t clipsync-server .

docker run -d \
  --name clipsync-server \
  -p 8080:8080 \
  -v clipsync-data:/app/data \
  -e JWT_SECRET="你的强随机密钥" \
  clipsync-server
```

> 📖 生产环境使用 **Cloudflare Tunnel + Docker Compose** 部署，请参考 [`ClipSyncServer/deploy_CN.md`](./ClipSyncServer/deploy_CN.md)。

### 2. 连接 Windows 客户端

1. 下载或编译 `ClipSyncClient.exe`。
2. 首次启动会弹出配置窗口。
3. 填入服务器地址、用户名和 Token（登录 Web 面板获取）。
4. 配置完成后客户端隐藏到系统托盘，静默同步。

### 3. 连接 Android 客户端

1. 安装 APK 后打开 App，输入服务器地址和登录信息。
2. 按引导开启 **无障碍服务** 和 **电池优化豁免**。
3. 后台服务自动运行，剪贴板实时同步。

---

## 📦 组件

| 组件 | 目录 | 语言 | 说明 |
|---|---|---|---|
| **服务端** | [`ClipSyncServer/`](./ClipSyncServer/) | Go | 后端 API、WebSocket Hub、嵌入式 Web UI、SQLite 存储 |
| **Windows 客户端** | [`ClipSyncClient/`](./ClipSyncClient/) | Go | 系统托盘程序，含 GUI 配置窗口，支持开机自启 |
| **Android 客户端** | [`ClipSyncAndroid/`](./ClipSyncAndroid/) | Kotlin | Jetpack Compose UI、无障碍服务、前台服务 |

每个子项目内有各自的详细 README，含构建说明和使用指南。

---

## 🛠️ 技术栈

| 层级 | 技术 |
|---|---|
| 服务端 | Go 1.23+, Gin, GORM, SQLite (WAL), Gorilla WebSocket |
| 认证 | JWT (HS256), Bcrypt |
| 桌面客户端 | Go, Win32 System Tray, CGO |
| Android 客户端 | Kotlin, Jetpack Compose, OkHttp, AccessibilityService |
| 部署 | Docker, Docker Compose, Cloudflare Tunnel |

---

## 📁 项目结构

```
ClipBoardSync/
├── ClipSyncServer/         # 服务端 (Go)
│   ├── main.go             # 入口 & 路由注册
│   ├── handlers.go         # REST API 处理器
│   ├── websocket.go        # WebSocket Hub & 实时广播
│   ├── auth.go             # JWT 中间件
│   ├── database.go         # GORM + SQLite 初始化
│   ├── admin.go            # 管理员路由处理器
│   ├── models.go           # 数据模型 & DTO
│   ├── frontend.go         # 嵌入式 HTML/CSS/JS 面板
│   ├── Dockerfile          # 多阶段 Docker 构建
│   └── deploy.md           # 生产环境部署指南
├── ClipSyncClient/         # Windows 客户端 (Go)
│   ├── main.go             # 入口，同步生命周期
│   ├── ws.go               # WebSocket 客户端（含断线重连）
│   ├── clipboard.go        # 剪贴板监控 & Anti-loop
│   ├── tray.go             # 系统托盘图标 & 菜单
│   ├── gui.go              # 原生配置窗口
│   ├── config.go           # 持久化配置管理
│   ├── api.go              # HTTP API 调用
│   └── autostart.go        # Windows 注册表开机自启
├── ClipSyncAndroid/        # Android 客户端 (Kotlin)
│   └── app/src/main/java/com/clipsync/android/
│       ├── MainActivity.kt
│       ├── clipboard/      # 剪贴板监控（无障碍、Logcat）
│       ├── data/           # AppConfig、偏好设置
│       ├── network/        # API 客户端、WebSocket 客户端
│       ├── service/        # 前台服务、开机广播接收器
│       └── ui/             # Compose 页面 & 主题
└── README.md               # 英文文档
```

---

## ⚙️ 环境变量（服务端）

| 变量名 | 默认值 | 说明 |
|---|---|---|
| `JWT_SECRET` | `clipboard-sync-secret-key-change-me` | JWT 签名密钥（**生产环境务必更改**） |
| `LISTEN_ADDR` | `:8080` | 监听地址 |
| `DB_PATH` | `./data/clip.db` | SQLite 数据库路径 |

---

## 🧪 接口快速测试

```bash
# 注册
curl -X POST http://localhost:8080/api/register \
  -H "Content-Type: application/json" \
  -d '{"username":"demo","password":"password123"}'

# 登录
curl -X POST http://localhost:8080/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"demo","password":"password123"}'
```
---

## 🤖 AI 开发声明

本项目在开发过程中得到了AI辅助，用于优化架构设计、提升代码质量及加速跨平台实现：

* **Claude 4.6 Opus** (via Antigravity): 辅助核心逻辑设计、Android 无障碍服务集成及 Windows 托盘程序实现。
* **Gemini 3.1 Pro**: 辅助 WebSocket 同步策略制定、文档编写及 Docker 部署优化。

---

## 📄 开源协议

MIT

