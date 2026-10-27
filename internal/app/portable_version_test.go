package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"MyMinecraftGoLuncher/internal/mojang"
	"MyMinecraftGoLuncher/internal/store"
)

func TestEnsurePortableVersionWritesStandardFiles(t *testing.T) {
	paths := store.PathsFor(t.TempDir())
	st, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	inst, err := st.CreateInstance(store.Instance{
		Name: "JinXiuYuan", MCVersion: "1.21.1", Loader: "vanilla", LaunchVersionID: "1.21.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	versionDir := filepath.Join(paths.Versions, "1.21.1")
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	versionJSON := map[string]any{
		"id": "1.21.1", "mainClass": "net.minecraft.client.main.Main",
		"downloads": map[string]any{
			"client": map[string]any{"sha1": "abc", "size": 3, "url": "https://example.invalid/client.jar"},
		},
		"assets": "17", "assetIndex": map[string]any{"id": "17", "sha1": "def", "size": 1, "totalSize": 1, "url": "https://example.invalid/assets.json"},
	}
	raw, _ := json.Marshal(versionJSON)
	if err := os.WriteFile(filepath.Join(versionDir, "1.21.1.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "1.21.1.jar"), []byte("jar"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, ".installed.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	app := &App{paths: paths, store: st, mojang: mojang.New(paths)}
	if err := app.ensurePortableVersion(inst); err != nil {
		t.Fatal(err)
	}
	targetDir := filepath.Join(paths.Versions, inst.VersionID)
	var generated mojang.Version
	data, err := os.ReadFile(filepath.Join(targetDir, inst.VersionID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &generated); err != nil {
		t.Fatal(err)
	}
	if generated.ID != inst.VersionID || generated.InheritsFrom != "" || generated.MainClass == "" {
		t.Fatalf("generated version = %#v", generated)
	}
	if jar, err := os.ReadFile(filepath.Join(targetDir, inst.VersionID+".jar")); err != nil || string(jar) != "jar" {
		t.Fatalf("client jar = %q, err=%v", jar, err)
	}
}
