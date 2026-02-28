package main

import (
	"log"
	"time"
	"unsafe"
)

// ============================================================================
// Windows global hotkey registration (Ctrl+Shift+Z for clipboard undo)
// ============================================================================

var (
	procRegisterHotKey   = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey = user32.NewProc("UnregisterHotKey")
	procGetMessageW      = user32.NewProc("GetMessageW")
)

const (
	MOD_CTRL     = 0x0002
	MOD_SHIFT    = 0x0004
	VK_Z         = 0x5A
	WM_HOTKEY    = 0x0312
	HOTKEY_ID    = 1 // our hotkey registration ID
)

type msg struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

var hotkeyStopCh chan struct{}

// StartHotkeyListener registers Ctrl+Shift+Z as a global hotkey and listens for it.
// The callback `onUndo` is called each time the hotkey is pressed.
// This must be called from a goroutine — it blocks until StopHotkeyListener is called.
func StartHotkeyListener(onUndo func()) {
	hotkeyStopCh = make(chan struct{})

	go func() {
		// RegisterHotKey requires a message loop on the same thread that registered it.
		// We use a raw Win32 message loop here.
		ret, _, err := procRegisterHotKey.Call(
			0,                              // NULL HWND = thread-level
			uintptr(HOTKEY_ID),
			uintptr(MOD_CTRL|MOD_SHIFT),
			uintptr(VK_Z),
		)
		if ret == 0 {
			log.Printf("[hotkey] 注册 Ctrl+Shift+Z 失败: %v (可能已被其他程序占用)", err)
			return
		}
		log.Println("[hotkey] 已注册全局热键 Ctrl+Shift+Z (剪贴板撤销)")

		defer func() {
			procUnregisterHotKey.Call(0, uintptr(HOTKEY_ID))
			log.Println("[hotkey] 已注销全局热键")
		}()

		var m msg
		for {
			select {
			case <-hotkeyStopCh:
				return
			default:
			}

			// PeekMessage with PM_REMOVE (0x0001) — non-blocking check
			ret, _, _ := user32.NewProc("PeekMessageW").Call(
				uintptr(unsafe.Pointer(&m)),
				0, 0, 0,
				0x0001, // PM_REMOVE
			)
			if ret == 0 {
				// No message, check stop signal with a short sleep
				select {
				case <-hotkeyStopCh:
					return
				default:
					time.Sleep(50 * time.Millisecond) // 50ms polling interval
				}
				continue
			}

			if m.Message == WM_HOTKEY && m.WParam == uintptr(HOTKEY_ID) {
				log.Println("[hotkey] 检测到 Ctrl+Shift+Z 按下")
				if onUndo != nil {
					go onUndo()
				}
			}
		}
	}()
}

// StopHotkeyListener stops the hotkey listener goroutine.
func StopHotkeyListener() {
	if hotkeyStopCh != nil {
		close(hotkeyStopCh)
	}
}
