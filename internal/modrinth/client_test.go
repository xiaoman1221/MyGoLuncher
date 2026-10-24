package modrinth

import "testing"

func testVersion() Version {
	return Version{
		GameVersions: []string{"1.21.1", "1.21"},
		Loaders:      []string{"fabric", "quilt"},
		Files: []File{{
			URL: "https://example.invalid/mod.jar", Filename: "mod.jar", Primary: true,
		}},
	}
}

func TestVersionCompatibility(t *testing.T) {
	version := testVersion()
	cases := []struct {
		game, loader string
		want         bool
	}{
		{"1.21.1", "fabric", true},
		{"1.21", "quilt", true},
		{"1.21.1", "", true},
		{"1.20.1", "fabric", false},
		{"1.21.1", "forge", false},
		{"1.21.1", "neoforge", false},
	}
	for _, tc := range cases {
		if got := version.Compatible(tc.game, tc.loader); got != tc.want {
			t.Errorf("Compatible(%q, %q) = %v, want %v", tc.game, tc.loader, got, tc.want)
		}
	}
	noFiles := version
	noFiles.Files = nil
	if noFiles.Compatible("1.21.1", "fabric") {
		t.Fatal("version without a downloadable file should not be compatible")
	}
}

func TestFilterCompatible(t *testing.T) {
	versions := []Version{
		testVersion(),
		{GameVersions: []string{"1.20.1"}, Loaders: []string{"forge"}, Files: []File{{Primary: true, Filename: "a.jar"}}},
		{GameVersions: []string{"1.21.1"}, Loaders: []string{"neoforge"}, Files: []File{{Primary: true, Filename: "b.jar"}}},
	}
	filtered := FilterCompatible(versions, "1.21.1", "fabric")
	if len(filtered) != 1 || filtered[0].Loaders[0] != "fabric" {
		t.Fatalf("unexpected filtered versions: %#v", filtered)
	}
}
