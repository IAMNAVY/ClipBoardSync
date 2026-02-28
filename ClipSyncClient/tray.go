package main

import (
	"log"
	"sync"
	"time"

	"github.com/getlantern/systray"
)

// trayCallbacks holds the callbacks for tray menu actions.
type trayCallbacks struct {
	onReconfigure     func()
	onRenameDevice    func()
	onSyncModeChanged func(string)
	onReconnect       func()
	onQuit            func()
}

var (
	trayStatusItem     *systray.MenuItem
	trayDeviceNameItem *systray.MenuItem
	trayAutoStartItem  *systray.MenuItem
	trayCbs            *trayCallbacks
	trayReady          bool
	cachedConnected    bool
	cachedDeviceName   string
	cachedSyncMode     string
	trayMu             sync.Mutex

	// Sync mode menu items
	traySyncBidi     *systray.MenuItem
	traySyncUpload   *systray.MenuItem
	traySyncDownload *systray.MenuItem
	traySyncOff      *systray.MenuItem
)

// StartTray initializes the system tray. This blocks the calling goroutine.
func StartTray(cbs *trayCallbacks) {
	trayCbs = cbs
	systray.Run(onTrayReady, onTrayExit)
}

// UpdateTrayStatus updates the status display in the tray menu.
func UpdateTrayStatus(connected bool) {
	trayMu.Lock()
	cachedConnected = connected
	ready := trayReady
	trayMu.Unlock()

	if !ready || trayStatusItem == nil {
		return
	}
	applyTrayStatus(connected)
}

func applyTrayStatus(connected bool) {
	if connected {
		trayStatusItem.SetTitle("● 已连接 (Connected)")
	} else {
		trayStatusItem.SetTitle("○ 未连接 (Disconnected)")
	}
}

// UpdateTrayDeviceName updates the device name display in the tray menu.
func UpdateTrayDeviceName(name string) {
	trayMu.Lock()
	cachedDeviceName = name
	ready := trayReady
	trayMu.Unlock()

	if !ready || trayDeviceNameItem == nil {
		return
	}
	trayDeviceNameItem.SetTitle("📱 " + name)
	systray.SetTooltip("ClipSync - " + name)
}

// UpdateTraySyncMode updates the sync mode display in the tray menu.
func UpdateTraySyncMode(mode string) {
	trayMu.Lock()
	cachedSyncMode = mode
	ready := trayReady
	trayMu.Unlock()

	if !ready {
		return
	}
	applySyncModeCheck(mode)
}

