package main

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// ConfigResult holds the result of the config GUI.
type ConfigResult struct {
	ServerURL    string
	Username     string
	Password     string
	Token        string
	RefreshToken string
}

// ShowConfigGUI displays a Fyne window for server configuration.
// It blocks until the user either successfully logs in or closes the window.
// Returns nil if the user cancels.
func ShowConfigGUI(prefill *AppConfig) *ConfigResult {
	a := app.New()
	w := a.NewWindow("ClipSync - 配置")
	w.Resize(fyne.NewSize(420, 380))
	w.CenterOnScreen()
	w.SetFixedSize(true)

	// Pre-fill from existing config
	serverVal := "http://localhost:8080"
	userVal := ""
	if prefill != nil {
		if prefill.ServerURL != "" {
			serverVal = prefill.ServerURL
		}
		if prefill.Username != "" {
			userVal = prefill.Username
		}
	}

	serverEntry := widget.NewEntry()
	serverEntry.SetPlaceHolder("https://your-server.com")
	serverEntry.SetText(serverVal)

	usernameEntry := widget.NewEntry()
	usernameEntry.SetPlaceHolder("用户名")
	usernameEntry.SetText(userVal)

	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetPlaceHolder("密码")

	statusLabel := widget.NewLabel("")
	statusLabel.Wrapping = fyne.TextWrapWord

	var result *ConfigResult

	connectBtn := widget.NewButton("连接 (Connect)", func() {
		server := strings.TrimSpace(serverEntry.Text)
		username := strings.TrimSpace(usernameEntry.Text)
		password := passwordEntry.Text

		if server == "" || username == "" || password == "" {
			statusLabel.SetText("⚠ 请填写所有字段")
			return
		}

		// Remove trailing slash
		server = strings.TrimRight(server, "/")

		statusLabel.SetText("⏳ 正在连接...")

		// Try login
		go func() {
			accessTok, refreshTok, err := Login(server, username, password)
			if err != nil {
				statusLabel.SetText(fmt.Sprintf("❌ %v", err))
				return
			}

			result = &ConfigResult{
				ServerURL:    server,
				Username:     username,
				Password:     password,
				Token:        accessTok,
				RefreshToken: refreshTok,
			}

			statusLabel.SetText("✅ 登录成功！")

			// Show success and close
			dialog.ShowInformation("成功", "登录成功，开始同步！", w)
			w.Close()
		}()
	})

	form := container.NewVBox(
		widget.NewLabelWithStyle("ClipSync 客户端配置", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		widget.NewLabel("服务器地址 (Server URL):"),
		serverEntry,
		widget.NewLabel("用户名 (Username):"),
		usernameEntry,
		widget.NewLabel("密码 (Password):"),
		passwordEntry,
		layout.NewSpacer(),
		connectBtn,
		container.NewGridWrap(fyne.NewSize(380, 60), container.NewScroll(statusLabel)),
	)

	w.SetContent(container.NewPadded(form))
	w.ShowAndRun()

	return result
}

// ShowRenameGUI displays a simple Fyne window for device renaming.
// Returns the new name, or "" if cancelled.
func ShowRenameGUI(currentName string) string {
	a := app.New()
	w := a.NewWindow("ClipSync - 重命名设备")
	w.Resize(fyne.NewSize(380, 200))
	w.CenterOnScreen()
	w.SetFixedSize(true)

	nameEntry := widget.NewEntry()
	nameEntry.SetText(currentName)

	var newName string

	saveBtn := widget.NewButton("保存 (Save)", func() {
		val := strings.TrimSpace(nameEntry.Text)
		if val != "" {
			newName = val
		}
		w.Close()
	})

	form := container.NewVBox(
		widget.NewLabelWithStyle("重命名设备", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		widget.NewLabel("设备名称 (Device Name):"),
		nameEntry,
		layout.NewSpacer(),
		saveBtn,
	)

	w.SetContent(container.NewPadded(form))
	w.ShowAndRun()

	return newName
}
