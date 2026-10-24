//go:build windows

package sysutil

import (
	"fmt"
	"os/exec"
	"strconv"
	"syscall"
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
