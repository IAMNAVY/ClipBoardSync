package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/atotto/clipboard"
)

// ============================================================================
// Clipboard Panel — runs as a child process (--clipboard-ui)
// ============================================================================

func RunClipboardPanel(cfg *AppConfig) {
	a := app.New()
	w := a.NewWindow("ClipSync 剪贴板")
	w.Resize(fyne.NewSize(500, 560))
	w.CenterOnScreen()

	var (
		searchText string
		filterCat  string
		filterPin  bool
	)

	var refreshUI func()

	refreshUI = func() {
		entries, err := FetchClipboardHistory(cfg.ServerURL, cfg.Token, searchText, filterCat, filterPin)

		// Search bar
		searchEntry := widget.NewEntry()
		searchEntry.SetPlaceHolder("搜索剪贴板内容...")
		searchEntry.SetText(searchText)

		var searchTimer *time.Timer
		searchEntry.OnChanged = func(s string) {
			searchText = s
			if searchTimer != nil {
				searchTimer.Stop()
			}
			searchTimer = time.AfterFunc(400*time.Millisecond, func() {
				refreshUI()
			})
		}

		// Filter buttons
		type flt struct {
			label string
			cat   string
			pin   bool
		}
		filters := []flt{
			{"全部", "", false},
			{"📝 文本", "text", false},
			{"🔗 链接", "url", false},
			{"💻 代码", "code", false},
			{"⭐ 收藏", "", true},
		}
		var fbtns []fyne.CanvasObject
		for _, f := range filters {
			f := f
			active := (f.pin && filterPin) || (!f.pin && !filterPin && filterCat == f.cat)
			btn := widget.NewButton(f.label, func() {
				if f.pin {
					filterPin = !filterPin
					if filterPin {
						filterCat = ""
					}
				} else {
					filterPin = false
					filterCat = f.cat
				}
				refreshUI()
			})
			if active {
				btn.Importance = widget.HighImportance
			}
			fbtns = append(fbtns, btn)
		}

		// Build list
		var rows []fyne.CanvasObject
		if err != nil {
			rows = append(rows, widget.NewLabel("加载失败: "+err.Error()))
		} else if len(entries) == 0 {
			rows = append(rows, widget.NewLabel("暂无记录"))
		} else {
			for _, e := range entries {
				e := e
				rows = append(rows, buildClipRow(e, cfg, w, refreshUI))
			}
		}

		scroll := container.NewVScroll(container.NewVBox(rows...))
		scroll.SetMinSize(fyne.NewSize(480, 400))

		content := container.NewBorder(
			container.NewVBox(searchEntry, container.NewHBox(fbtns...)),
			nil, nil, nil, scroll,
		)
		w.SetContent(content)
	}

	refreshUI()
	w.ShowAndRun()
}

func buildClipRow(e ClipEntryAPI, cfg *AppConfig, w fyne.Window, refresh func()) fyne.CanvasObject {
	emoji := "📝"
	switch e.Category {
	case "url":
		emoji = "🔗"
	case "code":
		emoji = "💻"
	}

	preview := e.Content
	if len(preview) > 120 {
		preview = preview[:120] + "..."
	}
	preview = strings.ReplaceAll(preview, "\n", " ↵ ")

	timeStr := ""
	if t, err := time.Parse(time.RFC3339, e.CreatedAt); err == nil {
		timeStr = t.Local().Format("01-02 15:04")
	}
	devInfo := ""
	if e.DeviceName != "" {
		devInfo = " · " + e.DeviceName
	}

	contentLabel := widget.NewLabel(emoji + " " + preview)
	contentLabel.Wrapping = fyne.TextWrapWord

	metaLabel := widget.NewLabel(timeStr + devInfo)
	metaLabel.TextStyle = fyne.TextStyle{Italic: true}

	// Pin button
	pinText := "☆"
	if e.IsPinned {
		pinText = "⭐"
	}
	pinBtn := widget.NewButtonWithIcon(pinText, theme.ContentAddIcon(), func() {
		TogglePin(cfg.ServerURL, cfg.Token, e.ID)
		refresh()
	})

	// Select button — copies and exits so parent can simulate paste
	selectBtn := widget.NewButtonWithIcon("选择粘贴", theme.ConfirmIcon(), func() {
		clipboard.WriteAll(e.Content)
		log.Printf("[panel] Selected clip %d", e.ID)
		w.Close()
		// Print a marker to stdout so parent knows to simulate paste
		fmt.Println("PASTE:" + fmt.Sprint(e.ID))
	})
	selectBtn.Importance = widget.HighImportance

	// Copy button — just copies without closing
	copyBtn := widget.NewButtonWithIcon("复制", theme.ContentCopyIcon(), func() {
		clipboard.WriteAll(e.Content)
	})

	actions := container.NewHBox(pinBtn, copyBtn, selectBtn)
	row := container.NewVBox(
		contentLabel,
		container.New(layout.NewHBoxLayout(), metaLabel, layout.NewSpacer(), actions),
		widget.NewSeparator(),
	)
	return row
}
