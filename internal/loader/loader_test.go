package loader

import (
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"Fabric":    "fabric",
		"Quilt":     "quilt",
		"Forge":     "forge",
		"NeoForge":  "neoforge",
		"neo-forge": "neoforge",
		"vanilla":   "vanilla",
		"":          "vanilla",
	}
	for input, want := range cases {
		if got := Normalize(input); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCleanLoaderVersion(t *testing.T) {
	cases := []struct {
		version, mc, loader, want string
	}{
		{"forge-47.2.0", "1.20.1", "forge", "47.2.0"},
		{"1.20.1-47.2.0", "1.20.1", "forge", "47.2.0"},
		{"neoforge-21.1.77", "1.21.1", "neoforge", "21.1.77"},
	}
	for _, tc := range cases {
		if got := cleanLoaderVersion(tc.version, tc.mc, tc.loader); got != tc.want {
			t.Errorf("cleanLoaderVersion(%q) = %q, want %q", tc.version, got, tc.want)
		}
	}
}

func TestForgeInstallerURL(t *testing.T) {
	url, err := forgeInstallerURL("1.20.1", "forge", "47.4.28")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(url, "/forge/1.20.1-47.4.28/forge-1.20.1-47.4.28-installer.jar") {
		t.Fatalf("unexpected Forge URL: %s", url)
	}

	neo, err := forgeInstallerURL("1.21.1", "neoforge", "21.1.77")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(neo, "/neoforge/21.1.77/neoforge-21.1.77-installer.jar") {
		t.Fatalf("unexpected NeoForge URL: %s", neo)
	}

	legacy, err := forgeInstallerURL("1.20.1", "neoforge", "47.1.106")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(legacy, "/net/neoforged/forge/1.20.1-47.1.106/forge-1.20.1-47.1.106-installer.jar") {
		t.Fatalf("unexpected NeoForge legacy URL: %s", legacy)
	}
}

func TestCompareVersions(t *testing.T) {
	if compareVersions("21.1.77", "21.1.9") <= 0 {
		t.Fatal("21.1.77 should sort after 21.1.9")
	}
	if compareVersions("21.10.1", "21.9.9") <= 0 {
		t.Fatal("numeric comparison failed")
	}
	if compareVersions("20.4.1", "21.0.0") >= 0 {
		t.Fatal("20.4.1 should sort before 21.0.0")
	}
}