func onTrayReady() {
	systray.SetIcon(iconBytes)
	systray.SetTitle("ClipSync")
	systray.SetTooltip("ClipSync - 剪贴板同步")

	// Status (clickable — triggers reconnect)
	trayStatusItem = systray.AddMenuItem("○ 未连接 (Disconnected)", "点击重新连接 / Click to reconnect")

	// Device name (disabled — just for display)
	trayDeviceNameItem = systray.AddMenuItem("📱 设备", "当前设备名称")
	trayDeviceNameItem.Disable()

	// Mark tray as ready and apply any cached status
	trayMu.Lock()
	trayReady = true
	status := cachedConnected
	devName := cachedDeviceName
	trayMu.Unlock()
	applyTrayStatus(status)
	if devName != "" {
		trayDeviceNameItem.SetTitle("📱 " + devName)
		systray.SetTooltip("ClipSync - " + devName)
	}

	systray.AddSeparator()

	// Sync mode submenu
	mSyncMode := systray.AddMenuItem("同步方向 (Sync Mode)", "设置同步方向")
	traySyncBidi = mSyncMode.AddSubMenuItemCheckbox("↔ 双向同步 (Bidirectional)", "本机与云端双向同步", true)
	traySyncUpload = mSyncMode.AddSubMenuItemCheckbox("→ 仅上传 (Upload Only)", "仅本机上传到云端", false)
	traySyncDownload = mSyncMode.AddSubMenuItemCheckbox("← 仅下载 (Download Only)", "仅接收云端内容", false)
	traySyncOff = mSyncMode.AddSubMenuItemCheckbox("✕ 关闭同步 (Off)", "暂停所有同步", false)

	// Apply cached sync mode
	trayMu.Lock()
	initMode := cachedSyncMode
	trayMu.Unlock()
	if initMode != "" {
		applySyncModeCheck(initMode)
	}

	// Rename device
	mRenameDevice := systray.AddMenuItem("重命名设备 (Rename)", "修改设备名称")

	// Auto-start toggle
	trayAutoStartItem = systray.AddMenuItemCheckbox(
		"开机启动 (Auto-Start)",
		"设置开机自动启动",
		IsAutoStartEnabled(),
	)

	// Reconfigure
	mReconfigure := systray.AddMenuItem("重新配置 (Reconfigure)", "重新输入服务器信息")

	systray.AddSeparator()

	// Quit
	mQuit := systray.AddMenuItem("退出 (Quit)", "退出程序")

	// Event handling
	go func() {
		for {
			select {
			case <-trayStatusItem.ClickedCh:
				if trayCbs != nil && trayCbs.onReconnect != nil {
					go trayCbs.onReconnect()
				}
			case <-traySyncBidi.ClickedCh:
				handleSyncModeClick(SyncBidirectional)
			case <-traySyncUpload.ClickedCh:
				handleSyncModeClick(SyncUploadOnly)
			case <-traySyncDownload.ClickedCh:
				handleSyncModeClick(SyncDownloadOnly)
			case <-traySyncOff.ClickedCh:
				handleSyncModeClick(SyncOff)
			case <-trayAutoStartItem.ClickedCh:
				if trayAutoStartItem.Checked() {
					trayAutoStartItem.Uncheck()
					if err := DisableAutoStart(); err != nil {
						log.Printf("[tray] 禁用开机启动失败: %v", err)
					}
				} else {
					trayAutoStartItem.Check()
					if err := EnableAutoStart(); err != nil {
						log.Printf("[tray] 启用开机启动失败: %v", err)
					}
				}
			case <-mRenameDevice.ClickedCh:
				if trayCbs != nil && trayCbs.onRenameDevice != nil {
					trayCbs.onRenameDevice()
				}
			case <-mReconfigure.ClickedCh:
				if trayCbs != nil && trayCbs.onReconfigure != nil {
					trayCbs.onReconfigure()
				}
			case <-mQuit.ClickedCh:
				if trayCbs != nil && trayCbs.onQuit != nil {
					trayCbs.onQuit()
				}
				systray.Quit()
			}
		}
	}()
}

func handleSyncModeClick(mode string) {
	applySyncModeCheck(mode)
	if trayCbs != nil && trayCbs.onSyncModeChanged != nil {
		trayCbs.onSyncModeChanged(mode)
	}
}

func applySyncModeCheck(mode string) {
	if traySyncBidi == nil {
		return
	}
	traySyncBidi.Uncheck()
	traySyncUpload.Uncheck()
	traySyncDownload.Uncheck()
	traySyncOff.Uncheck()
	switch mode {
	case SyncBidirectional:
		traySyncBidi.Check()
	case SyncUploadOnly:
		traySyncUpload.Check()
	case SyncDownloadOnly:
		traySyncDownload.Check()
	case SyncOff:
		traySyncOff.Check()
	default:
		traySyncBidi.Check()
	}
}

// FlashTrayIcon briefly changes the tray tooltip to indicate sync activity.
func FlashTrayIcon() {
	trayMu.Lock()
	ready := trayReady
	trayMu.Unlock()
	if !ready {
		return
	}

	go func() {
		systray.SetTooltip("ClipSync - 📋 同步中...")
		time.Sleep(500 * time.Millisecond)
		trayMu.Lock()
		name := cachedDeviceName
		trayMu.Unlock()
		if name != "" {
			systray.SetTooltip("ClipSync - " + name)
		} else {
			systray.SetTooltip("ClipSync - 剪贴板同步")
		}
	}()
}

func onTrayExit() {
	log.Println("[tray] 系统托盘已退出")
}
