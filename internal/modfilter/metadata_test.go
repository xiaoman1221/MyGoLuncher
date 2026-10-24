package modfilter

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func writeMetadataJar(t *testing.T, path, metadataName, metadata string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create(metadataName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(metadata)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestReadMetadataFabricAndForge(t *testing.T) {
	fabric := filepath.Join(t.TempDir(), "fabric.jar")
	writeMetadataJar(t, fabric, "fabric.mod.json",
		`{"id":"coolmod","name":"Cool Mod","version":"1.2.3","environment":"*","provides":["cool_api"],"breaks":{"badmod":"*"}}`)
	meta, err := ReadMetadata(fabric)
	if err != nil {
		t.Fatal(err)
	}
	if meta.ID != "coolmod" || meta.Version != "1.2.3" || len(meta.Breaks) != 1 || meta.Breaks[0] != "badmod" {
		t.Fatalf("fabric metadata = %#v", meta)
	}

	forge := filepath.Join(t.TempDir(), "forge.jar")
	writeMetadataJar(t, forge, "META-INF/mods.toml",
		"modLoader=\"javafml\"\n[[mods]]\nmodId=\"forge_mod\"\ndisplayName=\"Forge Mod\"\nversion=\"2.0.0\"\n[[dependencies.forge_mod]]\nmodId=\"old_api\"\ntype=\"incompatible\"\n")
	meta, err = ReadMetadata(forge)
	if err != nil {
		t.Fatal(err)
	}
	if meta.ID != "forge_mod" || meta.Name != "Forge Mod" || len(meta.Breaks) != 1 || meta.Breaks[0] != "old_api" {
		t.Fatalf("forge metadata = %#v", meta)
	}
}

func TestScanInstalledAndDetectConflicts(t *testing.T) {
	instance := t.TempDir()
	writeMetadataJar(t, filepath.Join(instance, "mods", "coolmod-1.0.jar"), "fabric.mod.json",
		`{"id":"coolmod","version":"1.0"}`)
	writeMetadataJar(t, filepath.Join(instance, "mods", "other.jar"), "fabric.mod.json",
		`{"id":"other","breaks":["newmod"]}`)
	installed := ScanInstalled(instance)
	if len(installed) != 2 {
		t.Fatalf("installed = %#v", installed)
	}

	candidate := Metadata{ID: "coolmod", Breaks: []string{"other"}}
	conflicts := DetectConflicts(candidate, "coolmod-2.0.jar", installed, nil, nil)
	if len(conflicts) < 2 {
		t.Fatalf("conflicts = %#v", conflicts)
	}
	foundDuplicate := false
	foundBreak := false
	for _, conflict := range conflicts {
		if conflict.Kind == "duplicate" {
			foundDuplicate = true
		}
		if conflict.Kind == "incompatible" {
			foundBreak = true
		}
	}
	if !foundDuplicate || !foundBreak {
		t.Fatalf("conflicts = %#v", conflicts)
	}

	// Same file name is an update, not a duplicate.
	conflicts = DetectConflicts(Metadata{ID: "coolmod"}, "coolmod-1.0.jar", installed, nil, nil)
	for _, conflict := range conflicts {
		if conflict.Kind == "duplicate" {
			t.Fatalf("same-file update should not be duplicate: %#v", conflicts)
		}
	}
}
