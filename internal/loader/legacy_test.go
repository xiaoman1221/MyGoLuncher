package loader

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"MyMinecraftGoLuncher/internal/mojang"
	"MyMinecraftGoLuncher/internal/store"
)

func writeInstallerArchive(t *testing.T, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "forge-installer.jar")
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(out)
	for name, body := range files {
		entry, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInstallForgeLegacyVersionJSON(t *testing.T) {
	paths := store.PathsFor(t.TempDir())
	installer := New(mojang.New(paths), paths)
	archivePath := writeInstallerArchive(t, map[string]string{
		"install_profile.json": `{"json":"/version.json"}`,
		"version.json":         `{"id":"legacy","mainClass":"net.minecraft.launchwrapper.Launch","inheritsFrom":"1.7.10","libraries":[]}`,
		"maven/net/minecraftforge/forge/1.7.10-10.13.4.1614/forge-1.7.10-10.13.4.1614.jar": "jar",
	})
	id, err := installer.installForgeLegacy(archivePath, "1.7.10", "10.13.4.1614", nil)
	if err != nil {
		t.Fatal(err)
	}
	if id != "1.7.10-forge-10.13.4.1614" {
		t.Fatalf("profile id = %s", id)
	}
	if !installer.Mojang.IsProfileInstalled(id) {
		t.Fatal("legacy profile marker missing")
	}
	if _, err := os.Stat(filepath.Join(paths.Libraries, "net", "minecraftforge", "forge", "1.7.10-10.13.4.1614", "forge-1.7.10-10.13.4.1614.jar")); err != nil {
		t.Fatal("maven tree was not extracted")
	}
}

func TestInstallForgeLegacyInstallObject(t *testing.T) {
	paths := store.PathsFor(t.TempDir())
	installer := New(mojang.New(paths), paths)
	archivePath := writeInstallerArchive(t, map[string]string{
		"install_profile.json": `{"install":{"path":"net.minecraftforge:forge:1.12.2-14.23.5.2859","filePath":"forge-universal.jar","versionInfo":{"id":"legacy","mainClass":"net.minecraft.launchwrapper.Launch","libraries":[]}}}`,
		"forge-universal.jar":  "patched",
	})
	id, err := installer.installForgeLegacy(archivePath, "1.12.2", "14.23.5.2859", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(paths.Libraries, "net", "minecraftforge", "forge", "1.12.2-14.23.5.2859", "forge-1.12.2-14.23.5.2859.jar")); err != nil {
		t.Fatal("legacy forge jar was not installed")
	}
	if !installer.Mojang.IsProfileInstalled(id) {
		t.Fatal("legacy profile marker missing")
	}
}

func TestLegacyForgeVersionSplit(t *testing.T) {
	if !isLegacyForgeVersion("14.23.5.2859") || !isLegacyForgeVersion("10.13.4.1614") {
		t.Fatal("old Forge versions should use legacy install")
	}
	if isLegacyForgeVersion("47.4.28") || isLegacyForgeVersion("43.5.2") {
		t.Fatal("modern Forge versions must use the official installer")
	}
}
