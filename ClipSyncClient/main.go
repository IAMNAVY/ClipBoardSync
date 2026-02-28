package main

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"strings"
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
			cfg.Password = result.Password
			cfg.Token = result.Token
			cfg.RefreshToken = result.RefreshToken
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
	UpdateTraySyncMode(appConfig.GetSyncMode())

	StartTray(&trayCallbacks{
		onReconfigure:     handleReconfigure,
		onRenameDevice:    handleRenameDevice,
		onSyncModeChanged: handleSyncModeChanged,
		onReconnect:       handleReconnect,
		onQuit:            handleQuit,
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

// startSyncServices launches the clipboard monitor, WebSocket client, and hotkey listener.
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
		d := GetDeviceName(appConfig)
		canUpload := appConfig.ShouldUpload()
		configLock.Unlock()

		if !canUpload {
			log.Println("[main] 上传已禁用，跳过")
			return
		}

		// URL tracking parameter cleanup
		content = CleanTrackingURL(content)

		// Check if WebSocket is connected; if not, enqueue for later
		if wsClient != nil && !wsClient.IsConnected() {
			clipMon.Enqueue(content)
			log.Println("[main] 网络断开，已暂存到离线队列")
			return
		}

		if err := PushClipboard(s, t, content, d); err != nil {
			log.Printf("[main] 上传剪贴板失败: %v", err)
			if strings.Contains(err.Error(), "HTTP 401") || strings.Contains(err.Error(), "401 Unauthorized") {
				log.Println("[main] 检测到 HTTP 401，尝试自动续签 Token...")
				handleTokenRenewal()
				// Retry once with new token
				configLock.Lock()
				newT := appConfig.Token
				configLock.Unlock()
				if newT != "" && newT != t {
					if retryErr := PushClipboard(s, newT, content, d); retryErr != nil {
						log.Printf("[main] 续签后重试上传失败: %v", retryErr)
					} else {
						log.Println("[main] 续签后重试上传成功")
					}
				}
			}
		} else {
			log.Println("[main] 剪贴板内容已上传")
		}
	})
	clipMon.Start()

	// Create WebSocket client
	wsClient = NewWSClient(serverURL, token, devName,
		// onClip: write remote content to local clipboard
		func(content string) {
			configLock.Lock()
			canDownload := appConfig.ShouldDownload()
			configLock.Unlock()

			if !canDownload {
				log.Println("[main] 下载已禁用，跳过远程剪贴板")
				return
			}

			// URL tracking parameter cleanup on received content too
			content = CleanTrackingURL(content)

			clipMon.WriteClipboard(content)

			// Sync feedback: notification + sound + tray flash
			ShowSyncNotification(content)
		},
		// onStatus: update tray icon + flush offline queue on reconnect
		func(connected bool) {
			UpdateTrayStatus(connected)

			// Flush offline queue when connection is restored
			if connected && clipMon != nil {
				go func() {
					configLock.Lock()
					s := appConfig.ServerURL
					t := appConfig.Token
					d := GetDeviceName(appConfig)
					configLock.Unlock()
					clipMon.FlushQueue(func(content string) error {
						return PushClipboard(s, t, content, d)
					})
				}()
			}
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
		// onForceDisconnect: server forced disconnect
		func(reason string) {
			log.Printf("[main] 收到强制下线指令: %s", reason)
			stopSyncServices()
			
			// Wipe local configuration
			configLock.Lock()
			appConfig.Token = ""
			appConfig.Username = ""
			appConfig.Password = ""
			configLock.Unlock()
			SaveConfig(appConfig)

			UpdateTrayStatus(false)
			trayMu.Lock()
			if trayStatusItem != nil {
				trayStatusItem.SetTitle("❌ 强制下线: " + reason)
			}
			trayMu.Unlock()

			// Launch configure window
			go handleReconfigure()
		},
		// onTokenExpired: auto re-login to renew token
		func() {
			handleTokenRenewal()
		},
	)
	wsClient.Start()

	// Start global hotkey listener (Ctrl+Shift+Z for clipboard undo)
	StartHotkeyListener(handleClipboardUndo)
}

