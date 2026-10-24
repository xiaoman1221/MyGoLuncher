package lansync

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShareAndSync(t *testing.T) {
	source := t.TempDir()
	writeTestFile(t, filepath.Join(source, "mods", "a.jar"), "mod-a")
	writeTestFile(t, filepath.Join(source, "mods", "sub", "b.jar"), "mod-b")
	writeTestFile(t, filepath.Join(source, "shaderpacks", "shader.zip"), "shader")
	writeTestFile(t, filepath.Join(source, "config", "settings.txt"), "config")
	writeTestFile(t, filepath.Join(source, "mods", ".DS_Store"), "junk")

	server, err := StartServer(context.Background(), source, InstanceInfo{
		ID: "source", Name: "Source", MCVersion: "1.21.1", Loader: "fabric", LoaderVersion: "0.16",
	}, []string{"mods", "shaderpacks", "resourcepacks", "config"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop()
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", server.Port())

	if _, err := FetchManifest(context.Background(), baseURL, "wrong-token"); err == nil {
		t.Fatal("wrong token should be rejected")
	}
	manifest, err := FetchManifest(context.Background(), baseURL, server.Token())
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Instance.Name != "Source" || len(manifest.Files) != 4 {
		t.Fatalf("manifest = %#v", manifest)
	}
	for _, entry := range manifest.Files {
		if strings.Contains(entry.Path, ".DS_Store") {
			t.Fatalf("temporary file leaked into manifest: %#v", manifest.Files)
		}
	}

	destination := t.TempDir()
	writeTestFile(t, filepath.Join(destination, "mods", "a.jar"), "mod-a")
	writeTestFile(t, filepath.Join(destination, "mods", "sub", "b.jar"), "xxxxx")
	result, err := Sync(context.Background(), baseURL, server.Token(), destination, manifest, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Downloaded != 3 || result.Skipped != 1 {
		t.Fatalf("result = %#v", result)
	}
	for rel, want := range map[string]string{
		"mods/a.jar":             "mod-a",
		"mods/sub/b.jar":         "mod-b",
		"shaderpacks/shader.zip": "shader",
		"config/settings.txt":    "config",
	} {
		raw, err := os.ReadFile(filepath.Join(destination, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		if string(raw) != want {
			t.Fatalf("%s = %q, want %q", rel, raw, want)
		}
	}

	again, err := Sync(context.Background(), baseURL, server.Token(), destination, manifest, nil)
	if err != nil {
		t.Fatal(err)
	}
	if again.Downloaded != 0 || again.Skipped != 4 {
		t.Fatalf("second sync = %#v", again)
	}
}

func TestShareServerRejectsTraversalAndMissingToken(t *testing.T) {
	source := t.TempDir()
	writeTestFile(t, filepath.Join(source, "mods", "a.jar"), "mod-a")
	server, err := StartServer(context.Background(), source, InstanceInfo{Name: "Source"}, []string{"mods"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop()
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", server.Port())

	resp, err := http.Get(baseURL + "/manifest")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("missing token status = %d", resp.StatusCode)
	}

	// A path outside the manifest is never served, even with a valid token.
	req, _ := http.NewRequest(http.MethodGet, baseURL+"/file?path="+url.QueryEscape("mods/../config/secret"), nil)
	req.Header.Set(tokenHeader, server.Token())
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("traversal status = %d", resp.StatusCode)
	}
}

func TestSafeRelativePath(t *testing.T) {
	dirs := []string{"mods", "shaderpacks"}
	for _, valid := range []string{"mods/a.jar", "shaderpacks/sub/x.zip"} {
		if _, err := safeRelativePath(valid, dirs); err != nil {
			t.Fatalf("%q should be valid: %v", valid, err)
		}
	}
	for _, invalid := range []string{"", "../secret", "/etc/passwd", "mods/../secret", "logs/a.log", "mods//a.jar"} {
		if _, err := safeRelativePath(invalid, dirs); err == nil {
			t.Fatalf("%q should be rejected", invalid)
		}
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
