package mojang

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"MyMinecraftGoLuncher/internal/store"
)

func TestRepairProfileReplacesCorruptLibrary(t *testing.T) {
	data := []byte("repaired loader library")
	sum := sha1.Sum(data)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(data)
	}))
	defer server.Close()

	paths := store.PathsFor(t.TempDir())
	client := New(paths)
	writeVersion := func(id, body string) {
		dir := filepath.Join(paths.Versions, id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, id+".json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeVersion("1.20.1", `{"id":"1.20.1","libraries":[]}`)
	writeVersion("loader", fmt.Sprintf(`{"id":"loader","inheritsFrom":"1.20.1","libraries":[{"name":"example:lib:1","downloads":{"artifact":{"path":"example/lib/1/lib-1.jar","url":%q,"sha1":%q,"size":%d}}}]}`, server.URL+"/lib.jar", hex.EncodeToString(sum[:]), len(data)))
	dest := filepath.Join(paths.Libraries, "example", "lib", "1", "lib-1.jar")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := client.RepairProfile(context.Background(), "loader", 1, nil); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatalf("library was not repaired: %q", got)
	}
	if _, err := os.Stat(filepath.Join(paths.Versions, "loader", ".installed.json")); err != nil {
		t.Fatalf("profile marker missing: %v", err)
	}
}
