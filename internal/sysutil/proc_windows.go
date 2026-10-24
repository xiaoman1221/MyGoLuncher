//go:build windows

package sysutil

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// HideWindow prevents console windows from flashing while launching helpers.
func HideWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
}

// ConfigureProcess prepares a process for clean tree termination.
func ConfigureProcess(cmd *exec.Cmd) {
	HideWindow(cmd)
}

var (
	user32                    = syscall.NewLazyDLL("user32.dll")
	enumWindowsProc           = user32.NewProc("EnumWindows")
	getWindowThreadProcessID  = user32.NewProc("GetWindowThreadProcessId")
	isWindowVisibleProc       = user32.NewProc("IsWindowVisible")
	getWindowProc             = user32.NewProc("GetWindow")
	getWindowRectProc         = user32.NewProc("GetWindowRect")
	windowCheckMu             sync.Mutex
	windowCheckTargetPID      uint32
	windowCheckFound          bool
	enumWindowsCallbackHandle = syscall.NewCallback(enumWindowsCallback)
)

const gwOwner = 4

type winRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

func enumWindowsCallback(hwnd uintptr, _ uintptr) uintptr {
	// This callback runs synchronously inside EnumWindows while windowCheckMu
	// is held by the caller, so the package-level target is safe to read.
	if windowBelongsToProcess(hwnd, windowCheckTargetPID) {
		windowCheckFound = true
		return 0 // stop enumeration
	}
	return 1
}

func windowBelongsToProcess(hwnd uintptr, pid uint32) bool {
	var ownerPID uint32
	getWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&ownerPID)))
	if ownerPID != pid {
		return false
	}
	visible, _, _ := isWindowVisibleProc.Call(hwnd)
	if visible == 0 {
		return false
	}
	owner, _, _ := getWindowProc.Call(hwnd, gwOwner)
	if owner != 0 {
		return false
	}
	var rect winRect
	getWindowRectProc.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	return rect.Right-rect.Left >= 100 && rect.Bottom-rect.Top >= 100
}

func hasProcessWindow(pid uint32) bool {
	windowCheckMu.Lock()
	defer windowCheckMu.Unlock()
	windowCheckTargetPID = pid
	windowCheckFound = false
	enumWindowsProc.Call(enumWindowsCallbackHandle, 0)
	return windowCheckFound
}

// WaitForGameWindow waits until the process owns a visible top-level window.
// It returns false when the context is cancelled before a window appears.
func WaitForGameWindow(ctx context.Context, pid int) bool {
	if pid <= 0 {
		return false
	}
	check := func() bool {
		select {
		case <-ctx.Done():
			return false
		default:
			return hasProcessWindow(uint32(pid))
		}
	}
	if check() {
		return true
	}
	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			if check() {
				return true
			}
		}
	}
}

// KillProcessTree force-terminates a process and all children.
func KillProcessTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	pid := strconv.Itoa(cmd.Process.Pid)
	kill := exec.Command("taskkill", "/PID", pid, "/T", "/F")
	HideWindow(kill)
	if err := kill.Run(); err != nil {
		if err := cmd.Process.Kill(); err != nil {
			return fmt.Errorf("结束进程失败: %w", err)
		}
	}
	return nil
}
