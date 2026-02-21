# ClipSync Android Client

The official Android client for ClipSync, built with Kotlin and Jetpack Compose for real-time clipboard synchronization with ClipSyncServer.

## ✨ Features

- **Accessibility-based Clipboard Monitoring**: Uses Android AccessibilityService to monitor clipboard changes on Android 10+ where background clipboard access is restricted
- **Real-time Bidirectional Sync**: Maintains a persistent WebSocket connection for sub-second clipboard synchronization
- **Foreground Service Keep-alive**: Persistent notification displaying connection status, preventing the system from killing the background process
- **Boot Auto-start**: Automatically starts the sync service on device boot
- **Battery Optimization Bypass**: Guides users to disable battery optimization for uninterrupted background sync
- **Smart Anti-loop Protection**: Prevents infinite sync loops by filtering out remotely-written clipboard content
- **Remote Management**: Responds to server-side force-disconnect and remote device renaming
- **Local Device Renaming**: Rename the current device directly from within the app

## 🛠️ Build

1. **Requirements**: Android Studio + JDK 17
2. **Minimum Android Version**: Android 10 (API 29)
3. **Open Project**: Open the `ClipSyncAndroid` directory in Android Studio
4. **Build**: `./gradlew assembleDebug`

## 🚀 Usage

1. Install the APK and launch the app. Enter your server URL, username, and password to log in
2. Follow the in-app guides to enable required permissions:
   - **Accessibility Service**: System Settings → Accessibility → ClipSync → Enable
   - **Ignore Battery Optimization**: One-click setup within the app
   - **Auto-start**: Some manufacturers require manual setup in system settings
3. After login, the service runs in the background for real-time clipboard sync

## 📄 License

MIT
