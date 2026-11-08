package main

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	apppkg "MyMinecraftGoLuncher/internal/app"
	"MyMinecraftGoLuncher/internal/store"
	updaterpkg "MyMinecraftGoLuncher/internal/updater"
)

//go:embed all:frontend/dist
var assets embed.FS

// overlayFS gives a hot-updated web directory priority and falls back to the
// frontend embedded at build time. Only verified update packages may write to
// this directory.
type overlayFS struct {
	primary  fs.FS
	fallback fs.FS
}

func (o overlayFS) Open(name string) (fs.File, error) {
	if file, err := o.primary.Open(name); err == nil {
		return file, nil
	}
	return o.fallback.Open(name)
}

func webRoot() fs.FS {
	embedded, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		return assets
	}
	paths, err := store.DefaultPaths()
	if err != nil {
		return embedded
	}
	webDir := filepath.Join(paths.Root, "web")
	// Keep the overlay mounted even before the first hot update. os.DirFS
	// returns os.ErrNotExist for a missing web directory, so embedded assets
	// remain the fallback and a later update can be served without restarting.
	return overlayFS{primary: os.DirFS(webDir), fallback: embedded}
}

func main() {
	// The downloaded new executable doubles as the detached update helper.
	// This avoids PowerShell, execution policies and script encoding issues.
	if updaterpkg.RunHelperIfRequested(os.Args[1:]) {
		return
	}
	application := apppkg.NewApp()
	err := wails.Run(&options.App{
		Title:            "MyGoLuncher",
		Width:            1440,
		Height:           900,
		MinWidth:         1120,
		MinHeight:        720,
		Frameless:        true,
		BackgroundColour: &options.RGBA{R: 10, G: 14, B: 12, A: 1},
		AssetServer: &assetserver.Options{
			Assets: webRoot(),
		},
		Windows: &windows.Options{
			Theme: windows.SystemDefault,
		},
		OnStartup:     application.Startup,
		OnBeforeClose: application.BeforeClose,
		OnShutdown:    application.Shutdown,
		Bind: []interface{}{
			application,
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
