//go:build !windows

package tray

import "errors"

// Tray is unavailable on platforms without the Windows notification area.
type Tray struct{}

func New(onShow, onExit func()) (*Tray, error) {
	return nil, errors.New("system tray is not supported on this platform")
}

func (t *Tray) Close() {}
