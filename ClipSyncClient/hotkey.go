package main

import (
	"log"
	"strings"
	"syscall"
	"unsafe"
)

// ============================================================================
// Global Hotkey Registration (Windows only, via user32.dll)
// ============================================================================

var (
	user32                = syscall.NewLazyDLL("user32.dll")
	procRegisterHotKey    = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey  = user32.NewProc("UnregisterHotKey")
	procGetMessageW       = user32.NewProc("GetMessageW")
	procSendInput         = user32.NewProc("SendInput")
)

const (
	modAlt     = 0x0001
	modCtrl    = 0x0002
	modShift   = 0x0004
	modWin     = 0x0008
	wmHotkey   = 0x0312
	hotkeyID   = 1
)

// MSG is the Windows MSG structure for GetMessage
type MSG struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

// ParseHotkey parses a string like "ctrl+shift+v" into (modifiers, vk).
func ParseHotkey(s string) (mod int, vk int, err error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(s)), "+")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		switch p {
		case "ctrl":
			mod |= modCtrl
		case "alt":
			mod |= modAlt
		case "shift":
			mod |= modShift
		case "win":
			mod |= modWin
		default:
			// Single character key
			if len(p) == 1 && p[0] >= 'a' && p[0] <= 'z' {
				vk = int(p[0] - 'a' + 0x41)
			} else {
				// Try common special keys
				switch p {
				case "f1":
					vk = 0x70
				case "f2":
					vk = 0x71
				case "f3":
					vk = 0x72
				case "f4":
					vk = 0x73
				case "f5":
					vk = 0x74
				case "f6":
					vk = 0x75
				case "f7":
					vk = 0x76
				case "f8":
					vk = 0x77
				case "f9":
					vk = 0x78
				case "f10":
					vk = 0x79
				case "f11":
					vk = 0x7A
				case "f12":
					vk = 0x7B
				case "space":
					vk = 0x20
				case "tab":
					vk = 0x09
				default:
					// Numeric keys 0-9
					if len(p) == 1 && p[0] >= '0' && p[0] <= '9' {
						vk = int(p[0])
					}
				}
			}
		}
	}
	return
}

// FormatHotkey returns a human-readable string for modifiers + vk.
func FormatHotkey(mod, vk int) string {
	parts := []string{}
	if mod&modCtrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if mod&modAlt != 0 {
		parts = append(parts, "Alt")
	}
	if mod&modShift != 0 {
		parts = append(parts, "Shift")
	}
	if mod&modWin != 0 {
		parts = append(parts, "Win")
	}
	// Key name
	if vk >= 0x41 && vk <= 0x5A {
		parts = append(parts, string(rune('A'+vk-0x41)))
	} else if vk >= 0x30 && vk <= 0x39 {
		parts = append(parts, string(rune(vk)))
	} else if vk >= 0x70 && vk <= 0x7B {
		parts = append(parts, "F"+string(rune('1'+vk-0x70)))
	} else if vk == 0x20 {
		parts = append(parts, "Space")
	} else if vk == 0x09 {
		parts = append(parts, "Tab")
	}
	return strings.Join(parts, "+")
}

// RegisterGlobalHotkey registers a global hotkey. Returns true on success.
func RegisterGlobalHotkey(mod, vk int) bool {
	ret, _, _ := procRegisterHotKey.Call(0, hotkeyID, uintptr(mod), uintptr(vk))
	return ret != 0
}

// UnregisterGlobalHotkey removes the global hotkey.
func UnregisterGlobalHotkey() {
	procUnregisterHotKey.Call(0, hotkeyID)
}

// ListenHotkey listens for the global hotkey in a blocking loop.
// When triggered, calls onTriggered(). Must run on its own goroutine.
func ListenHotkey(onTriggered func()) {
	var msg MSG
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if ret == 0 {
			break
		}
		if msg.Message == wmHotkey && msg.WParam == hotkeyID {
			log.Println("[hotkey] Global hotkey triggered")
			onTriggered()
		}
	}
}

// SimulateCtrlV sends a Ctrl+V keyboard input using SendInput.
func SimulateCtrlV() {
	type KEYBDINPUT struct {
		Vk        uint16
		Scan      uint16
		Flags     uint32
		Time      uint32
		ExtraInfo uintptr
	}
	type INPUT struct {
		Type uint32
		Ki   KEYBDINPUT
		pad  [8]byte
	}

	const (
		inputKeyboard   = 1
		keyeventfKeyup  = 0x0002
		vkControl       = 0x11
		vkV             = 0x56
	)

	inputs := [4]INPUT{
		{Type: inputKeyboard, Ki: KEYBDINPUT{Vk: vkControl}},
		{Type: inputKeyboard, Ki: KEYBDINPUT{Vk: vkV}},
		{Type: inputKeyboard, Ki: KEYBDINPUT{Vk: vkV, Flags: keyeventfKeyup}},
		{Type: inputKeyboard, Ki: KEYBDINPUT{Vk: vkControl, Flags: keyeventfKeyup}},
	}

	procSendInput.Call(4, uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
}
