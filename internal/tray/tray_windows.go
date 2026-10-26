//go:build windows

// Package tray provides a minimal Windows notification-area icon for
// MyGoLuncher. Left-click restores the window; right-click opens a menu with
// Open and Exit.
package tray

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmApp            = 0x8000
	trayCallbackMsg  = wmApp + 1
	trayCloseMsg     = wmApp + 2
	wmDestroy        = 0x0002
	wmLButtonUp      = 0x0202
	wmLButtonDblClk  = 0x0203
	wmRButtonUp      = 0x0205
	wmCommand        = 0x0111
	wmNull           = 0x0000
	nimAdd           = 0x00000000
	nimDelete        = 0x00000002
	nifMessage       = 0x00000001
	nifIcon          = 0x00000002
	nifTip           = 0x00000004
	mfString         = 0x00000000
	mfSeparator      = 0x00000800
	tpmRightButton   = 0x0002
	tpmReturnCommand = 0x0100
	idiApplication   = 32512
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassEx  = user32.NewProc("RegisterClassExW")
	procCreateWindowEx   = user32.NewProc("CreateWindowExW")
	procDefWindowProc    = user32.NewProc("DefWindowProcW")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procGetMessage       = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessage  = user32.NewProc("DispatchMessageW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procPostMessage      = user32.NewProc("PostMessageW")
	procCreatePopupMenu  = user32.NewProc("CreatePopupMenu")
	procAppendMenu       = user32.NewProc("AppendMenuW")
	procTrackPopupMenu   = user32.NewProc("TrackPopupMenu")
	procDestroyMenu      = user32.NewProc("DestroyMenu")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
	procSetForeground    = user32.NewProc("SetForegroundWindow")
	procLoadIcon         = user32.NewProc("LoadIconW")
	procDestroyIcon      = user32.NewProc("DestroyIcon")
	procShellNotifyIcon  = shell32.NewProc("Shell_NotifyIconW")
	procExtractIconEx    = shell32.NewProc("ExtractIconExW")
	procGetModuleHandle  = kernel32.NewProc("GetModuleHandleW")
	activeMu             sync.Mutex
	active               *Tray
)

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

type msg struct {
	HWnd     windows.Handle
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       point
	LPrivate uint32
}

type point struct {
	X int32
	Y int32
}

type notifyIconData struct {
	Size             uint32
	Wnd              windows.Handle
	ID               uint32
	Flags            uint32
	CallbackMessage  uint32
	Icon             windows.Handle
	Tip              [128]uint16
	State            uint32
	StateMask        uint32
	Info             [256]uint16
	TimeoutOrVersion uint32
	InfoTitle        [64]uint16
	InfoFlags        uint32
	GUID             windows.GUID
	BalloonIcon      windows.Handle
}

// Tray owns the notification-area icon and its hidden message window.
type Tray struct {
	hwnd      windows.Handle
	hicon     windows.Handle
	ownedIcon bool
	className *uint16
	onShow    func()
	onExit    func()
	ready     chan error
	done      chan struct{}
	closeOnce sync.Once
}

// New starts the tray icon. onShow is called on left-click; onExit is called
// from the right-click menu.
func New(onShow, onExit func()) (*Tray, error) {
	t := &Tray{
		onShow: onShow,
		onExit: onExit,
		ready:  make(chan error, 1),
		done:   make(chan struct{}),
	}
	go t.run()
	if err := <-t.ready; err != nil {
		return nil, err
	}
	return t, nil
}

func (t *Tray) run() {
	runtime.LockOSThread()
	defer close(t.done)
	classNameText := fmt.Sprintf("MyGoLuncherTray-%d", os.Getpid())
	className, _ := windows.UTF16PtrFromString(classNameText)
	t.className = className
	instance, _, _ := procGetModuleHandle.Call(0)
	wc := wndClassEx{
		Size:      uint32(unsafe.Sizeof(wndClassEx{})),
		WndProc:   syscall.NewCallback(wndProc),
		Instance:  windows.Handle(instance),
		ClassName: className,
	}
	if atom, _, err := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); atom == 0 {
		t.ready <- fmt.Errorf("注册托盘窗口失败: %w", err)
		return
	}
	hwnd, _, err := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(className)),
		0x00000000,
		0, 0, 0, 0,
		0, 0, instance, 0,
	)
	if hwnd == 0 {
		t.ready <- fmt.Errorf("创建托盘窗口失败: %w", err)
		return
	}
	t.hwnd = windows.Handle(hwnd)
	activeMu.Lock()
	active = t
	activeMu.Unlock()
	t.hicon, t.ownedIcon = loadTrayIcon()
	nid := t.notifyData()
	if !notify(nimAdd, nid) {
		t.cleanupWindow()
		t.ready <- errors.New("无法创建系统托盘图标")
		return
	}
	t.ready <- nil

	var m msg
	for {
		ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
	notify(nimDelete, nid)
	if t.ownedIcon && t.hicon != 0 {
		procDestroyIcon.Call(uintptr(t.hicon))
	}
	t.cleanupWindow()
}

