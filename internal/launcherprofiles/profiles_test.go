package launcherprofiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"MyMinecraftGoLuncher/internal/store"
)

func TestSyncWritesPortableProfilesAndPreservesOthers(t *testing.T) {
	dir := t.TempDir()
	existing := map[string]any{
		"profiles": map[string]any{
			"official-profile": map[string]any{"name": "Official", "lastVersionId": "1.21.1"},
		},
		"settings": map[string]any{"locale": "zh-cn"},
		"custom":   "keep-me",
	}
	raw, _ := json.Marshal(existing)
	if err := os.WriteFile(filepath.Join(dir, "launcher_profiles.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "versions", "FabricPack"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "versions", "FabricPack", "FabricPack.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	created := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	instances := []store.Instance{
		{
			ID: "abc", VersionID: "FabricPack", Name: "Fabric Pack", MCVersion: "1.21.1",
			Loader: "fabric", LaunchVersionID: "fabric-loader-0.16.10-1.21.1",
			MemoryMB: 4096, CreatedAt: created,
		},
	}
	if err := Sync(dir, instances, true); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	data, err := os.ReadFile(filepath.Join(dir, "launcher_profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["custom"] != "keep-me" {
		t.Fatal("unknown root keys were not preserved")
	}
	profiles := doc["profiles"].(map[string]any)
	if _, ok := profiles["official-profile"]; !ok {
		t.Fatal("existing profile was removed")
	}
	ours, ok := profiles["mygoluncher-abc"].(map[string]any)
	if !ok {
		t.Fatalf("our profile missing: %#v", profiles)
	}
	if ours["lastVersionId"] != "FabricPack" || ours["gameDir"] != "versions/FabricPack" {
		t.Fatalf("profile = %#v", ours)
	}
	settings := doc["settings"].(map[string]any)
	if settings["locale"] != "zh-cn" {
		t.Fatal("launcher settings were not preserved")
	}

	// Deleting the instance removes only our stale profile.
	if err := Sync(dir, nil, true); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(dir, "launcher_profiles.json"))
	_ = json.Unmarshal(data, &doc)
	profiles = doc["profiles"].(map[string]any)
	if _, ok := profiles["mygoluncher-abc"]; ok {
		t.Fatal("stale profile was not removed")
	}
	if _, ok := profiles["official-profile"]; !ok {
		t.Fatal("official profile was removed on the second sync")
	}
}

func TestSyncRecoversMalformedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "launcher_profiles.json")
	if err := os.WriteFile(path, []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Sync(dir, []store.Instance{{ID: "x", Name: "X", MCVersion: "1.20.1"}}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Fatal("malformed file backup missing")
	}
	var doc map[string]any
	data, _ := os.ReadFile(path)
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	profile := doc["profiles"].(map[string]any)["mygoluncher-x"].(map[string]any)
	if profile["gameDir"] != "." {
		t.Fatalf("shared gameDir = %#v", profile["gameDir"])
	}
}
