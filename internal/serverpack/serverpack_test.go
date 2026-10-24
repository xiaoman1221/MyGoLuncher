package serverpack

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"MyMinecraftGoLuncher/internal/store"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWritePackIncludesContentAndRuntime(t *testing.T) {
	gameDir := t.TempDir()
	writeTestFile(t, filepath.Join(gameDir, "mods", "example.jar"), "jar")
	writeTestFile(t, filepath.Join(gameDir, "config", "example.toml"), "config")
	writeTestFile(t, filepath.Join(gameDir, "saves", "world", "level.dat"), "save")
	writeTestFile(t, filepath.Join(gameDir, "server.properties"), "server-port=25566\n")
	writeTestFile(t, filepath.Join(gameDir, "eula.txt"), "eula=true\n")

	runtimePath := filepath.Join(t.TempDir(), "fabric-server-launch.jar")
	writeTestFile(t, runtimePath, "runtime")

	zipPath := filepath.Join(t.TempDir(), "server.zip")
	out, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(out)
	err = writePack(zw, Options{
		Instance: store.Instance{
			MCVersion: "1.21.1", Loader: "fabric", LoaderVersion: "0.16.10",
		},
		GameDir:   gameDir,
		JavaMajor: 21,
	}, runtimePath, "fabric-server-launch.jar", runtimeFabric, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}

	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	files := map[string]string{}
	for _, file := range reader.File {
		handle, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(handle)
		handle.Close()
		if err != nil {
			t.Fatal(err)
		}
		files[file.Name] = string(data)
	}
	for _, name := range []string{
		"mods/example.jar", "config/example.toml", "server.properties",
		"eula.txt", "fabric-server-launch.jar", "start.bat", "start.sh",
		"server-pack.json", "README.txt",
	} {
		if _, ok := files[name]; !ok {
			t.Fatalf("missing %s in server pack", name)
		}
	}
	if _, ok := files["saves/world/level.dat"]; ok {
		t.Fatal("saves should not be included in a server pack")
	}
	if !strings.Contains(files["server.properties"], "server-port=25566") {
		t.Fatalf("server.properties was not preserved: %q", files["server.properties"])
	}
	if strings.Contains(files["eula.txt"], "eula=true") {
		t.Fatal("eula should not be pre-accepted")
	}
	if !strings.Contains(files["server-pack.json"], `"loader": "fabric"`) {
		t.Fatalf("manifest missing loader: %s", files["server-pack.json"])
	}
}

func TestForgeScriptsIncludeInstaller(t *testing.T) {
	scripts := scriptsFor(runtimeForgeInstaller, store.Instance{
		MCVersion: "1.20.1", Loader: "forge", LoaderVersion: "47.2.0",
	})
	joined := ""
	for _, script := range scripts {
		joined += script.name + ":" + script.body + "\n"
	}
	if !strings.Contains(joined, "--installServer") || !strings.Contains(joined, "run.bat") {
		t.Fatalf("unexpected Forge scripts: %s", joined)
	}
}

func TestFabricScriptsUseServerLauncher(t *testing.T) {
	scripts := scriptsFor(runtimeFabric, store.Instance{})
	joined := ""
	for _, script := range scripts {
		joined += script.name + ":" + script.body + "\n"
	}
	if !strings.Contains(joined, "fabric-server-launch.jar") {
		t.Fatalf("unexpected Fabric scripts: %s", joined)
	}
}
