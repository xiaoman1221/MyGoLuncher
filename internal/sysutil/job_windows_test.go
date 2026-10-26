//go:build windows

package sysutil

import (
	"os/exec"
	"testing"
	"time"
)

func TestKillOnCloseJobTerminatesChild(t *testing.T) {
	job := NewKillOnCloseJob()
	if job == nil {
		t.Skip("job objects are unavailable")
	}
	cmd := exec.Command("cmd.exe", "/c", "ping", "-n", "30", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := job.Assign(cmd.Process.Pid); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Skipf("could not assign child to job: %v", err)
	}
	if err := job.Close(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("assigned process survived job close")
	}
}
