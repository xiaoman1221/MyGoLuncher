//go:build !windows

package security

import "os"

// IsElevated reports root on Unix-like systems. The launcher never requests
// elevation; this only drives the warning screen.
func IsElevated() bool {
	return os.Geteuid() == 0
}
