//go:build !windows

package updater

// RunHelperIfRequested is Windows-only; other platforms use front-end hot
// updates and do not replace the running executable automatically.
func RunHelperIfRequested(args []string) bool {
	return false
}