func (t *Tray) cleanupWindow() {
	activeMu.Lock()
	if active == t {
		active = nil
	}
	activeMu.Unlock()
	if t.hwnd != 0 {
		procDestroyWindow.Call(uintptr(t.hwnd))
		t.hwnd = 0
	}
}

func (t *Tray) notifyData() *notifyIconData {
	nid := &notifyIconData{
		Flags:           nifMessage | nifIcon | nifTip,
		Wnd:             t.hwnd,
		ID:              1,
		CallbackMessage: trayCallbackMsg,
		Icon:            t.hicon,
	}
	nid.Size = uint32(unsafe.Sizeof(*nid))
	copy(nid.Tip[:], windows.StringToUTF16("MyGoLuncher"))
	return nid
}

func notify(action uintptr, nid *notifyIconData) bool {
	ret, _, _ := procShellNotifyIcon.Call(action, uintptr(unsafe.Pointer(nid)))
	return ret != 0
}

func loadTrayIcon() (windows.Handle, bool) {
	exe, err := os.Executable()
	if err == nil {
		file, _ := windows.UTF16PtrFromString(exe)
		var small windows.Handle
		count, _, _ := procExtractIconEx.Call(uintptr(unsafe.Pointer(file)), 0, 0, uintptr(unsafe.Pointer(&small)), 1)
		if count > 0 && small != 0 {
			return small, true
		}
	}
	icon, _, _ := procLoadIcon.Call(0, idiApplication)
	return windows.Handle(icon), false
}

func (t *Tray) showMenu() {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)
	openText, _ := windows.UTF16PtrFromString("打开 MyGoLuncher")
	exitText, _ := windows.UTF16PtrFromString("退出")
	procAppendMenu.Call(menu, mfString, 1, uintptr(unsafe.Pointer(openText)))
	procAppendMenu.Call(menu, mfSeparator, 0, 0)
	procAppendMenu.Call(menu, mfString, 2, uintptr(unsafe.Pointer(exitText)))
	var cursor point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor)))
	procSetForeground.Call(uintptr(t.hwnd))
	cmd, _, _ := procTrackPopupMenu.Call(
		menu,
		tpmRightButton|tpmReturnCommand,
		uintptr(cursor.X),
		uintptr(cursor.Y),
		0,
		uintptr(t.hwnd),
		0,
	)
	procPostMessage.Call(uintptr(t.hwnd), wmNull, 0, 0)
	switch cmd {
	case 1:
		go t.onShow()
	case 2:
		go t.onExit()
	}
}

func (t *Tray) show() {
	if t.onShow != nil {
		go t.onShow()
	}
}

// Close removes the tray icon and stops its message loop.
func (t *Tray) Close() {
	if t == nil {
		return
	}
	t.closeOnce.Do(func() {
		if t.hwnd != 0 {
			procPostMessage.Call(uintptr(t.hwnd), trayCloseMsg, 0, 0)
		}
	})
	<-t.done
}

func wndProc(hwnd, message, wparam, lparam uintptr) uintptr {
	activeMu.Lock()
	t := active
	activeMu.Unlock()
	if t == nil {
		ret, _, _ := procDefWindowProc.Call(hwnd, message, wparam, lparam)
		return ret
	}
	switch message {
	case trayCallbackMsg:
		switch uint32(lparam) & 0xffff {
		case wmLButtonUp, wmLButtonDblClk:
			t.show()
		case wmRButtonUp:
			t.showMenu()
		}
		return 0
	case wmCommand:
		switch uint32(wparam) & 0xffff {
		case 1:
			t.show()
		case 2:
			go t.onExit()
		}
		return 0
	case trayCloseMsg:
		procDestroyWindow.Call(uintptr(t.hwnd))
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProc.Call(hwnd, message, wparam, lparam)
	return ret
}
