# ClipSync Android 客户端

> ClipSync 官方 Android 客户端 — 基于无障碍服务的常驻剪贴板实时同步。

[English](./README.md)

---

## 🌟 核心特性

- **无障碍服务剪贴板监控** — 通过 Android `AccessibilityService` 在 Android 10+ 系统中检测后台剪贴板变更（系统限制了后台剪贴板访问）。
- **Logcat 备用监控** — 基于 logcat 日志解析的次级剪贴板检测机制，提升设备兼容性。
- **实时双向同步** — WebSocket 持久连接，毫秒级跨设备剪贴板推送。
- **前台服务保活** — 常驻通知栏显示连接状态，防止系统杀死后台进程。
- **开机自启** — `BroadcastReceiver` 监听开机广播，自动启动同步服务。
- **电池优化豁免** — App 内一键引导关闭电池优化，确保后台不间断运行。
- **同步模式控制** — 四种模式：双向（↔）、仅上传（→）、仅下载（←）、关闭 — App 内随时切换。
- **智能防循环** — Anti-loop 机制过滤远程写入的剪贴板内容，防止无限同步循环。
- **远程管理响应** — 处理服务端发起的强制下线和远程设备重命名，实时反映变化。
- **本地设备重命名** — 在 App 内直接修改设备显示名称（自动重连以生效）。
- **现代 UI** — 基于 Jetpack Compose 构建，Material 3 自适应主题。

## 🛠️ 技术栈

| 层级 | 技术 |
|---|---|
| 语言 | Kotlin |
| UI | Jetpack Compose + Material 3 |
| 网络 | OkHttp (REST)、OkHttp WebSocket |
| 剪贴板 | AccessibilityService、ClipboardManager、Logcat 监控 |
| 后台 | 前台服务 (Foreground Service)、BroadcastReceiver (BOOT_COMPLETED) |
| 持久化 | DataStore / SharedPreferences，通过 `PrefsManager` 封装 |
| 最低版本 | Android 10 (API 29) |

## 📁 项目结构

```
app/src/main/java/com/clipsync/android/
├── ClipSyncApp.kt                    # Application 类
├── MainActivity.kt                   # 主 Activity：登录/主页路由、生命周期管理
├── clipboard/
│   ├── ClipboardHelper.kt            # 剪贴板读写工具类
│   ├── ClipboardFloatingActivity.kt  # 透明 Activity，用于前台剪贴板访问
│   └── LogcatClipboardMonitor.kt     # 基于 Logcat 的剪贴板变更检测
├── data/
│   ├── AppConfig.kt                  # 配置数据类（serverUrl、token、syncMode 等）
│   └── PrefsManager.kt              # 持久化存储（加密偏好 / DataStore）
├── network/
│   ├── ApiClient.kt                  # HTTP 客户端，用于 REST API 调用（登录、推送等）
│   └── WsClient.kt                  # WebSocket 客户端，含重连、Ping/Pong、消息路由
├── service/
│   ├── ClipSyncService.kt           # 前台服务：协调 WebSocket + 剪贴板监控
│   ├── ClipAccessibilityService.kt  # 无障碍服务，监听剪贴板变更事件
│   └── BootReceiver.kt             # 广播接收器，实现开机自启
└── ui/
    ├── LoginScreen.kt               # Compose 登录页面
    ├── MainScreen.kt                # Compose 主面板（状态、设备、设置）
    └── theme/
        └── Theme.kt                 # Material 3 配色方案 & 排版
```

## 🚀 构建

**环境要求**：
- Android Studio（最新稳定版）
- JDK 17
- 已安装 API 29+ 的 Android SDK

### 步骤

1. 在 Android Studio 中打开 `ClipSyncAndroid` 目录。
2. 等待 Gradle 同步完成。
3. 构建 Debug APK：

```bash
./gradlew assembleDebug
```

APK 输出路径：`app/build/outputs/apk/debug/app-debug.apk`。

## 📖 使用指南

### 登录

1. 启动 App，输入：
   - **服务器地址** — 例如 `https://clip.yourdomain.com`
   - **用户名** 和 **密码**
2. 点击 **登录**，成功后进入主界面。

### 权限设置

登录后，App 会引导你开启以下权限：

| 权限 | 用途 | 开启方式 |
|---|---|---|
| **无障碍服务** | 在 Android 10+ 后台监控剪贴板变化 | 系统设置 → 无障碍 → ClipSync → 开启 |
| **电池优化** | 防止系统杀死同步服务 | App 内一键设置按钮 |
| **通知权限** | 显示前台服务状态通知 | Android 13+ 自动弹窗请求 |
| **自启动** | 开机后自动启动 | 部分厂商需在系统设置中手动添加白名单 |

### 主界面

- **连接状态** — 绿色/红色指示器显示 WebSocket 连接状态。
- **同步模式** — 在双向、仅上传、仅下载、关闭之间切换。
- **设备名称** — 点击可重命名（自动重连生效）。
- **退出登录** — 停止服务、清除认证信息、返回登录页。

### 后台运行模式

同步服务以 Android **前台服务** (Foreground Service) 运行，带有常驻通知。它会：
- 维持 WebSocket 连接。
- 通过无障碍服务检测本地剪贴板变化。
- 将远程同步的剪贴板内容写入本地。
- 在网络变化时自动重连。

## 📄 开源协议

MIT
