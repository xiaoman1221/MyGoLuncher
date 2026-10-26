package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"MyMinecraftGoLuncher/internal/curseforge"
	"MyMinecraftGoLuncher/internal/modrinth"
)

func writeConflictJar(t *testing.T, path, id string, breaks []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("fabric.mod.json")
	if err != nil {
		t.Fatal(err)
	}
	jsonText := "{\"id\":\"" + id + "\",\"version\":\"1.0\",\"breaks\":["
	for index, value := range breaks {
		if index > 0 {
			jsonText += ","
		}
		jsonText += "\"" + value + "\""
	}
	jsonText += "]}"
	if _, err := entry.Write([]byte(jsonText)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDetectModConflictsAndInstallMove(t *testing.T) {
	instance := t.TempDir()
	writeConflictJar(t, filepath.Join(instance, "mods", "cool-old.jar"), "coolmod", nil)
	writeConflictJar(t, filepath.Join(instance, "mods", "enemy.jar"), "enemy", nil)
	candidate := filepath.Join(t.TempDir(), "cool-new.jar")
	writeConflictJar(t, candidate, "coolmod", []string{"enemy"})
	conflicts := detectModConflicts(instance, candidate, nil, nil)
	if len(conflicts) < 2 {
		t.Fatalf("conflicts = %#v", conflicts)
	}
	if _, err := installDownloadedFile(candidate, filepath.Join(instance, "mods")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(instance, "mods", "cool-new.jar")); err != nil {
		t.Fatal("candidate was not moved into mods")
	}
}

func TestScanModJarsHashesAndMetadata(t *testing.T) {
	instance := t.TempDir()
	modPath := filepath.Join(instance, "mods", "sample.jar")
	writeConflictJar(t, modPath, "samplemod", nil)
	if err := os.WriteFile(modPath+".old", []byte("rollback"), 0o644); err != nil {
		t.Fatal(err)
	}
	jars, err := scanModJars(instance)
	if err != nil {
		t.Fatal(err)
	}
	if len(jars) != 1 || jars[0].meta.ID != "samplemod" || jars[0].relative != "mods/sample.jar" || jars[0].sha1 == "" {
		t.Fatalf("jars = %#v", jars)
	}
	if jars[0].fp == 0 {
		t.Fatal("fingerprint was not calculated")
	}
}

func TestIncompatibilityExtraction(t *testing.T) {
	files, ids := modrinthIncompatibilities(modrinth.Version{Dependencies: []modrinth.Dependency{
		{DependencyType: "incompatible", FileName: "bad.jar", ProjectID: "bad"},
		{DependencyType: "required", ProjectID: "good"},
	}})
	if len(files) != 1 || files[0] != "bad.jar" || len(ids) != 1 || ids[0] != "bad" {
		t.Fatalf("modrinth incompatibilities = %#v %#v", files, ids)
	}
	cfIDs := curseForgeIncompatibilities(curseforge.File{Dependencies: []curseforge.FileDependency{
		{RelationType: 5, ModID: 42},
		{RelationType: 3, ModID: 43},
	}})
	if len(cfIDs) != 1 || cfIDs[0] != "42" {
		t.Fatalf("curseforge incompatibilities = %#v", cfIDs)
	}
}

func TestInstallDownloadedFileReplacesExisting(t *testing.T) {
	modsDir := t.TempDir()
	oldPath := filepath.Join(modsDir, "same.jar")
	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	candidate := filepath.Join(t.TempDir(), "same.jar")
	if err := os.WriteFile(candidate, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := installDownloadedFile(candidate, modsDir)
	if err != nil {
		t.Fatal(err)
	}
	if got != oldPath {
		t.Fatalf("target = %s, want %s", got, oldPath)
	}
	data, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("file content = %q", data)
	}
	if _, err := os.Stat(oldPath + ".old"); !os.IsNotExist(err) {
		t.Fatalf("backup was not cleaned up: %v", err)
	}
}
