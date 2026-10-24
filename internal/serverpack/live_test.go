package serverpack

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"MyMinecraftGoLuncher/internal/store"
)

// TestLiveFabricExport is opt-in because it downloads the Fabric server
// launcher from the network.
func TestLiveFabricExport(t *testing.T) {
	if os.Getenv("MYGOLUNCHER_LIVE_TEST") == "" {
		t.Skip("set MYGOLUNCHER_LIVE_TEST=1 to run the network export test")
	}
	gameDir := t.TempDir()
	writeTestFile(t, filepath.Join(gameDir, "mods", "example.jar"), "jar")
	writeTestFile(t, filepath.Join(gameDir, "config", "example.toml"), "config")
	dest := filepath.Join(t.TempDir(), "server.zip")
	result, err := Export(context.Background(), Options{
		Instance: store.Instance{
			MCVersion: "1.21.1", Loader: "fabric", LoaderVersion: "0.16.10",
		},
		GameDir:   gameDir,
		Dest:      dest,
		JavaMajor: 21,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Path == "" {
		t.Fatal("export path is empty")
	}
	reader, err := zip.OpenReader(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	found := false
	for _, file := range reader.File {
		if file.Name == "fabric-server-launch.jar" {
			found = true
		}
	}
	if !found {
		t.Fatalf("fabric launcher missing; warnings: %#v", result.Warnings)
	}
}
