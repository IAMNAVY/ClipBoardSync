package main

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// ============================================================================
// Hotkey Config GUI — runs as child process (--hotkey-ui)
// User types a hotkey combo like "ctrl+shift+v", we validate and check conflicts.
// ============================================================================

func RunHotkeyConfigUI(currentHotkey string) {
	a := app.New()
	w := a.NewWindow("ClipSync - 配置快捷键")
	w.Resize(fyne.NewSize(420, 260))
	w.CenterOnScreen()

	if currentHotkey == "" {
		currentHotkey = "ctrl+shift+v"
	}

	currentLabel := widget.NewLabel("当前快捷键: " + currentHotkey)
	currentLabel.TextStyle = fyne.TextStyle{Bold: true}

	hintLabel := widget.NewLabel("输入新的快捷键组合，例如:\nctrl+shift+v / alt+c / ctrl+alt+f1")
	hintLabel.Wrapping = fyne.TextWrapWord

	resultLabel := widget.NewLabel("")
	resultLabel.TextStyle = fyne.TextStyle{Bold: true}

	hotkeyEntry := widget.NewEntry()
	hotkeyEntry.SetText(currentHotkey)
	hotkeyEntry.SetPlaceHolder("ctrl+shift+v")

	// Live validation on text change
	hotkeyEntry.OnChanged = func(s string) {
		mod, vk, _ := ParseHotkey(s)
		if vk == 0 {
			resultLabel.SetText("⚠ 格式无效")
		} else {
			resultLabel.SetText("→ " + FormatHotkey(mod, vk))
		}
	}

	saveBtn := widget.NewButton("保存", func() {
		newHotkey := hotkeyEntry.Text
		if newHotkey == "" {
			resultLabel.SetText("⚠ 请输入快捷键")
			return
		}

		mod, vk, _ := ParseHotkey(newHotkey)
		if vk == 0 {
			resultLabel.SetText("⚠ 无效的快捷键格式")
			return
		}

		// Test for conflict by trying to register
		if RegisterGlobalHotkey(mod, vk) {
			UnregisterGlobalHotkey()
			resultLabel.SetText("✅ 快捷键可用!")
			fmt.Println("HOTKEY:" + newHotkey)
			w.Close()
		} else {
			resultLabel.SetText("⚠ 快捷键冲突！已被其他程序占用")
		}
	})
	saveBtn.Importance = widget.HighImportance

	cancelBtn := widget.NewButton("取消", func() {
		w.Close()
	})

	content := container.NewVBox(
		currentLabel,
		widget.NewSeparator(),
		hintLabel,
		hotkeyEntry,
		resultLabel,
		container.NewHBox(cancelBtn, saveBtn),
	)

	w.SetContent(content)
	w.ShowAndRun()
}