// stopSyncServices gracefully stops the clipboard monitor, WebSocket client, and hotkey listener.
func stopSyncServices() {
	StopHotkeyListener()
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

// handleReconnect triggers an immediate reconnection attempt.
func handleReconnect() {
	if wsClient != nil {
		log.Println("[main] 用户手动触发重连")
		wsClient.Reconnect()
	} else {
		log.Println("[main] WebSocket 客户端未初始化，无法重连")
	}
}

// handleTokenRenewal attempts to refresh the access token using the refresh token first,
// falling back to re-login with saved credentials if refresh fails.
func handleTokenRenewal() {
	configLock.Lock()
	serverURL := appConfig.ServerURL
	refreshTok := appConfig.RefreshToken
	username := appConfig.Username
	password := appConfig.Password
	configLock.Unlock()

	if serverURL == "" {
		log.Println("[main] 无法续签 Token: 缺少服务器地址，需要重新配置")
		go handleReconfigure()
		return
	}

	// Strategy 1: Try refresh token (preferred — no password needed)
	if refreshTok != "" {
		log.Println("[main] Token 过期，正在使用 Refresh Token 续签...")
		newAccess, newRefresh, err := RefreshAccessToken(serverURL, refreshTok)
		if err == nil {
			log.Println("[main] Refresh Token 续签成功")
			configLock.Lock()
			appConfig.Token = newAccess
			appConfig.RefreshToken = newRefresh
			configLock.Unlock()
			if err := SaveConfig(appConfig); err != nil {
				log.Printf("[main] 保存续签 Token 失败: %v", err)
			}
			if wsClient != nil {
				wsClient.UpdateToken(newAccess)
			}
			return
		}
		log.Printf("[main] Refresh Token 续签失败: %v，尝试密码登录...", err)
	}

	// Strategy 2: Fall back to password re-login
	if username == "" || password == "" {
		log.Println("[main] 无法续签 Token: 缺少已保存的凭据，需要重新配置")
		go handleReconfigure()
		return
	}

	log.Println("[main] 正在使用密码重新登录...")
	newToken, newRefreshTok, err := Login(serverURL, username, password)
	if err != nil {
		log.Printf("[main] 密码登录失败: %v", err)
		return
	}

	log.Println("[main] 密码登录续签成功")
	configLock.Lock()
	appConfig.Token = newToken
	appConfig.RefreshToken = newRefreshTok
	configLock.Unlock()

	if err := SaveConfig(appConfig); err != nil {
		log.Printf("[main] 保存续签 Token 失败: %v", err)
	}

	// Update the WS client's token so the next reconnect uses the new one
	if wsClient != nil {
		wsClient.UpdateToken(newToken)
	}
}

// handleSyncModeChanged saves the new sync mode to config.
func handleSyncModeChanged(mode string) {
	log.Printf("[main] 同步方向已切换: %s", mode)
	configLock.Lock()
	appConfig.SyncMode = mode
	configLock.Unlock()

	if err := SaveConfig(appConfig); err != nil {
		log.Printf("[main] 保存同步方向失败: %v", err)
	}
}

// handleClipboardUndo fetches the previous clipboard entry from server and writes it to local clipboard.
// This implements the Ctrl+Shift+Z "undo" feature.
func handleClipboardUndo() {
	configLock.Lock()
	s := appConfig.ServerURL
	t := appConfig.Token
	configLock.Unlock()

	if s == "" || t == "" {
		log.Println("[undo] 未配置服务器/未登录，无法撤销")
		return
	}

	entries, err := FetchClipboardHistory(s, t, "", "", false)
	if err != nil {
		log.Printf("[undo] 获取历史记录失败: %v", err)
		return
	}

	// We need at least 2 entries to undo (current + previous)
	if len(entries) < 2 {
		log.Println("[undo] 历史记录不足，无法回退")
		return
	}

	// entries[0] is the most recent, entries[1] is the previous one
	prevContent := entries[1].Content
	if prevContent == "" {
		log.Println("[undo] 上一条记录为空")
		return
	}

	log.Printf("[undo] 回退剪贴板到上一条记录 (%d 字符)", len(prevContent))
	if clipMon != nil {
		clipMon.WriteClipboard(prevContent)
	}

	// Show notification
	ShowSyncNotification("↩ 已回退: " + prevContent)
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


