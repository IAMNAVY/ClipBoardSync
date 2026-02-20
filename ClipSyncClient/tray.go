package main

import (
	"log"
	"sync"

	"github.com/getlantern/systray"
)

// trayCallbacks holds the callbacks for tray menu actions.
type trayCallbacks struct {
	onReconfigure  func()
	onRenameDevice func()
	onQuit         func()
}

var (
	trayStatusItem     *systray.MenuItem
	trayDeviceNameItem *systray.MenuItem
	trayAutoStartItem  *systray.MenuItem
	trayCbs            *trayCallbacks
	trayReady          bool
	cachedConnected    bool
	cachedDeviceName   string
	trayMu             sync.Mutex
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

func onTrayReady() {
	systray.SetIcon(iconBytes)
	systray.SetTitle("ClipSync")
	systray.SetTooltip("ClipSync - 剪贴板同步")

	// Status (disabled — just for display)
	trayStatusItem = systray.AddMenuItem("○ 未连接 (Disconnected)", "WebSocket 连接状态")
	trayStatusItem.Disable()

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

func onTrayExit() {
	log.Println("[tray] 系统托盘已退出")
}
