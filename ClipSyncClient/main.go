package main

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"sync"
)

var (
	appConfig  *AppConfig
	wsClient   *WSClient
	clipMon    *ClipMonitor
	configLock sync.Mutex
)

func main() {
	// Set up logging to a file in config dir (best-effort)
	setupLogging()

	// Handle out-of-process configuration GUI
	if len(os.Args) > 1 && os.Args[1] == "--config-ui" {
		cfg, _ := LoadConfig()
		if cfg == nil {
			cfg = &AppConfig{}
		}
		result := ShowConfigGUI(cfg)
		if result != nil {
			cfg.ServerURL = result.ServerURL
			cfg.Username = result.Username
			cfg.Token = result.Token
			if err := SaveConfig(cfg); err != nil {
				log.Printf("[config-ui] 保存配置失败: %v", err)
			}
		}
		os.Exit(0)
	}

	// Handle out-of-process rename GUI
	if len(os.Args) > 1 && os.Args[1] == "--rename-ui" {
		currentName := "Desktop Client"
		if len(os.Args) > 2 {
			currentName = os.Args[2]
		}
		newName := ShowRenameGUI(currentName)
		// Output result as JSON to stdout
		result := map[string]string{"new_name": newName}
		json.NewEncoder(os.Stdout).Encode(result)
		os.Exit(0)
	}

	log.Println("========================================")
	log.Println("ClipSyncClient 启动")
	log.Println("========================================")

	// Load saved config
	cfg, err := LoadConfig()
	if err != nil {
		log.Printf("[main] 加载配置失败: %v", err)
		cfg = &AppConfig{}
	}
	appConfig = cfg

	// If no token, show config GUI
	if appConfig.Token == "" || appConfig.ServerURL == "" {
		showConfigAndConnect()
	}

	// If still no token after GUI (user cancelled), exit
	if appConfig.Token == "" || appConfig.ServerURL == "" {
		log.Println("[main] 未完成配置，退出")
		return
	}

	// Start sync services
	startSyncServices()

	// Start system tray (this blocks)
	devName := GetDeviceName(appConfig)
	UpdateTrayDeviceName(devName)

	StartTray(&trayCallbacks{
		onReconfigure:  handleReconfigure,
		onRenameDevice: handleRenameDevice,
		onQuit:         handleQuit,
	})
}

// showConfigAndConnect spawns a child process for the GUI and reloads config on exit.
func showConfigAndConnect() {
	log.Println("[main] 正在启动配置界面(独立进程)...")
	cmd := exec.Command(os.Args[0], "--config-ui")
	err := cmd.Run()
	if err != nil {
		log.Printf("[main] 配置界面异常退出: %v", err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		log.Printf("[main] 重新加载配置失败: %v", err)
		return
	}

	configLock.Lock()
	if cfg != nil {
		appConfig = cfg
	}
	configLock.Unlock()
}

// startSyncServices launches the clipboard monitor and WebSocket client.
func startSyncServices() {
	configLock.Lock()
	serverURL := appConfig.ServerURL
	token := appConfig.Token
	devName := GetDeviceName(appConfig)
	configLock.Unlock()

	// Create clipboard monitor
	clipMon = NewClipMonitor(func(content string) {
		configLock.Lock()
		s := appConfig.ServerURL
		t := appConfig.Token
		configLock.Unlock()

		if err := PushClipboard(s, t, content); err != nil {
			log.Printf("[main] 上传剪贴板失败: %v", err)
		} else {
			log.Println("[main] 剪贴板内容已上传")
		}
	})
	clipMon.Start()

	// Create WebSocket client
	wsClient = NewWSClient(serverURL, token, devName,
		// onClip: write remote content to local clipboard
		func(content string) {
			clipMon.WriteClipboard(content)
		},
		// onStatus: update tray icon
		func(connected bool) {
			UpdateTrayStatus(connected)
		},
		// onDeviceRenamed: server remotely renamed this device
		func(newName string) {
			log.Printf("[main] 设备被远程重命名为: %s", newName)
			configLock.Lock()
			appConfig.DeviceName = newName
			configLock.Unlock()

			if err := SaveConfig(appConfig); err != nil {
				log.Printf("[main] 保存设备名失败: %v", err)
			}
			UpdateTrayDeviceName(newName)
		},
	)
	wsClient.Start()
}

// stopSyncServices gracefully stops the clipboard monitor and WebSocket client.
func stopSyncServices() {
	if clipMon != nil {
		clipMon.Stop()
		clipMon = nil
	}
	if wsClient != nil {
		wsClient.Stop()
		wsClient = nil
	}
}

// handleRenameDevice spawns a child process for the rename GUI and applies the result.
func handleRenameDevice() {
	configLock.Lock()
	currentName := GetDeviceName(appConfig)
	configLock.Unlock()

	log.Println("[main] 正在启动重命名界面(独立进程)...")
	cmd := exec.Command(os.Args[0], "--rename-ui", currentName)
	output, err := cmd.Output()
	if err != nil {
		log.Printf("[main] 重命名界面异常退出: %v", err)
		return
	}

	var result map[string]string
	if err := json.Unmarshal(output, &result); err != nil {
		log.Printf("[main] 解析重命名结果失败: %v", err)
		return
	}

	newName := result["new_name"]
	if newName == "" || newName == currentName {
		log.Println("[main] 设备名未更改")
		return
	}

	log.Printf("[main] 设备重命名: '%s' -> '%s'", currentName, newName)

	configLock.Lock()
	appConfig.DeviceName = newName
	configLock.Unlock()

	if err := SaveConfig(appConfig); err != nil {
		log.Printf("[main] 保存设备名失败: %v", err)
	}

	UpdateTrayDeviceName(newName)

	// Restart WS to apply new device name
	stopSyncServices()
	startSyncServices()
}

// handleReconfigure stops sync, shows the GUI, and restarts sync.
func handleReconfigure() {
	stopSyncServices()
	showConfigAndConnect()
	if appConfig.Token != "" && appConfig.ServerURL != "" {
		startSyncServices()
	}
}

// handleQuit cleans up and exits.
func handleQuit() {
	log.Println("[main] 正在退出...")
	stopSyncServices()
}

// setupLogging configures logging to a file alongside the config.
func setupLogging() {
	dir, err := configDir()
	if err != nil {
		return
	}
	logFile, err := os.OpenFile(dir+`\clipsync.log`, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	log.SetOutput(logFile)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
}
