//go:build !windows

package secretstore

import (
	"os"
	"path/filepath"
)

// New stores secrets in the user config directory on non-Windows platforms.
func New() (Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = os.TempDir()
	}
	return NewFileStore(filepath.Join(dir, "MyGoLuncher", "secrets"))
}
