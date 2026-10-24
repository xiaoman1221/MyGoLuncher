package main

import (
	"path/filepath"
	"testing"

	"MyMinecraftGoLuncher/internal/store"
)

func TestInstanceDirHonoursVersionIsolation(t *testing.T) {
	base := t.TempDir()
	paths := store.PathsFor(base)
	st, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{paths: paths, store: st}
	instance := store.Instance{ID: "demo"}

	if got := app.instanceDir(instance); got != filepath.Join(paths.Instances, "demo") {
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
