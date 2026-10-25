//go:build windows

package security

import "golang.org/x/sys/windows"

// IsElevated reports whether the current process token is elevated (UAC).
// MyGoLuncher never requests elevation itself; this only warns the user when
// the launcher was started from an elevated shell or shortcut.
func IsElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}
