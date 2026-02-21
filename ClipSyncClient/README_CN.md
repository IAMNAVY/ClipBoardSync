# ClipSync Client（桌面客户端）

> ClipSync 官方 Windows 桌面托盘客户端 — 静默运行、无感同步、常驻后台。

[English](./README.md)

---

## 🌟 核心特性

- **系统托盘常驻** — 以通知区域图标静默运行，无界面干扰。
- **原生 GUI 配置** — 内置图形化配置窗口，可直接设置服务器地址、账号和 Token，无需手动编辑配置文件。
- **实时双向同步** — WebSocket 持久连接 + 自动断线重连，亚秒级剪贴板推送。
- **同步模式控制** — 四种模式：双向、仅上传、仅下载、关闭 — 托盘菜单一键切换。
- **开机自启** — 一键添加/移除 Windows 启动项（通过注册表）。
- **智能防循环** — 剪贴板读写防护机制，杜绝设备间无限同步风暴。
- **设备命名** — 为当前电脑设置易识别名称（在 Web 面板中显示）。
- **远程管理响应** — 支持服务端发起的强制下线和远程重命名指令。
- **安全存储** — 凭证以 Base64 混淆存储在 `%APPDATA%\ClipSyncClient\config.json`。

## 🛠️ 技术栈

| 组件 | 技术 |
|---|---|
| 语言 | Go 1.23+ |
| WebSocket | Gorilla WebSocket |
| 剪贴板 | `golang.design/x/clipboard`（CGO） |
| 系统托盘 | `fyne.io/systray` |
| GUI | `github.com/nickolai-kolin/dlgs` |
| 开机自启 | Windows 注册表，通过 `golang.org/x/sys/windows/registry` |

## 📁 源码文件说明

| 文件 | 职责 |
|---|---|
| `main.go` | 入口函数、生命周期管理（配置 → 同步 → 托盘）、子进程 GUI 调用 |
| `ws.go` | WebSocket 客户端：连接、重连、Ping/Pong、消息路由（clip / force\_disconnect / device\_renamed） |
| `clipboard.go` | 剪贴板监控协程，含 Anti-loop 过滤 |
| `tray.go` | 系统托盘图标、菜单项、状态更新、同步模式子菜单 |
| `gui.go` | 原生配置窗口和重命名对话框（以子进程方式启动） |
| `config.go` | `AppConfig` 结构体、Base64 编码加载/保存、同步模式辅助方法 |
| `api.go` | 调用服务端 REST API（推送剪贴板等） |
| `autostart.go` | Windows 启动项注册表管理 |
| `icon.go` | 嵌入式托盘图标数据 |

## 🚀 编译

**环境要求**：
- Go 1.23+
- 支持 CGO 的 C 编译器（例如 [TDM-GCC](https://jmeubank.github.io/tdm-gcc/) 或 [MSYS2 MinGW-w64](https://www.msys2.org/)）

```bash
# 安装依赖
go mod tidy

# 编译（隐藏 Windows 控制台窗口）
go build -ldflags="-H windowsgui" -o ClipSyncClient.exe
```

## 📖 使用指南

### 首次启动

1. 双击运行 `ClipSyncClient.exe`。
2. 弹出 **配置窗口**，填入：
   - **服务器地址** — 例如 `http://your-server:8080` 或 `https://clip.yourdomain.com`
   - **用户名** — 你的 ClipSync 账号
   - **Token** — 在 Web 面板登录后获取
3. 点击确定，客户端连接成功后隐藏到系统托盘。

### 托盘菜单

右键托盘图标可访问：

| 操作 | 说明 |
|---|---|
| **连接状态** | 显示当前状态（✅ 已连接 / ❌ 未连接） |
| **设备名称** | 当前设备名 |
| **同步模式** | 在 双向 / 仅上传 / 仅下载 / 关闭 之间切换 |
| **重命名设备** | 弹出对话框修改当前设备显示名称 |
| **开机自启** | 启用/禁用 Windows 开机启动 |
| **重新配置** | 重新打开配置窗口（暂停同步后重启） |
| **退出** | 完全关闭客户端 |

### 配置文件位置

```
%APPDATA%\ClipSyncClient\config.json   （Base64 编码）
%APPDATA%\ClipSyncClient\clipsync.log  （运行日志）
```

## 📄 开源协议

MIT
