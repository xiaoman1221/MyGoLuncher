package modrinth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testVersion() Version {
	return Version{
		GameVersions: []string{"1.21.1", "1.21"},
		Loaders:      []string{"fabric", "quilt"},
		Files: []File{{
			URL: "https://example.invalid/mod.jar", Filename: "mod.jar", Primary: true,
		}},
	}
}

func TestVersionsByHashes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/version_files" {
			http.NotFound(w, r)
			return
		}
		var body struct {
			Hashes    []string `json:"hashes"`
			Algorithm string   `json:"algorithm"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body.Hashes) != 1 || body.Hashes[0] != "abc" || body.Algorithm != "sha1" {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]Version{
			"abc": {ID: "ver1", ProjectID: "proj1", VersionNumber: "1.0", Name: "Test"},
		})
	}))
	defer server.Close()
	versions, err := versionsByHashesAt(context.Background(), server.URL, []string{"abc"}, "sha1")
	if err != nil {
		t.Fatal(err)
	}
	if versions["abc"].ID != "ver1" || versions["abc"].ProjectID != "proj1" {
		t.Fatalf("versions = %#v", versions)
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
