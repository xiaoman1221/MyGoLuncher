package curseforge

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestClassAndLoaderMapping(t *testing.T) {
	cases := map[string]int{
		"mod": ClassMods, "resourcepack": ClassResourcePacks,
		"shader": ClassShaders, "datapack": ClassDataPacks, "modpack": ClassModpacks,
	}
	for resourceType, want := range cases {
		got, ok := ClassIDFor(resourceType)
		if !ok || got != want {
			t.Errorf("ClassIDFor(%q) = %d %v, want %d", resourceType, got, ok, want)
		}
		if TypeForClass(want) != resourceType {
			t.Errorf("TypeForClass(%d) = %q, want %q", want, TypeForClass(want), resourceType)
		}
	}
	loaders := map[string]int{"forge": 1, "fabric": 4, "quilt": 5, "neoforge": 6}
	for name, want := range loaders {
		if got, ok := LoaderID(name); !ok || got != want {
			t.Errorf("LoaderID(%q) = %d %v, want %d", name, got, ok, want)
		}
	}
}

func TestSearchFilesAndDependencies(t *testing.T) {
	payload := []byte("curseforge-dependency")
	sum := sha1.Sum(payload)
	download := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer download.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/mods/search":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{
					"id": 1, "classId": ClassMods, "name": "Test Mod", "slug": "test-mod",
					"summary": "A test", "downloadCount": 1234,
					"logo":       map[string]string{"thumbnailUrl": "https://example.invalid/icon.png"},
					"authors":    []map[string]string{{"name": "Tester"}},
					"categories": []map[string]string{{"name": "Fabric"}},
					"links":      map[string]string{"websiteUrl": "https://example.invalid/project"},
				}},
				"pagination": map[string]int{"totalCount": 1, "index": 0, "pageSize": 20},
			})
		case r.URL.Path == "/mods/1/files/11":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"id": 11, "modId": 1, "displayName": "Test Mod 1.0", "fileName": "test-mod.jar",
				"fileLength": len(payload), "downloadUrl": download.URL + "/test-mod.jar",
				"gameVersions": []string{"1.21.1", "Fabric"},
				"hashes":       []map[string]any{{"algo": 1, "value": hex.EncodeToString(sum[:])}},
				"dependencies": []map[string]any{{"modId": 2, "fileId": 0, "relationType": 3}},
			}})
		case r.URL.Path == "/mods/2/files/22":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"id": 22, "modId": 2, "displayName": "Dependency", "fileName": "dependency.jar",
				"fileLength": len(payload), "downloadUrl": download.URL + "/dependency.jar",
				"gameVersions": []string{"1.21.1", "Fabric"},
				"hashes":       []map[string]any{{"algo": 1, "value": hex.EncodeToString(sum[:])}},
			}})
		case r.URL.Path == "/mods/2/files":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{
				"id": 22, "modId": 2, "displayName": "Dependency", "fileName": "dependency.jar",
				"fileLength": len(payload), "downloadUrl": download.URL + "/dependency.jar",
				"gameVersions": []string{"1.21.1", "Fabric"},
				"hashes":       []map[string]any{{"algo": 1, "value": hex.EncodeToString(sum[:])}},
			}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := &Client{APIKey: "test", baseURL: server.URL}
	result, err := client.Search(context.Background(), SearchRequest{
		ClassID: ClassMods, GameVersion: "1.21.1", Loader: "fabric", Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || len(result.Projects) != 1 || result.Projects[0].Name != "Test Mod" {
		t.Fatalf("unexpected search result: %#v", result)
	}
	file, err := client.FileByID(context.Background(), 1, 11)
	if err != nil {
		t.Fatal(err)
	}
	if !file.Compatible("1.21.1", "fabric") || file.Compatible("1.20.1", "fabric") {
		t.Fatalf("unexpected compatibility: %#v", file)
	}
	dir := t.TempDir()
	deps, err := client.InstallDependencies(context.Background(), file, "1.21.1", "fabric", dir, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected one dependency, got %#v", deps)
	}
	if _, err := os.Stat(filepath.Join(dir, "dependency.jar")); err != nil {
		t.Fatalf("dependency was not downloaded: %v", err)
	}
}

func TestMissingAPIKey(t *testing.T) {
	client := New("")
	if _, err := client.Search(context.Background(), SearchRequest{}); err != ErrNoAPIKey {
		t.Fatalf("expected ErrNoAPIKey, got %v", err)
	}
}
