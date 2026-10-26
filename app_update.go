package main

import (
	"context"
	"os"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	licensepkg "MyMinecraftGoLuncher/internal/license"
	"MyMinecraftGoLuncher/internal/updater"
)

func (a *App) configureUpdates() {
	baseURL, apiKey, err := licensepkg.UpdateCredentials()
	if err != nil {
		return
	}
	updater.ConfigureAPI(baseURL, apiKey)
}

// CheckForUpdates fetches the OTA manifest from the LicensingGo product API.
// An unconfigured build is reported as a normal error so Settings can show
// "not configured".
func (a *App) CheckForUpdates() (updater.Info, error) {
	if err := a.baseReady(); err != nil {
		return updater.Info{CurrentVersion: appVersion}, err
	}
	ctx, cancel := context.WithTimeout(a.context(), 30*time.Second)
	defer cancel()
	return updater.Check(ctx, appVersion, updater.CurrentWebVersion(a.paths.Root))
}

// ApplyWebUpdate downloads a signed front-end package, swaps MGL/web and
// reloads the WebView. A broken package can be rolled back from Settings.
func (a *App) ApplyWebUpdate() (string, error) {
	if err := a.baseReady(); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(a.context(), 30*time.Minute)
	defer cancel()
	path, err := updater.ApplyWeb(ctx, a.paths.Root)
	if err != nil {
		return "", err
	}
	if a.ctx != nil {
		runtime.WindowReload(a.ctx)
	}
	return path, nil
}

// ApplyAppUpdate downloads a signed executable and starts the detached
// replacement helper. The caller must quit the current process immediately.
func (a *App) ApplyAppUpdate() (string, error) {
	if err := a.baseReady(); err != nil {
		return "", err
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(a.context(), 30*time.Minute)
	defer cancel()
	script, err := updater.ApplyApp(ctx, a.paths.Root, exe, appVersion)
	if err != nil {
		return "", err
	}
	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
	return script, nil
}

// RollbackWebUpdate restores MGL/web.old after a bad hot update.
func (a *App) RollbackWebUpdate() error {
	if err := a.baseReady(); err != nil {
		return err
	}
	if err := updater.RollbackWeb(a.paths.Root); err != nil {
		return err
	}
	if a.ctx != nil {
		runtime.WindowReload(a.ctx)
	}
	return nil
}
