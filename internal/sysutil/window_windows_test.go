//go:build windows

package sysutil

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestWaitForGameWindowDetectsVisibleWindow(t *testing.T) {
	if os.Getenv("MYGOLUNCHER_LIVE_TEST") == "" {
		t.Skip("set MYGOLUNCHER_LIVE_TEST=1 to run the window detection test")
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-Command",
		"Add-Type -AssemblyName System.Windows.Forms; $f = New-Object System.Windows.Forms.Form; $f.Text = 'MyGoLuncher Test Window'; $f.Width = 320; $f.Height = 200; [System.Windows.Forms.Application]::Run($f)")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	if !WaitForGameWindow(ctx, cmd.Process.Pid) {
		t.Fatal("visible process window was not detected")
	}
}
