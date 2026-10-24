package main

import (
	"path/filepath"
	"testing"

	"MyMinecraftGoLuncher/internal/store"
)

func TestContainsVersionTokenBoundaries(t *testing.T) {
	cases := []struct {
		text, version string
		want          bool
	}{
		{"fabric-loader-0.16.10-1.21.1", "1.21.1", true},
		{"1.20.1-forge-47.2.0", "1.20.1", true},
		{"1.20.10", "1.20.1", false},
		{"fabric-loader-0.16.10", "1.21.1", false},
		{"24w14a", "24w14a", true},
	}
	for _, test := range cases {
		if got := containsVersionToken(test.text, test.version); got != test.want {
			t.Fatalf("containsVersionToken(%q, %q) = %v, want %v", test.text, test.version, got, test.want)
		}
	}
}

func TestInstanceDirHonoursVersionIsolation(t *testing.T) {
	base := t.TempDir()
	paths := store.PathsFor(base)
	st, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{paths: paths, store: st}
	instance := store.Instance{ID: "demo"}

	if got := app.instanceDir(instance); got != filepath.Join(paths.Versions, "demo") {
		t.Fatalf("isolated directory = %q", got)
	}

	settings := st.Settings()
	settings.VersionIsolation = false
	if _, err := st.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	if got := app.instanceDir(instance); got != paths.Data {
		t.Fatalf("shared directory = %q", got)
	}
}
