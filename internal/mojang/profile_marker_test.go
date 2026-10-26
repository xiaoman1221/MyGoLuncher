package mojang

import (
	"os"
	"path/filepath"
	"testing"

	"MyMinecraftGoLuncher/internal/store"
	"MyMinecraftGoLuncher/internal/util"
)

func TestMarkProfileInstalled(t *testing.T) {
	paths := store.PathsFor(t.TempDir())
	client := New(paths)
	dir := filepath.Join(paths.Versions, "1.20.1-forge-47.2.0")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "1.20.1-forge-47.2.0.json"), []byte(`{"id":"1.20.1-forge-47.2.0","inheritsFrom":"1.20.1","mainClass":"cpw.mods.bootstraplauncher.BootstrapLauncher"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := client.MarkProfileInstalled("1.20.1-forge-47.2.0"); err != nil {
		t.Fatal(err)
	}
	if !client.IsProfileInstalled("1.20.1-forge-47.2.0") {
		t.Fatal("profile marker was not written")
	}
	var marker installedMarker
	if err := util.ReadJSON(filepath.Join(dir, ".installed.json"), &marker); err != nil {
		t.Fatal(err)
	}
	if !marker.Loader {
		t.Fatalf("expected loader marker: %#v", marker)
	}
}
