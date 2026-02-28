package main

import (
	"log"
	"syscall"
	"unsafe"
)

// ============================================================================
// Windows Toast notification & sound feedback
// ============================================================================

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	winmm                = syscall.NewLazyDLL("winmm.dll")
	procMessageBoxW      = user32.NewProc("MessageBoxW")
	procPlaySoundW       = winmm.NewProc("PlaySoundW")
	procFindWindowW      = user32.NewProc("FindWindowW")
	procShell_NotifyIcon = syscall.NewLazyDLL("shell32.dll").NewProc("Shell_NotifyIconW")
)

const (
	SND_ALIAS      = 0x00010000
	SND_ASYNC      = 0x0001
	SND_NODEFAULT  = 0x0002
)

// PlaySyncSound plays a short system notification sound asynchronously.
func PlaySyncSound() {
	go func() {
		// Play the Windows "SystemDefault" notification sound
		soundAlias, _ := syscall.UTF16PtrFromString("SystemDefault")
		procPlaySoundW.Call(
			uintptr(unsafe.Pointer(soundAlias)),
			0,
			uintptr(SND_ALIAS|SND_ASYNC|SND_NODEFAULT),
		)
	}()
}

// ShowBalloonTip shows a non-intrusive tooltip notification near the system tray.
// Since we're using getlantern/systray which doesn't expose balloon tips,
// we use a lightweight approach: just log + play sound. The tray icon flash
// serves as the primary visual indicator.
func ShowSyncNotification(content string) {
	// Truncate for preview
	preview := content
	if len(preview) > 60 {
		preview = preview[:60] + "..."
	}
	log.Printf("[notify] 📋 已接收: %s", preview)

	// Play sound
	PlaySyncSound()

	// Flash tray icon
	FlashTrayIcon()
}
