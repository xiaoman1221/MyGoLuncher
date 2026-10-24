package external

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"MyMinecraftGoLuncher/internal/util"
)

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := util.WriteJSON(path, value); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanMinecraftDirectoryAndCopy(t *testing.T) {
	launcher := t.TempDir()
	mcRoot := filepath.Join(launcher, ".minecraft")
	writeFile(t, filepath.Join(launcher, "hmcl.json"), `{}`)

	fabricDir := filepath.Join(mcRoot, "versions", "fabric-loader-0.16.10-1.21.1")
	writeJSON(t, filepath.Join(fabricDir, "fabric-loader-0.16.10-1.21.1.json"), map[string]any{
		"id": "fabric-loader-0.16.10-1.21.1", "inheritsFrom": "1.21.1",
		"mainClass": "net.fabricmc.loader.impl.launch.knot.KnotClient",
		"libraries": []map[string]string{{"name": "net.fabricmc:fabric-loader:0.16.10"}},
	})
	writeFile(t, filepath.Join(fabricDir, "mods", "sodium.jar"), "jar")
	writeFile(t, filepath.Join(fabricDir, "config", "sodium.properties"), "enabled=true")
	writeFile(t, filepath.Join(mcRoot, "saves", "World", "level.dat"), "save")
	writeFile(t, filepath.Join(mcRoot, "options.txt"), "fov:70")

	vanillaDir := filepath.Join(mcRoot, "versions", "1.20.1")
	writeJSON(t, filepath.Join(vanillaDir, "1.20.1.json"), map[string]any{
		"id": "1.20.1", "mainClass": "net.minecraft.client.main.Main",
	})
	writeFile(t, filepath.Join(mcRoot, "mods", "global.jar"), "jar")

	result, err := Scan(launcher)
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != "HMCL" {
		t.Fatalf("unexpected source: %s", result.Source)
	}
	if len(result.Candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %#v", result.Candidates)
	}
	var fabric Candidate
	for _, candidate := range result.Candidates {
		if candidate.Loader == "fabric" {
			fabric = candidate
		}
	}
	if fabric.MCVersion != "1.21.1" || fabric.LoaderVersion != "0.16.10" || fabric.ModCount != 1 {
		t.Fatalf("unexpected fabric candidate: %#v", fabric)
	}
	if fabric.Source != "HMCL" {
		t.Fatalf("candidate source: %s", fabric.Source)
	}
	dest := t.TempDir()
	stats, err := CopyContent(fabric, dest)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Mods != 1 || stats.Files < 3 {
		t.Fatalf("unexpected copy stats: %#v", stats)
	}
	for _, name := range []string{"mods/sodium.jar", "config/sodium.properties", "saves/World/level.dat", "options.txt"} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(name))); err != nil {
			t.Fatalf("%s was not copied: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dest, "mods", "global.jar")); err == nil {
		t.Fatal("global mods should not leak into an isolated version")
	}

	found, err := FindCandidate(launcher, fabric.ID)
	if err != nil || found.Loader != "fabric" {
		t.Fatalf("FindCandidate failed: %#v %v", found, err)
	}
}

func TestScanPrismInstances(t *testing.T) {
	root := t.TempDir()
	instanceDir := filepath.Join(root, "instances", "Quilt World")
	writeJSON(t, filepath.Join(instanceDir, "mmc-pack.json"), map[string]any{
		"formatVersion": 1,
		"components": []map[string]string{
			{"uid": "net.minecraft", "version": "1.19.2"},
			{"uid": "org.quiltmc.quilt-loader", "version": "0.20.0"},
		},
	})
	writeFile(t, filepath.Join(instanceDir, "instance.cfg"), "InstanceType=OneSix\nname=Quilt World\n")
	writeFile(t, filepath.Join(instanceDir, "minecraft", "mods", "qsl.jar"), "jar")
	writeFile(t, filepath.Join(instanceDir, "minecraft", "saves", "A", "level.dat"), "save")

	result, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("expected one candidate, got %#v", result.Candidates)
	}
	candidate := result.Candidates[0]
	if candidate.Name != "Quilt World" || candidate.MCVersion != "1.19.2" || candidate.Loader != "quilt" || candidate.LoaderVersion != "0.20.0" {
		t.Fatalf("unexpected candidate: %#v", candidate)
	}
	dest := t.TempDir()
	if _, err := CopyContent(candidate, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "mods", "qsl.jar")); err != nil {
		t.Fatal("prism mod was not copied")
	}
	if _, err := os.Stat(filepath.Join(dest, "saves", "A", "level.dat")); err != nil {
		t.Fatal("prism save was not copied")
	}
}

