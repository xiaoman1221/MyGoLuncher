package app

import (
	"context"
	"fmt"
	"os"
	"strings"
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

func (a *App) runningGameNames() []string {
	a.procMu.Lock()
	seen := make(map[string]bool, len(a.procs)+len(a.launching))
	ids := make([]string, 0, len(a.procs)+len(a.launching))
	for id := range a.procs {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for id := range a.launching {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	a.procMu.Unlock()
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		name := id
		if a.store != nil {
			if instance, ok := a.store.Instance(id); ok && strings.TrimSpace(instance.Name) != "" {
				name = instance.Name
			}
		}
		names = append(names, name)
	}
	return names
}

// ensureNoRunningGames blocks replacing launcher files while a Minecraft
// process is still owned by this launcher.
func (a *App) ensureNoRunningGames() error {
	names := a.runningGameNames()
	if len(names) == 0 {
		return nil
	}
	return fmt.Errorf("请先关闭正在运行或启动中的 Minecraft 版本：%s", strings.Join(names, "、"))
}

// CheckForUpdates fetches the OTA manifest from the LicensingGo product API.
// An unconfigured build is reported as a normal error so About can show
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
// reloads the WebView. A broken package can be rolled back from About.
func (a *App) ApplyWebUpdate() (string, error) {
	if err := a.baseReady(); err != nil {
		return "", err
	}
	if err := a.ensureNoRunningGames(); err != nil {
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
	if err := a.ensureNoRunningGames(); err != nil {
		return "", err
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(a.context(), 30*time.Minute)
	defer cancel()
	script, err := updater.ApplyAppWithGuard(ctx, a.paths.Root, exe, appVersion, a.ensureNoRunningGames)
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
