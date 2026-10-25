//go:build !windows

package security

// Enforce enables release-only anti-debug checks.
var Enforce = "false"

// DebuggerAttached is a no-op on non-Windows platforms.
func DebuggerAttached() bool { return false }

// Guard is a no-op on non-Windows platforms.
func Guard() error { return nil }
