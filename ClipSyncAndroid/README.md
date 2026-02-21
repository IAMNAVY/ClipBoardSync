# ClipSync Android

> The official Android client for ClipSync — always-on clipboard sync powered by AccessibilityService.

[中文文档](./README_CN.md)

---

## 🌟 Features

- **Accessibility-based Clipboard Monitoring** — Uses Android `AccessibilityService` to detect clipboard changes on Android 10+ where background clipboard access is restricted by the OS.
- **Logcat Fallback Monitor** — Secondary clipboard detection mechanism via logcat parsing for broader compatibility.
- **Real-time Bidirectional Sync** — Persistent WebSocket connection delivers clipboard content across devices within milliseconds.
- **Foreground Service Keep-alive** — Persistent notification displays connection status and prevents system process killing.
- **Boot Auto-start** — `BroadcastReceiver` automatically starts the sync service after device reboot.
- **Battery Optimization Bypass** — In-app one-click guide to disable battery optimization for uninterrupted background operation.
- **Sync Mode Control** — Four modes: Bidirectional (↔), Upload Only (→), Download Only (←), Off — switchable from within the app.
- **Smart Anti-loop** — Filters out remotely-written clipboard content to prevent infinite sync loops.
- **Remote Management** — Handles server-side force-disconnect, remote device renaming, and reflects changes in real-time.
- **Local Device Renaming** — Change the device display name directly from the app (auto-reconnects to apply).
- **Modern UI** — Built with Jetpack Compose, adaptive Material 3 theming.

## 🛠️ Tech Stack

| Layer | Technology |
|---|---|
| Language | Kotlin |
| UI | Jetpack Compose + Material 3 |
| Networking | OkHttp (REST), OkHttp WebSocket |
| Clipboard | AccessibilityService, ClipboardManager, Logcat monitor |
| Background | Foreground Service, BroadcastReceiver (BOOT_COMPLETED) |
| Persistence | DataStore / SharedPreferences via `PrefsManager` |
| Min SDK | Android 10 (API 29) |

## 📁 Project Structure

```
app/src/main/java/com/clipsync/android/
├── ClipSyncApp.kt                    # Application class
├── MainActivity.kt                   # Main activity: login/main screen routing, lifecycle
├── clipboard/
│   ├── ClipboardHelper.kt            # Clipboard read/write utilities
│   ├── ClipboardFloatingActivity.kt  # Transparent activity for foreground clipboard access
│   └── LogcatClipboardMonitor.kt     # Logcat-based clipboard change detection
├── data/
│   ├── AppConfig.kt                  # Configuration data class (serverUrl, token, syncMode, etc.)
│   └── PrefsManager.kt              # Persistent storage (encrypted prefs / DataStore)
├── network/
│   ├── ApiClient.kt                  # HTTP client for REST API calls (login, push, etc.)
│   └── WsClient.kt                  # WebSocket client with reconnect, ping/pong, message routing
├── service/
│   ├── ClipSyncService.kt           # Foreground service: orchestrates WS + clipboard monitoring
│   ├── ClipAccessibilityService.kt  # AccessibilityService for clipboard change events
│   └── BootReceiver.kt             # BroadcastReceiver for auto-start on boot
└── ui/
    ├── LoginScreen.kt               # Compose login screen
    ├── MainScreen.kt                # Compose main dashboard (status, devices, settings)
    └── theme/
        └── Theme.kt                 # Material 3 color scheme & typography
```

## 🚀 Build

**Prerequisites:**
- Android Studio (latest stable)
- JDK 17
- Android SDK with API 29+ installed

### Steps

1. Open the `ClipSyncAndroid` directory in Android Studio.
2. Let Gradle sync complete.
3. Build the debug APK:

```bash
./gradlew assembleDebug
```

The APK is output to `app/build/outputs/apk/debug/app-debug.apk`.

## 📖 Usage

### Login

1. Launch the app and enter:
   - **Server URL** — e.g. `https://clip.yourdomain.com`
   - **Username** and **Password**
2. Tap **Login**. On success, the main screen appears.

### Permissions Setup

After login, the app guides you to enable:

| Permission | Why | How |
|---|---|---|
| **Accessibility Service** | Monitor clipboard changes in background on Android 10+ | System Settings → Accessibility → ClipSync → Enable |
| **Battery Optimization** | Prevent OS from killing the sync service | In-app one-click button |
| **Notification** | Display foreground service status | Android 13+ auto-prompts |
| **Auto-start** | Start on boot | Some OEMs require manual allowlist in settings |

### Main Screen

- **Connection Status** — Green/red indicator for WebSocket state.
- **Sync Mode** — Switch between bidirectional, upload-only, download-only, and off.
- **Device Name** — Tap to rename (reconnects automatically).
- **Logout** — Stops service, clears auth, returns to login.

### Background Behavior

The service runs as an Android **Foreground Service** with a persistent notification. It:
- Maintains the WebSocket connection.
- Detects local clipboard changes via AccessibilityService.
- Writes incoming remote clipboard content locally.
- Auto-reconnects on network changes.

## 📄 License

MIT
