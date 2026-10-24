package modpack

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"MyMinecraftGoLuncher/internal/store"
)

func writeZip(t *testing.T, path string, entries map[string][]byte) {
	t.Helper()
	var buffer bytes.Buffer
	zw := zip.NewWriter(&buffer)
	for name, data := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buffer.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(store.PathsFor(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestPortableExportSkipVersionMetadata(t *testing.T) {
	gameDir := filepath.Join(t.TempDir(), "versions", "JinXiuYuan")
	if err := os.MkdirAll(gameDir, 0o755); err != nil {
		t.Fatal(err)
	}
	infoFor := func(name string) os.FileInfo {
		path := filepath.Join(gameDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		return info
	}
	skip := portableExportSkip(gameDir, exportSkip)
	for _, name := range []string{"JinXiuYuan.json", "JinXiuYuan.jar", "JinXiuYuan-natives/lib.dll", "natives/lib.dll"} {
		if !skip(name, infoFor(name)) {
			t.Errorf("%s should not be exported", name)
		}
	}
	for _, name := range []string{"mods/example.jar", "config/settings.json"} {
		if skip(name, infoFor(name)) {
			t.Errorf("%s should be exported", name)
		}
	}
}

func TestExportSkipRollbackJar(t *testing.T) {
	path := filepath.Join(t.TempDir(), "example.jar.old")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !exportSkip("mods/example.jar.old", info) {
		t.Fatal("rollback jar should not be exported")
	}
	jarPath := filepath.Join(filepath.Dir(path), "example.jar")
	if err := os.WriteFile(jarPath, []byte("jar"), 0o644); err != nil {
		t.Fatal(err)
	}
	jarInfo, _ := os.Stat(jarPath)
	if exportSkip("mods/example.jar", jarInfo) {
		t.Fatal("normal jar should be exported")
	}
}

func TestInspectMRPackAndImportExport(t *testing.T) {
	payload := []byte("fake mod jar")
	sum := sha512.Sum512(payload)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	index := map[string]any{
		"formatVersion": 1,
		"game":          "minecraft",
		"versionId":     "1.0.0",
		"name":          "Test Modpack",
		"summary":       "test",
		"files": []map[string]any{{
			"path":      "mods/example.jar",
			"hashes":    map[string]string{"sha512": hex.EncodeToString(sum[:])},
			"env":       map[string]string{"client": "required"},
			"downloads": []string{server.URL + "/example.jar"},
			"fileSize":  len(payload),
		}},
		"dependencies": map[string]string{
			"minecraft":     "1.21.1",
			"fabric-loader": "0.16.10",
		},
	}
	indexData, _ := json.Marshal(index)
	path := filepath.Join(t.TempDir(), "pack.mrpack")
	writeZip(t, path, map[string][]byte{
		"modrinth.index.json":       indexData,
		"overrides/config/test.txt": []byte("enabled=true"),
		"overrides/mods/extra.jar":  []byte("override mod"),
	})

	inspection, err := Inspect(path)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Format != FormatMRPack || inspection.MCVersion != "1.21.1" || inspection.Loader != "fabric" || inspection.LoaderVersion != "0.16.10" {
		t.Fatalf("unexpected inspection: %#v", inspection)
	}
	if inspection.Name != "Test Modpack" {
		t.Fatalf("unexpected name: %q", inspection.Name)
	}

	st := testStore(t)
	result, err := Import(context.Background(), st, path, ImportOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Instance.Loader != "fabric" || result.Instance.MCVersion != "1.21.1" {
		t.Fatalf("unexpected instance: %#v", result.Instance)
	}
	gameDir := st.InstanceDir(result.Instance.ID)
	for file, want := range map[string]string{
		"mods/example.jar": string(payload),
		"config/test.txt":  "enabled=true",
		"mods/extra.jar":   "override mod",
	} {
		data, err := os.ReadFile(filepath.Join(gameDir, file))
		if err != nil {
			t.Fatalf("%s missing: %v", file, err)
		}
		if string(data) != want {
			t.Fatalf("%s = %q, want %q", file, data, want)
		}
	}

	exportPath := filepath.Join(t.TempDir(), "export.mrpack")
	if err := ExportMRPack(context.Background(), result.Instance, gameDir, exportPath); err != nil {
		t.Fatal(err)
	}
	exported, err := Inspect(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	if exported.Format != FormatMRPack || exported.MCVersion != "1.21.1" || exported.Loader != "fabric" {
		t.Fatalf("exported pack metadata mismatch: %#v", exported)
	}
	zr, err := zip.OpenReader(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	found := map[string]bool{}
	for _, f := range zr.File {
		found[f.Name] = true
	}
	if !found["modrinth.index.json"] || !found["overrides/mods/example.jar"] || !found["overrides/config/test.txt"] {
		t.Fatalf("exported archive missing files: %#v", found)
	}
}

func TestImportMRPackUsesMirrorCandidates(t *testing.T) {
	payload := []byte("mirror mod jar")
	sum := sha512.Sum512(payload)
	official := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "official unavailable", http.StatusBadGateway)
	}))
	defer official.Close()
	mirrorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer mirrorServer.Close()

	index := map[string]any{
		"formatVersion": 1, "game": "minecraft", "versionId": "1.0.0", "name": "Mirror Pack",
		"files": []map[string]any{{
			"path":      "mods/example.jar",
			"hashes":    map[string]string{"sha512": hex.EncodeToString(sum[:])},
			"downloads": []string{official.URL + "/example.jar"},
			"fileSize":  len(payload),
		}},
		"dependencies": map[string]string{"minecraft": "1.21.1", "fabric-loader": "0.16.10"},
	}
	indexData, _ := json.Marshal(index)
	path := filepath.Join(t.TempDir(), "mirror.mrpack")
	writeZip(t, path, map[string][]byte{"modrinth.index.json": indexData})

	st := testStore(t)
	result, err := Import(context.Background(), st, path, ImportOptions{
		URLRewriter: func(string) []string {
			return []string{mirrorServer.URL + "/example.jar", official.URL + "/example.jar"}
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	gameDir := st.InstanceDir(result.Instance.ID)
	data, err := os.ReadFile(filepath.Join(gameDir, "mods", "example.jar"))
	if err != nil || string(data) != string(payload) {
		t.Fatalf("mirror file = %q, %v", data, err)
	}
}

func TestExportMRPackIsImportableByAnotherStore(t *testing.T) {
	source := testStore(t)
	inst, err := source.CreateInstance(store.Instance{
		Name: "Portable Pack", MCVersion: "1.20.1", Loader: "fabric",
		LoaderVersion: "0.15.11", LaunchVersionID: "fabric-loader-0.15.11-1.20.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	gameDir := source.InstanceDir(inst.ID)
	writeTestFile(t, filepath.Join(gameDir, "mods", "a.jar"), "mod")
	writeTestFile(t, filepath.Join(gameDir, "config", "x.toml"), "config")
	writeTestFile(t, filepath.Join(gameDir, "saves", "world", "level.dat"), "save")
	writeTestFile(t, filepath.Join(gameDir, "options.txt"), "fov:70")
	exportPath := filepath.Join(t.TempDir(), "portable.mrpack")
	if err := ExportMRPack(context.Background(), inst, gameDir, exportPath); err != nil {
		t.Fatal(err)
	}

	target := testStore(t)
	result, err := Import(context.Background(), target, exportPath, ImportOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	importedDir := target.InstanceDir(result.Instance.ID)
	for _, name := range []string{"mods/a.jar", "config/x.toml"} {
		if _, err := os.Stat(filepath.Join(importedDir, filepath.FromSlash(name))); err != nil {
			t.Fatalf("%s missing after re-import: %v", name, err)
		}
	}
	for _, name := range []string{"saves/world/level.dat", "options.txt"} {
		if _, err := os.Stat(filepath.Join(importedDir, filepath.FromSlash(name))); err == nil {
			t.Fatalf("%s should not be part of a portable modpack", name)
		}
	}
}

func TestCurseForgeWithoutAPIKey(t *testing.T) {
	manifest := map[string]any{
		"minecraft": map[string]any{
			"version": "1.20.1",
			"modLoaders": []map[string]any{
				{"id": "forge-47.2.0", "primary": true},
			},
		},
		"manifestType":    "minecraftModpack",
		"manifestVersion": 1,
		"name":            "CF Pack",
		"version":         "1.0",
		"overrides":       "overrides",
		"files": []map[string]any{
			{"projectID": 123, "fileID": 456, "required": true},
		},
	}
	data, _ := json.Marshal(manifest)
	path := filepath.Join(t.TempDir(), "cf.zip")
	writeZip(t, path, map[string][]byte{
		"manifest.json":              data,
		"overrides/config/forge.cfg": []byte("forge=true"),
	})
	ins, err := Inspect(path)
	if err != nil {
		t.Fatal(err)
	}
	if ins.Format != FormatCurseForge || ins.Loader != "forge" || ins.LoaderVersion != "47.2.0" {
		t.Fatalf("unexpected inspection: %#v", ins)
	}
	st := testStore(t)
	result, err := Import(context.Background(), st, path, ImportOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.MissingFiles != 1 || len(result.Warnings) == 0 {
		t.Fatalf("expected missing file warning, got %#v", result)
	}
	content, err := os.ReadFile(filepath.Join(st.InstanceDir(result.Instance.ID), "config", "forge.cfg"))
	if err != nil || string(content) != "forge=true" {
		t.Fatalf("override not extracted: %q, %v", content, err)
	}
}

func TestMultiMCImport(t *testing.T) {
	pack := map[string]any{
		"formatVersion": 1,
		"components": []map[string]string{
			{"uid": "net.minecraft", "version": "1.19.2"},
			{"uid": "org.quiltmc.quilt-loader", "version": "0.20.0"},
		},
	}
	packData, _ := json.Marshal(pack)
	path := filepath.Join(t.TempDir(), "mmc.zip")
	writeZip(t, path, map[string][]byte{
		"mmc-pack.json":           packData,
		"instance.cfg":            []byte("InstanceType=OneSix\nname=Quilt World\n"),
		"minecraft/mods/test.jar": []byte("quilt mod"),
		"minecraft/options.txt":   []byte("renderDistance:12"),
	})
	ins, err := Inspect(path)
	if err != nil {
		t.Fatal(err)
	}
	if ins.Format != FormatMultiMC || ins.MCVersion != "1.19.2" || ins.Loader != "quilt" || ins.Name != "Quilt World" {
		t.Fatalf("unexpected inspection: %#v", ins)
	}
	st := testStore(t)
	result, err := Import(context.Background(), st, path, ImportOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	gameDir := st.InstanceDir(result.Instance.ID)
	if data, err := os.ReadFile(filepath.Join(gameDir, "mods", "test.jar")); err != nil || string(data) != "quilt mod" {
		t.Fatalf("mod not imported: %q, %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(gameDir, "options.txt")); err != nil || string(data) != "renderDistance:12" {
		t.Fatalf("options not imported: %q, %v", data, err)
	}
}

func TestGenericImportUsesFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "generic.zip")
	writeZip(t, path, map[string][]byte{
		"MyPack/options.txt":  []byte("fov:70"),
		"MyPack/mods/mod.jar": []byte("generic mod"),
	})
	ins, err := Inspect(path)
	if err != nil {
		t.Fatal(err)
	}
	if ins.Format != FormatGeneric || ins.MCVersion != "" {
		t.Fatalf("unexpected inspection: %#v", ins)
	}
	st := testStore(t)
	result, err := Import(context.Background(), st, path, ImportOptions{FallbackMCVersion: "1.20.1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Instance.MCVersion != "1.20.1" {
		t.Fatalf("fallback version not applied: %#v", result.Instance)
	}
	if data, err := os.ReadFile(filepath.Join(st.InstanceDir(result.Instance.ID), "options.txt")); err != nil || string(data) != "fov:70" {
		t.Fatalf("generic content not imported: %q, %v", data, err)
	}
}

func TestExportZipProducesMultiMCLayout(t *testing.T) {
	st := testStore(t)
	inst, err := st.CreateInstance(store.Instance{
		Name:            "Backup Test",
		MCVersion:       "1.20.1",
		Loader:          "fabric",
		LoaderVersion:   "0.15.0",
		LaunchVersionID: "fabric-loader-0.15.0-1.20.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	gameDir := st.InstanceDir(inst.ID)
	if err := os.MkdirAll(filepath.Join(gameDir, "mods"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "mods", "a.jar"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(gameDir, "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "logs", "latest.log"), []byte("skip"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "backup.zip")
	if err := ExportZip(context.Background(), inst, gameDir, dest); err != nil {
		t.Fatal(err)
	}
	ins, err := Inspect(dest)
	if err != nil {
		t.Fatal(err)
	}
	if ins.Format != FormatMultiMC || ins.MCVersion != "1.20.1" || ins.Loader != "fabric" {
		t.Fatalf("unexpected backup metadata: %#v", ins)
	}
	zr, err := zip.OpenReader(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	found := map[string]bool{}
	for _, f := range zr.File {
		found[f.Name] = true
	}
	if !found["minecraft/mods/a.jar"] || found["minecraft/logs/latest.log"] || !found["mmc-pack.json"] || !found["instance.cfg"] {
		t.Fatalf("unexpected zip layout: %#v", found)
	}
	target := testStore(t)
	imported, err := Import(context.Background(), target, dest, ImportOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if imported.Instance.Loader != "fabric" || imported.Instance.MCVersion != "1.20.1" {
		t.Fatalf("imported backup metadata: %#v", imported.Instance)
	}
	if _, err := os.Stat(filepath.Join(target.InstanceDir(imported.Instance.ID), "mods", "a.jar")); err != nil {
		t.Fatalf("backup mod missing after re-import: %v", err)
	}
}
