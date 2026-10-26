package lansync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkipRollbackMods(t *testing.T) {
	for _, name := range []string{"sodium.jar.old", "sodium.jar.old.1", "Sodium.JAR.OLD", "mod.jar.old.1699999999", "sodium.jar.dis"} {
		if !skipSyncName(name) {
			t.Errorf("rollback file %q should not be synced", name)
		}
	}
	if skipSyncName("sodium.jar") {
		t.Fatal("normal jar should still be synced")
	}
}

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

func TestBuildPlanReportsVersionDifferences(t *testing.T) {
	local := t.TempDir()
	writeTestFile(t, filepath.Join(local, "mods", "same.jar"), "same")
	writeTestFile(t, filepath.Join(local, "mods", "old.jar"), "old")
	writeTestFile(t, filepath.Join(local, "mods", "local-only.jar"), "extra")
	manifest := Manifest{
		Dirs: []string{"mods"},
		Files: []FileEntry{
			{Path: "mods/same.jar", Size: 4, SHA256: sha("same")},
			{Path: "mods/new.jar", Size: 3, SHA256: sha("new")},
			{Path: "mods/old.jar", Size: 4, SHA256: sha("new-old")},
		},
	}
	plan, err := BuildPlan(local, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if plan.NewFiles != 1 || plan.ChangedFiles != 1 || plan.SameFiles != 1 || plan.ExtraFiles != 1 {
		t.Fatalf("plan = %#v", plan)
	}
	if plan.Digest == "" || len(plan.Changes) != 2 {
		t.Fatalf("plan changes = %#v", plan)
	}
	if plan.Changes[0].Path != "mods/new.jar" || plan.Changes[0].State != "new" {
		t.Fatalf("first change = %#v", plan.Changes[0])
	}
	if plan.Extras[0] != "mods/local-only.jar" {
		t.Fatalf("extras = %#v", plan.Extras)
	}
}

func sha(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
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

func TestFullVersionShareSkipsLogs(t *testing.T) {
	source := t.TempDir()
	writeTestFile(t, filepath.Join(source, "mods", "example.jar"), "jar")
	writeTestFile(t, filepath.Join(source, "saves", "world", "level.dat"), "save")
	writeTestFile(t, filepath.Join(source, "options.txt"), "lang:zh_cn")
	writeTestFile(t, filepath.Join(source, "logs", "latest.log"), "log")
	writeTestFile(t, filepath.Join(source, "crash-reports", "crash.txt"), "crash")
	writeTestFile(t, filepath.Join(source, "natives", "lwjgl.dll"), "native")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, err := StartServer(ctx, source, InstanceInfo{
		Name: "Full", MCVersion: "1.19.2", Loader: "forge", LoaderVersion: "43.5.2",
	}, []string{DirAll}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop()
	manifest := server.Manifest()
	if !manifest.Full || len(manifest.Dirs) != 1 || manifest.Dirs[0] != DirAll {
		t.Fatalf("manifest full metadata = %#v", manifest)
	}
	paths := map[string]bool{}
	for _, file := range manifest.Files {
		paths[file.Path] = true
	}
	for _, expected := range []string{"mods/example.jar", "saves/world/level.dat", "options.txt"} {
		if !paths[expected] {
			t.Fatalf("full share missing %s: %#v", expected, paths)
		}
	}
	for _, skipped := range []string{"logs/latest.log", "crash-reports/crash.txt", "natives/lwjgl.dll"} {
		if paths[skipped] {
			t.Fatalf("full share contains generated file %s", skipped)
		}
	}
	destination := t.TempDir()
	plan, err := BuildPlan(destination, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if plan.NewFiles != len(manifest.Files) {
		t.Fatalf("plan = %#v", plan)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", server.Port())
	result, err := Sync(ctx, baseURL, server.Token(), destination, manifest, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Downloaded != len(manifest.Files) {
		t.Fatalf("sync result = %#v", result)
	}
	for _, expected := range []string{"mods/example.jar", "saves/world/level.dat", "options.txt"} {
		if _, err := os.Stat(filepath.Join(destination, filepath.FromSlash(expected))); err != nil {
			t.Fatalf("installed file %s missing: %v", expected, err)
		}
	}
	if _, err := os.Stat(filepath.Join(destination, "logs", "latest.log")); !os.IsNotExist(err) {
		t.Fatalf("generated logs should not be installed: %v", err)
	}
}
