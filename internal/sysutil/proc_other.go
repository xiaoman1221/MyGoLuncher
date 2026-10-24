//go:build !windows

package sysutil

import (
	"context"
	"os/exec"
	"syscall"
	"time"
)

// HideWindow is a no-op on platforms without console windows.
func HideWindow(cmd *exec.Cmd) {}

// ConfigureProcess runs the Java process in its own process group so the whole
// game tree can be stopped without affecting the launcher.
func ConfigureProcess(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// WaitForGameWindow is a best-effort fallback on platforms without native
// window enumeration. The launcher hides after a short game-start delay.
func WaitForGameWindow(ctx context.Context, pid int) bool {
	if pid <= 0 {
		return false
	}
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// KillProcessTree terminates the process group.
func KillProcessTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	pid := cmd.Process.Pid
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		return cmd.Process.Kill()
	}
	return nil
}
