package resourcemanager

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestListAndToggleResources(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "mods", "sodium.jar"), "enabled")
	write(t, filepath.Join(root, "mods", "old.jar.disabled"), "disabled")
	write(t, filepath.Join(root, "mods", "notes.txt"), "ignored")
	write(t, filepath.Join(root, "resourcepacks", "pack.zip"), "zip")
	if err := os.MkdirAll(filepath.Join(root, "resourcepacks", "folder-pack"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "shaderpacks", "shader.zip.disabled"), "disabled")

	inventory := List(root)
	if len(inventory.Mods) != 2 {
		t.Fatalf("mods = %#v", inventory.Mods)
	}
	if len(inventory.ResourcePacks) != 2 || len(inventory.Shaders) != 1 {
		t.Fatalf("inventory = %#v", inventory)
	}
	mods, packs, shaders := Counts(inventory)
	if mods != 1 || packs != 2 || shaders != 0 {
		t.Fatalf("counts = %d %d %d", mods, packs, shaders)
	}

	if err := SetEnabled(root, KindMods, "sodium.jar", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "mods", "sodium.jar.disabled")); err != nil {
		t.Fatalf("disabled mod missing: %v", err)
	}
	if mods, _, _ := Counts(List(root)); mods != 0 {
		t.Fatalf("enabled mods = %d", mods)
	}
	if err := SetEnabled(root, KindMods, "sodium.jar.disabled", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "mods", "sodium.jar")); err != nil {
		t.Fatalf("re-enabled mod missing: %v", err)
	}

	if err := SetEnabled(root, KindResourcePacks, "pack.zip", false); err != nil {
		t.Fatal(err)
	}
	if err := SetEnabled(root, KindShaders, "shader.zip.disabled", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "shaderpacks", "shader.zip")); err != nil {
		t.Fatalf("shader not enabled: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "resourcepacks", "pack.zip.disabled")); err != nil {
		t.Fatalf("resource pack not disabled: %v", err)
	}
}

func TestSetEnabledRejectsInvalidPathsAndCollisions(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "mods", "a.jar"), "a")
	write(t, filepath.Join(root, "mods", "a.jar.disabled"), "already")
	for _, name := range []string{"../a.jar", "missing.jar", "", "."} {
		if err := SetEnabled(root, KindMods, name, false); err == nil {
			t.Fatalf("%q should fail", name)
		}
	}
	if err := SetEnabled(root, KindMods, "a.jar", false); err == nil {
		t.Fatal("collision should fail")
	}
	if err := SetEnabled(root, "unknown", "a.jar", false); err == nil {
		t.Fatal("unknown kind should fail")
	}
}
