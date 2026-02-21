# ClipSync Android 客户端

ClipSync 官方 Android 客户端，基于 Kotlin + Jetpack Compose 开发，实现与 ClipSyncServer 的实时剪贴板同步。

## ✨ 核心特性

- **无障碍服务剪贴板监控**：通过 Android 无障碍服务 (AccessibilityService) 实现 Android 10+ 后台剪贴板变更监控
- **WebSocket 实时双向同步**：与服务端保持长连接，剪贴板变更秒级同步到所有设备
- **前台服务保活**：常驻通知栏显示连接状态，防止系统杀死后台进程
- **开机自启**：支持开机后自动启动同步服务
- **省电白名单引导**：引导用户关闭电池优化，避免系统限制后台同步
- **智能防死循环**：内置 Anti-loop 机制，远程写入的剪贴板内容不会被重复上报
- **远程管理**：支持服务端强制下线、远程重命名设备
- **本地设备重命名**：可在 App 内修改当前设备名称

## 🛠️ 构建

1. **环境要求**：Android Studio + JDK 17
2. **最低 Android 版本**：Android 10 (API 29)
3. **打开项目**：使用 Android Studio 打开 `ClipSyncAndroid` 目录
4. **编译**：`./gradlew assembleDebug`

## 🚀 使用指南

1. 安装 APK 后打开 App，输入服务器地址、用户名和密码登录
2. 按照 App 内引导开启以下权限：
   - **无障碍服务**：系统设置 → 无障碍 → ClipSync → 开启
   - **忽略电池优化**：App 内一键设置
   - **自启动**：部分厂商需在系统设置中手动允许
3. 登录成功后，服务自动在后台运行，实现剪贴板实时同步

## 📄 开源协议

MIT
