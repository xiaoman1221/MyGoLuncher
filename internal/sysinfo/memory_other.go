//go:build !windows

package sysinfo

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// TotalMemoryMB returns installed physical memory.
func TotalMemoryMB() int {
	switch runtime.GOOS {
	case "linux":
		if data, err := os.ReadFile("/proc/meminfo"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "MemTotal:") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						if kb, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
							return int(kb / 1024)
						}
					}
				}
			}
		}
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err == nil {
			if bytes, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64); err == nil {
				return int(bytes / (1024 * 1024))
			}
		}
	}
	return 0
}
