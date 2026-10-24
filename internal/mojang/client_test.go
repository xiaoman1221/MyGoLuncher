package mojang

import (
	"path/filepath"
	"testing"

	"MyMinecraftGoLuncher/internal/mirror"
	"MyMinecraftGoLuncher/internal/store"
)

func TestLibraryDownloadsLegacyNativeAndClasses(t *testing.T) {
	client := New(store.PathsFor(t.TempDir()))
	classifier := "natives-" + OSName()
	lib := Library{
		Name:    "org.lwjgl.lwjgl:lwjgl:2.9.4",
		Natives: map[string]string{OSName(): classifier},
		Downloads: LibraryDownloads{
			Artifact: &Download{
				Path: "org/lwjgl/lwjgl/lwjgl/2.9.4/lwjgl-2.9.4.jar",
				URL:  "https://example.invalid/lwjgl.jar",
				SHA1: "aaaa",
			},
			Classifiers: map[string]Download{
				classifier: {
					Path: "org/lwjgl/lwjgl/lwjgl/2.9.4/lwjgl-2.9.4-" + classifier + ".jar",
					URL:  "https://example.invalid/lwjgl-natives.jar",
					SHA1: "bbbb",
				},
			},
		},
	}
	files, natives, err := client.libraryDownloads(Version{Libraries: []Library{lib}}, []mirror.Source{mirror.Official()})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected classes and native downloads, got %d", len(files))
	}
	if len(natives) != 1 {
		t.Fatalf("expected one native extraction, got %d", len(natives))
	}
	wantNative := filepath.Join(client.paths.Libraries, filepath.FromSlash(
		"org/lwjgl/lwjgl/lwjgl/2.9.4/lwjgl-2.9.4-"+classifier+".jar"))
	if natives[0].jarPath != wantNative {
		t.Fatalf("native jar = %s, want %s", natives[0].jarPath, wantNative)
	}
}

func TestLibraryDownloadsModernNative(t *testing.T) {
	client := New(store.PathsFor(t.TempDir()))
	lib := Library{
		Name: "org.lwjgl:lwjgl:3.4.3:natives-windows",
		Downloads: LibraryDownloads{
			Artifact: &Download{
				Path: "org/lwjgl/lwjgl/3.4.3/lwjgl-3.4.3-natives-windows.jar",
				URL:  "https://example.invalid/lwjgl-natives.jar",
				SHA1: "cccc",
			},
		},
	}
	files, natives, err := client.libraryDownloads(Version{Libraries: []Library{lib}}, []mirror.Source{mirror.Official()})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || len(natives) != 1 {
		t.Fatalf("expected one native artifact, got files=%d natives=%d", len(files), len(natives))
	}
}