func TestScanDirectVersionFolder(t *testing.T) {
	mcRoot := filepath.Join(t.TempDir(), ".minecraft")
	versionDir := filepath.Join(mcRoot, "versions", "1.21.1")
	writeJSON(t, filepath.Join(versionDir, "1.21.1.json"), map[string]any{
		"id": "1.21.1", "mainClass": "net.minecraft.client.main.Main",
	})
	writeFile(t, filepath.Join(mcRoot, "saves", "World", "level.dat"), "save")

	result, err := Scan(versionDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("expected one candidate, got %#v", result.Candidates)
	}
	candidate := result.Candidates[0]
	if candidate.MCVersion != "1.21.1" || candidate.Loader != "vanilla" {
		t.Fatalf("unexpected candidate: %#v", candidate)
	}
	if candidate.ID != "version:1.21.1" {
		t.Fatalf("unexpected direct version ID: %s", candidate.ID)
	}
	found, err := FindCandidate(versionDir, candidate.ID)
	if err != nil || found.MCVersion != "1.21.1" {
		t.Fatalf("FindCandidate failed: %#v %v", found, err)
	}
}

func TestScanVersionsDirectory(t *testing.T) {
	mcRoot := filepath.Join(t.TempDir(), ".minecraft")
	writeJSON(t, filepath.Join(mcRoot, "versions", "1.20.1", "1.20.1.json"), map[string]any{
		"id": "1.20.1", "mainClass": "net.minecraft.client.main.Main",
	})
	writeJSON(t, filepath.Join(mcRoot, "versions", "1.21.1", "1.21.1.json"), map[string]any{
		"id": "1.21.1", "mainClass": "net.minecraft.client.main.Main",
	})

	result, err := Scan(filepath.Join(mcRoot, "versions"))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 2 {
		t.Fatalf("expected two candidates, got %#v", result.Candidates)
	}
	if result.Candidates[0].MCVersion != "1.21.1" {
		t.Fatalf("candidates should be sorted newest first: %#v", result.Candidates)
	}
}

func TestSearchMinecraftFolders(t *testing.T) {
	root := t.TempDir()
	mcRoot := filepath.Join(root, "Games", "MyClient", ".minecraft")
	versionDir := filepath.Join(mcRoot, "versions", "1.21.1")
	writeJSON(t, filepath.Join(versionDir, "1.21.1.json"), map[string]any{
		"id": "1.21.1", "mainClass": "net.minecraft.client.main.Main",
	})
	writeFile(t, filepath.Join(mcRoot, "mods", "client.jar"), "jar")

	results, err := SearchMinecraftFolders(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || len(results[0].Candidates) != 1 {
		t.Fatalf("results = %#v", results)
	}
	if results[0].Candidates[0].MCVersion != "1.21.1" {
		t.Fatalf("candidate = %#v", results[0].Candidates[0])
	}
}

func TestCopyContentFromGameRootAndPrismInstance(t *testing.T) {
	source := t.TempDir()
	writeFile(t, filepath.Join(source, "mods", "a.jar"), "mod")
	writeFile(t, filepath.Join(source, "config", "x.txt"), "config")
	writeFile(t, filepath.Join(source, "saves", "world", "level.dat"), "save")
	writeFile(t, filepath.Join(source, "options.txt"), "lang:zh_cn")
	dest := t.TempDir()
	stats, err := CopyContentFrom(source, dest)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Mods != 1 || stats.Saves != 1 || stats.Files < 4 {
		t.Fatalf("stats = %#v", stats)
	}
	for _, name := range []string{"mods/a.jar", "config/x.txt", "saves/world/level.dat", "options.txt"} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(name))); err != nil {
			t.Fatalf("%s was not copied: %v", name, err)
		}
	}

	prism := t.TempDir()
	writeFile(t, filepath.Join(prism, ".minecraft", "mods", "b.jar"), "mod")
	prismDest := t.TempDir()
	if _, err := CopyContentFrom(prism, prismDest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(prismDest, "mods", "b.jar")); err != nil {
		t.Fatalf("Prism .minecraft content was not copied: %v", err)
	}
}

func TestInferMCVersionKeepsVanillaIDs(t *testing.T) {
	for _, id := range []string{"1.21.1", "24w14a", "1.21-pre1", "1.21-rc1", "b1.7.3", "a1.2.6"} {
		if got := inferMCVersion(id, "vanilla", ""); got != id {
			t.Fatalf("inferMCVersion(%q) = %q", id, got)
		}
	}
	if got := inferMCVersion("fabric-loader-0.16.10-1.21.1", "fabric", "0.16.10"); got != "1.21.1" {
		t.Fatalf("unexpected fabric version: %q", got)
	}
}

func TestKnownRootsIncludeOfficial(t *testing.T) {
	roots := KnownRoots()
	if len(roots) == 0 {
		t.Fatal("expected at least one known root")
	}
	for _, root := range roots {
		if root.ID == "official" && root.Path != "" {
			return
		}
	}
	t.Fatal("official launcher root missing")
}

func TestScanErrors(t *testing.T) {
	if _, err := Scan(t.TempDir()); err == nil {
		t.Fatal("empty directory should fail")
	}
	if _, err := FindCandidate(t.TempDir(), "missing"); err == nil {
		t.Fatal("missing candidate should fail")
	}
}
