package main

import (
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ConfirmElevation records the user's choice on the administrator/root warning.
// It is intentionally not persisted: every elevated launch asks again.
func (a *App) ConfirmElevation(allow bool) error {
	if !allow {
		if a.ctx != nil {
			runtime.Quit(a.ctx)
		}
		return nil
	}
	a.elevationMu.Lock()
	a.elevationConfirmed = true
	a.elevationMu.Unlock()
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "elevation:confirmed", nil)
	}
	return nil
}
