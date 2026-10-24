package mojang

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFallbackJavaMajor(t *testing.T) {
	cases := map[string]int{
		"1.12.2":  8,
		"1.16.5":  8,
		"1.17":    16,
		"1.17.1":  16,
		"1.18.2":  17,
		"1.20.4":  17,
		"1.20.5":  21,
		"1.21.1":  21,
		"1.21.11": 21,
		"26.3":    25,
	}
	for id, want := range cases {
		if got := FallbackJavaMajor(id); got != want {
			t.Errorf("FallbackJavaMajor(%q) = %d, want %d", id, got, want)
		}
	}
}

func TestArtifactPath(t *testing.T) {
	got, err := ArtifactPath("org.lwjgl:lwjgl:3.4.3:natives-windows")
	if err != nil {
		t.Fatal(err)
	}
	want := "org/lwjgl/lwjgl/3.4.3/lwjgl-3.4.3-natives-windows.jar"
	if got != want {
		t.Fatalf("ArtifactPath = %q, want %q", got, want)
	}
}

func TestArgumentUnmarshal(t *testing.T) {
	var args struct {
		Game []Argument `json:"game"`
	}
	raw := []byte(`{"game":["--username","${auth_player_name}",{"rules":[{"action":"allow","features":{"has_custom_resolution":true}}],"value":["--width","1280"]}]}`)
	if err := json.Unmarshal(raw, &args); err != nil {
		t.Fatal(err)
	}
	if len(args.Game) != 3 {
		t.Fatalf("got %d arguments", len(args.Game))
	}
	if args.Game[0].Value[0] != "--username" {
		t.Fatalf("unexpected first argument: %#v", args.Game[0])
	}
	if len(args.Game[2].Value) != 2 || !args.Game[2].Rules[0].Features["has_custom_resolution"] {
		t.Fatalf("conditional argument not parsed: %#v", args.Game[2])
	}
	if RulesAllow(args.Game[2].Rules, map[string]bool{"has_custom_resolution": false}) {
		t.Fatal("conditional argument should be excluded when feature is false")
	}
}

func TestMergeInheritance(t *testing.T) {
	child := Version{
		ID:           "fabric-loader-0.16.10-1.21.1",
		InheritsFrom: "1.21.1",
		MainClass:    "net.fabricmc.loader.impl.launch.knot.KnotClient",
		Libraries: []Library{
			{Name: "org.example:child:1"},
			{Name: "org.example:common:1"},
		},
		Arguments: Arguments{JVM: []Argument{{Value: []string{"-DFabricMcEmu= net.minecraft.client.main.Main "}}}},
	}
	parent := Version{
		ID:        "1.21.1",
		MainClass: "net.minecraft.client.main.Main",
		Libraries: []Library{
			{Name: "org.example:parent:1"},
			{Name: "org.example:common:1"},
		},
		AssetIndex: AssetIndex{ID: "17"},
		Assets:     "17",
		JavaVersion: &JavaVersion{
			MajorVersion: 21,
		},
		Arguments: Arguments{
			Game: []Argument{{Value: []string{"--username", "${auth_player_name}"}}},
			JVM:  []Argument{{Value: []string{"-cp", "${classpath}"}}},
		},
	}
	merged := Merge([]Version{child, parent})
	if merged.MainClass != child.MainClass {
		t.Fatalf("child main class was not preserved: %s", merged.MainClass)
	}
	if merged.AssetIndex.ID != "17" || merged.JavaMajor() != 21 {
		t.Fatalf("parent metadata was not inherited: %#v", merged)
	}
	if len(merged.Libraries) != 3 {
		t.Fatalf("libraries not deduplicated: %d", len(merged.Libraries))
	}
	if len(merged.Arguments.JVM) != 2 || len(merged.Arguments.Game) != 1 {
		t.Fatalf("arguments were not merged: %#v", merged.Arguments)
	}
}

func TestAssetCatalogMapToResources(t *testing.T) {
	var catalog AssetCatalog
	if err := json.Unmarshal([]byte(`{"objects":{},"map_to_resources":true}`), &catalog); err != nil {
		t.Fatal(err)
	}
	if !catalog.MapsToResources() {
		t.Fatal("expected map_to_resources to be true")
	}
	catalog = AssetCatalog{}
	if err := json.Unmarshal([]byte(`{"objects":{},"virtual":true}`), &catalog); err != nil {
		t.Fatal(err)
	}
	if catalog.MapsToResources() {
		t.Fatal("expected map_to_resources to be false")
	}
	if !catalog.Virtual {
		t.Fatal("expected virtual to be true")
	}
}

func TestMaterializeAssets(t *testing.T) {
	root := t.TempDir()
	hash := "aabbccddeeff00112233445566778899aabbccdd"
	objectPath := filepath.Join(root, "objects", hash[:2], hash)
	if err := os.MkdirAll(filepath.Dir(objectPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(objectPath, []byte("asset-data"), 0o644); err != nil {
		t.Fatal(err)
	}
	catalog := AssetCatalog{Objects: map[string]AssetObject{
		"minecraft/sounds/click.ogg": {Hash: hash, Size: 10},
	}}
	dest := filepath.Join(root, "virtual", "legacy")
	if err := MaterializeAssets(catalog, filepath.Join(root, "objects"), dest); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dest, "minecraft/sounds/click.ogg"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "asset-data" {
		t.Fatalf("unexpected asset content: %q", data)
	}
	if _, err := os.Stat(filepath.Join(dest, ".minecraft-go-complete")); err != nil {
		t.Fatal("completion marker missing")
	}
}

func TestNativeArtifactSelection(t *testing.T) {
	lib := Library{
		Name:    "org.lwjgl:lwjgl:3.3.3",
		Natives: map[string]string{OSName(): "natives-" + OSName() + "-${arch}"},
	}
	classifier := NativeClassifier(lib)
	if classifier == "" {
		t.Fatal("native classifier was not resolved for the host platform")
	}
	lib.Downloads.Classifiers = map[string]Download{
		classifier: {Path: "org/lwjgl/lwjgl/3.3.3/lwjgl-3.3.3-" + classifier + ".jar"},
	}
	dl := NativeArtifact(lib)
	if dl == nil || dl.Path == "" {
		t.Fatal("native artifact was not selected")
	}
	if !IsNativeLibrary(lib) {
		t.Fatal("library should be detected as native")
	}
}
