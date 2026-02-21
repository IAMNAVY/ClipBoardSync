# ClipSync Client

> The official Windows desktop tray application for ClipSync — silent, seamless, always-on clipboard sync.

[中文文档](./README_CN.md)

---

## 🌟 Features

- **System Tray Residence** — Runs silently as a tray icon in the notification area; zero UI clutter.
- **Native GUI Configuration** — Built-in graphical window for server URL, account, and token setup — no config files to edit manually.
- **Real-time Bidirectional Sync** — Maintains a persistent WebSocket connection with automatic reconnection; sub-second clipboard delivery.
- **Sync Mode Control** — Four modes: bidirectional, upload-only, download-only, and off — switchable from the tray menu.
- **Boot Auto-start** — One-click toggle to add/remove from Windows startup via Registry.
- **Smart Anti-loop** — Clipboard write/read guards prevent infinite sync storms between devices.
- **Device Naming** — Set a friendly name for this PC (shown in the Web dashboard).
- **Remote Management** — Responds to server-initiated force-disconnect and remote rename commands.
- **Secure Config** — Credentials are stored locally with Base64 obfuscation in `%APPDATA%\ClipSyncClient\config.json`.

## 🛠️ Tech Stack

| Component | Technology |
|---|---|
| Language | Go 1.23+ |
| WebSocket | Gorilla WebSocket |
| Clipboard | `golang.design/x/clipboard` (CGO) |
| System Tray | `fyne.io/systray` |
| GUI | `github.com/nickolai-kolin/dlgs` |
| Auto-start | Windows Registry via `golang.org/x/sys/windows/registry` |

## 📁 Source Files

| File | Responsibility |
|---|---|
| `main.go` | Entry point, lifecycle management (config → sync → tray), child-process GUI spawning |
| `ws.go` | WebSocket client: connect, reconnect, ping/pong, message routing (clip / force\_disconnect / device\_renamed) |
| `clipboard.go` | Clipboard monitor goroutine with anti-loop filtering |
| `tray.go` | System tray icon, menu items, status updates, sync mode submenu |
| `gui.go` | Native configuration & rename dialog windows (spawned as child processes) |
| `config.go` | `AppConfig` struct, load/save with Base64 encoding, sync mode helpers |
| `api.go` | HTTP calls to server REST API (push clipboard, etc.) |
| `autostart.go` | Windows startup registry key management |
| `icon.go` | Embedded tray icon data |

## 🚀 Build

**Prerequisites:**
- Go 1.23+
- C compiler with CGO support (e.g., [TDM-GCC](https://jmeubank.github.io/tdm-gcc/) or [MSYS2 MinGW-w64](https://www.msys2.org/))

```bash
# Install dependencies
go mod tidy

# Build (hides console window on Windows)
go build -ldflags="-H windowsgui" -o ClipSyncClient.exe
```

## 📖 Usage

### First Launch

1. Double-click `ClipSyncClient.exe`.
2. A **Configuration Window** appears — enter:
   - **Server URL** — e.g. `http://your-server:8080` or `https://clip.yourdomain.com`
   - **Username** — your ClipSync account
   - **Token** — obtain from the Web dashboard after logging in
3. Click OK. The client connects and hides to the system tray.

### Tray Menu

Right-click the tray icon to access:

| Action | Description |
|---|---|
| **Status** | Shows connection status (✅ Connected / ❌ Disconnected) |
| **Device Name** | Displays current device name |
| **Sync Mode** | Switch between Bidirectional / Upload Only / Download Only / Off |
| **Rename Device** | Opens a dialog to change this device's display name |
| **Auto Start** | Toggle Windows startup on/off |
| **Reconfigure** | Re-open the configuration window (stops sync, then restarts) |
| **Exit** | Cleanly shut down the client |

### Config File Location

```
%APPDATA%\ClipSyncClient\config.json   (Base64 encoded)
%APPDATA%\ClipSyncClient\clipsync.log  (Runtime log)
```

## 📄 License

MIT
