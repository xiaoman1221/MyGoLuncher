//go:build windows

package security

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Enforce enables release-only anti-debug checks. It is set with:
//
//	-ldflags "-X MyMinecraftGoLuncher/internal/security.Enforce=true"
var Enforce = "false"

var (
	kernel32                = windows.NewLazySystemDLL("kernel32.dll")
	procIsDebuggerPresent   = kernel32.NewProc("IsDebuggerPresent")
	procCheckRemoteDebugger = kernel32.NewProc("CheckRemoteDebuggerPresent")
)

// DebuggerAttached reports whether a user-mode debugger is attached.
func DebuggerAttached() bool {
	present, _, _ := procIsDebuggerPresent.Call()
	if present != 0 {
		return true
	}
	var remote int32
	ok, _, _ := procCheckRemoteDebugger.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&remote)))
	return ok != 0 && remote != 0
}

// Guard blocks startup in hardened release builds when a debugger is present.
func Guard() error {
	if Enforce != "true" {
		return nil
	}
	if DebuggerAttached() {
		return errors.New("检测到调试器，程序已停止启动")
	}
	return nil
}
