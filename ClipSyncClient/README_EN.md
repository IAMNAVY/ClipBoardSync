# ClipSync Client (Desktop App)

The official lightweight Windows desktop client for ClipSync. Written in Go, it seamlessly syncs your clipboard in real-time with the ClipSyncServer.

## ✨ Features

- **Silent Background Execution**: Runs unobtrusively in the system tray.
- **Built-in GUI Engine**: Includes a native graphical interface to configure server URL, username, and Token out of the box.
- **Real-time Bidirectional Sync**: Maintains a low-latency WebSocket connection for sub-second clipboard synchronization.
- **Auto-run on Boot**: Single-click toggle to add the client to Windows startup via Registry.
- **Smart Anti-loop Protection**: Advanced clipboard algorithms to prevent infinite sync loops when pasting content locally.
- **Remote Management**: Responds to server-side force-disconnects and handles remote device renaming initiated from the Web dashboard.

## 🛠️ Build Instructions

Ensure you are building this for Windows.

1. **Prerequisites**: Go 1.23+ installed.
2. **C Compiler requirement**: CGO is required for clipboard operations and system tray features (e.g., TDM-GCC or MSYS2 MinGW-w64).
3. **Install Dependencies**:
   ```bash
   go mod tidy
   ```
4. **Build the binary without console window**:
   ```bash
   go build -ldflags="-H windowsgui" -o ClipSyncClient.exe
   ```

## 🚀 Usage Guide

1. Double-click `ClipSyncClient.exe` to run.
2. If it's your first time, a **Configuration Window** will prompt you to enter:
   - **Server URL** (e.g., `http://your-server-ip:8080`)
   - **Username** and **Token** (Obtained from your Web dashboard after logging in)
3. Once connected, it hides into your Windows system tray (bottom-right area).
4. **Tray Menu actions**:
   - **Rename Device**: Set a recognizable name for this PC.
   - **Start on boot**: Toggle Windows auto-start.
   - **Reconfigure**: Update the server address/token.
   - **Exit**

## 📄 License

MIT
