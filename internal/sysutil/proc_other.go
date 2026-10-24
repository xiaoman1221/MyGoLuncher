//go:build !windows

package sysutil

import (
	"os/exec"
	"syscall"
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
