package mcsm

import (
	"archive/zip"
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"MyMinecraftGoLuncher/internal/secretstore"
)

func TestProfilesKeepAPIKeyEncrypted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "servers.json")
	secrets := secretstore.NewMemory()
	profiles, err := OpenProfiles(path, secrets)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := profiles.Save(SaveProfileRequest{
		Name: "测试服", PanelURL: "http://127.0.0.1:23333", DaemonID: "daemon",
		InstanceUUID: "instance", LocalInstanceID: "local", RemoteRoot: "/",
		SyncDirs: []string{"mods", "config"}, APIKey: "super-secret-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !saved.APIKeySet {
		t.Fatal("APIKeySet should be true")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "super-secret-key") {
		t.Fatalf("servers.json leaked the API key: %s", raw)
	}
	if got, _ := profiles.APIKey(saved.ID); got != "super-secret-key" {
		t.Fatalf("APIKey = %q", got)
	}

	// A blank key on edit keeps the stored one.
	saved.DeleteExtras = true
	saved.APIKeySet = false
	if _, err := profiles.Save(SaveProfileRequest{
		ID: saved.ID, Name: saved.Name, PanelURL: saved.PanelURL, DaemonID: saved.DaemonID,
		InstanceUUID: saved.InstanceUUID, LocalInstanceID: saved.LocalInstanceID,
		RemoteRoot: saved.RemoteRoot, SyncDirs: saved.SyncDirs, DeleteExtras: true,
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ := profiles.APIKey(saved.ID); got != "super-secret-key" {
		t.Fatalf("blank edit deleted the key: %q", got)
	}

	if err := profiles.Delete(saved.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := profiles.APIKey(saved.ID); got != "" {
		t.Fatalf("delete left the key: %q", got)
	}
	if len(profiles.List()) != 0 {
		t.Fatalf("profiles left: %#v", profiles.List())
	}
}

func TestNormalizeProfileValidation(t *testing.T) {
	for _, test := range []struct {
		name string
		req  SaveProfileRequest
	}{
		{"bad scheme", SaveProfileRequest{PanelURL: "ftp://x", DaemonID: "d", InstanceUUID: "i", LocalInstanceID: "l"}},
		{"traversal", SaveProfileRequest{PanelURL: "http://x", DaemonID: "d", InstanceUUID: "i", LocalInstanceID: "l", RemoteRoot: "/a/../b"}},
		{"bad dir", SaveProfileRequest{PanelURL: "http://x", DaemonID: "d", InstanceUUID: "i", LocalInstanceID: "l", SyncDirs: []string{"logs"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			profiles, _ := OpenProfiles(filepath.Join(t.TempDir(), "servers.json"), secretstore.NewMemory())
			if _, err := profiles.Save(test.req); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestListInstancesAndStatus(t *testing.T) {
	fake := newFakeMCSM(t, "key")
	defer fake.Close()

	client, err := NewClient(fake.URL(), "key")
	if err != nil {
		t.Fatal(err)
	}
	instances, err := client.ListInstances(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 1 || instances[0].Nickname != "Test Server" || instances[0].Status != 3 {
		t.Fatalf("instances = %#v", instances)
	}
	status, err := client.Status(context.Background(), "daemon", "instance")
	if err != nil {
		t.Fatal(err)
	}
	if status.Nickname != "Test Server" || status.Status != 3 {
		t.Fatalf("status = %#v", status)
	}
	if _, err := client.Status(context.Background(), "missing", "instance"); err == nil {
		t.Fatal("expected missing instance error")
	}
}

func TestStatusFallsBackToInstanceDetail(t *testing.T) {
	fake := newFakeMCSM(t, "key")
	defer fake.Close()
	fake.hideUserInstances = true

	client, err := NewClient(fake.URL(), "key")
	if err != nil {
		t.Fatal(err)
	}
	status, err := client.Status(context.Background(), "daemon", "instance")
	if err != nil {
		t.Fatal(err)
	}
	if status.Nickname != "Test Server" || status.Status != 3 {
		t.Fatalf("status = %#v", status)
	}
}

func TestListAllInstancesForAdminKeys(t *testing.T) {
	fake := newFakeMCSM(t, "key")
	defer fake.Close()

	client, err := NewClient(fake.URL(), "key")
	if err != nil {
		t.Fatal(err)
	}
	instances, err := client.ListAllInstances(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 1 || instances[0].DaemonID != "daemon" ||
		instances[0].InstanceUUID != "global-instance" || instances[0].Nickname != "Global Server" {
		t.Fatalf("instances = %#v", instances)
	}
}

func TestListAllInstancesFallsBackToDocumentedEndpoint(t *testing.T) {
	fake := newFakeMCSM(t, "key")
	defer fake.Close()
	fake.noGlobal = true

	client, err := NewClient(fake.URL(), "key")
	if err != nil {
		t.Fatal(err)
	}
	instances, err := client.ListAllInstances(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 1 || instances[0].InstanceUUID != "global-instance" {
		t.Fatalf("instances = %#v", instances)
	}
}

func TestStatusUsesAdminListBeforeForbiddenDetail(t *testing.T) {
	fake := newFakeMCSM(t, "key")
	defer fake.Close()
	fake.hideUserInstances = true
	fake.setRemote("/mods/a.jar", []byte("x"))

	client, err := NewClient(fake.URL(), "key")
	if err != nil {
		t.Fatal(err)
	}
	status, err := client.Status(context.Background(), "daemon", "global-instance")
	if err != nil {
		t.Fatal(err)
	}
	if status.Nickname != "Global Server" {
		t.Fatalf("status = %#v", status)
	}
}

func TestCreateBackupAndRestore(t *testing.T) {
	fake := newFakeMCSM(t, "key")
	defer fake.Close()
	fake.setRemote("/mods/a.jar", []byte("old-a"))
	fake.setRemote("/mods/sub/b.jar", []byte("old-b"))
	client, err := NewClient(fake.URL(), "key")
	if err != nil {
		t.Fatal(err)
	}
	backup, err := client.CreateBackup(context.Background(), "daemon", "instance", "/", "profile1",
		[]string{"mods"}, "sync", nil)
	if err != nil {
		t.Fatal(err)
	}
	if backup.ID == "" || len(backup.Dirs) != 1 || backup.Dirs[0] != "mods" {
		t.Fatalf("backup = %#v", backup)
	}
	if len(backup.MissingDirs) != 0 || len(backup.EmptyDirs) != 0 {
		t.Fatalf("unexpected backup state: %#v", backup)
	}

	fake.setRemote("/mods/a.jar", []byte("new-a"))
	fake.setRemote("/mods/b.jar", []byte("new-b"))
	if err := client.RestoreBackup(context.Background(), "daemon", "instance", "/", "profile1", backup, nil); err != nil {
		t.Fatal(err)
	}
	if got := string(fake.remoteFile("/mods/a.jar")); got != "old-a" {
		t.Fatalf("a.jar = %q", got)
	}
	if got := string(fake.remoteFile("/mods/sub/b.jar")); got != "old-b" {
		t.Fatalf("sub/b.jar = %q", got)
	}
	if len(fake.remoteFile("/mods/b.jar")) != 0 {
		t.Fatal("file added after the snapshot was not removed")
	}
}

func TestCreateBackupRecordsMissingAndEmptyDirs(t *testing.T) {
	fake := newFakeMCSM(t, "key")
	defer fake.Close()
	fake.dirs["/config"] = true
	client, err := NewClient(fake.URL(), "key")
	if err != nil {
		t.Fatal(err)
	}
	backup, err := client.CreateBackup(context.Background(), "daemon", "instance", "/", "profile1",
		[]string{"mods", "config"}, "sync", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(backup.MissingDirs) != 1 || backup.MissingDirs[0] != "mods" {
		t.Fatalf("missing = %#v", backup.MissingDirs)
	}
	if len(backup.EmptyDirs) != 1 || backup.EmptyDirs[0] != "config" {
		t.Fatalf("empty = %#v", backup.EmptyDirs)
	}
}

func TestUploadAndIncrementalSync(t *testing.T) {
	fake := newFakeMCSM(t, "key")
	defer fake.Close()
	fake.setRemote("/mods/a.jar", []byte("zzz"))
	fake.setRemote("/mods/old.jar", []byte("old"))

	local := t.TempDir()
	mustWrite(t, filepath.Join(local, "mods", "a.jar"), "aaa")
	mustWrite(t, filepath.Join(local, "mods", "b.jar"), "bbbb")
	mustWrite(t, filepath.Join(local, "config", "x.txt"), "xx")
	mustWrite(t, filepath.Join(local, "config", "empty.cfg"), "")

	client, err := NewClient(fake.URL(), "key")
	if err != nil {
		t.Fatal(err)
	}
	var stages []string
	result, err := client.Sync(context.Background(), "daemon", "instance", SyncOptions{
		LocalDir:     local,
		RemoteRoot:   "/",
		Dirs:         []string{"mods", "config"},
		DeleteExtras: true,
		Progress: func(stage, message string, current, total int64) {
			stages = append(stages, stage)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Uploaded != 3 || result.Skipped != 1 || result.Deleted != 1 {
		t.Fatalf("result = %#v", result)
	}
	if got := fake.remoteFile("/mods/a.jar"); string(got) != "zzz" {
		t.Fatalf("a.jar should have been skipped, got %q", got)
	}
	if got := fake.remoteFile("/mods/b.jar"); string(got) != "bbbb" {
		t.Fatalf("b.jar = %q", got)
	}
	if got := fake.remoteFile("/config/x.txt"); string(got) != "xx" {
		t.Fatalf("x.txt = %q", got)
	}
	if got := fake.remoteFile("/config/empty.cfg"); len(got) != 0 {
		t.Fatalf("empty.cfg = %q", got)
	}
	if got := fake.remoteFile("/mods/old.jar"); len(got) != 0 {
		t.Fatalf("old.jar should have been deleted, got %q", got)
	}
	if !containsStage(stages, "upload") {
		t.Fatalf("upload stage not reported: %#v", stages)
	}
}

func writeModJar(t *testing.T, filePath, metadataName, metadata string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create(metadataName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(metadata)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSyncSkipsClientOnlyMods(t *testing.T) {
	fake := newFakeMCSM(t, "key")
	defer fake.Close()
	local := t.TempDir()
	writeModJar(t, filepath.Join(local, "mods", "sodium.jar"), "fabric.mod.json",
		`{"id":"sodium","environment":"client"}`)
	writeModJar(t, filepath.Join(local, "mods", "lithium.jar"), "fabric.mod.json",
		`{"id":"lithium","environment":"*"}`)
	mustWrite(t, filepath.Join(local, "config", "server.toml"), "enabled=true")

	client, err := NewClient(fake.URL(), "key")
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Sync(context.Background(), "daemon", "instance", SyncOptions{
		LocalDir: local, RemoteRoot: "/", Dirs: []string{"mods", "config"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Uploaded != 2 || result.Filtered != 1 {
		t.Fatalf("result = %#v", result)
	}
	if len(fake.remoteFile("/mods/sodium.jar")) != 0 {
		t.Fatal("client-only mod was uploaded")
	}
	if len(fake.remoteFile("/mods/lithium.jar")) == 0 {
		t.Fatal("server mod was not uploaded")
	}
}

func TestSyncManualSelectionDoesNotDeleteUnselected(t *testing.T) {
	fake := newFakeMCSM(t, "key")
	defer fake.Close()
	fake.setRemote("/mods/old.jar", []byte("keep-me"))
	local := t.TempDir()
	mustWrite(t, filepath.Join(local, "mods", "a.jar"), "aaa")
	mustWrite(t, filepath.Join(local, "mods", "b.jar"), "bbb")
	mustWrite(t, filepath.Join(local, "config", "x.txt"), "xx")

	client, err := NewClient(fake.URL(), "key")
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Sync(context.Background(), "daemon", "instance", SyncOptions{
		LocalDir: local, RemoteRoot: "/", Dirs: []string{"mods", "config"},
		Files: []string{"mods/a.jar", "config/x.txt"}, DeleteExtras: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Uploaded != 2 || result.Deleted != 0 {
		t.Fatalf("result = %#v", result)
	}
	if len(fake.remoteFile("/mods/b.jar")) != 0 {
		t.Fatal("unselected file was uploaded")
	}
	if string(fake.remoteFile("/mods/old.jar")) != "keep-me" {
		t.Fatal("manual sync deleted an unselected remote file")
	}
}

func TestScanFilesMarksClientOnlyMods(t *testing.T) {
	local := t.TempDir()
	writeModJar(t, filepath.Join(local, "mods", "sodium.jar"), "fabric.mod.json",
		`{"id":"sodium","environment":"client"}`)
	mustWrite(t, filepath.Join(local, "mods", "plain.jar"), "not a zip")
	files, err := ScanFiles(local, []string{"mods"})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("files = %#v", files)
	}
	for _, file := range files {
		if file.Path == "mods/sodium.jar" && !file.ClientOnly {
			t.Fatalf("sodium should be client-only: %#v", file)
		}
		if file.Path == "mods/plain.jar" && file.ClientOnly {
			t.Fatalf("unknown jar must be kept: %#v", file)
		}
	}
}

func TestLegacyUploadFallback(t *testing.T) {
	fake := newFakeMCSM(t, "key")
	defer fake.Close()
	fake.legacy = true

	local := filepath.Join(t.TempDir(), "legacy.jar")
	mustWrite(t, local, "legacy-content")

	client, err := NewClient(fake.URL(), "key")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.UploadFile(context.Background(), "daemon", "instance", "/mods", local, nil); err != nil {
		t.Fatal(err)
	}
	if got := string(fake.remoteFile("/mods/legacy.jar")); got != "legacy-content" {
		t.Fatalf("uploaded content = %q", got)
	}
}

func TestNormalizeDaemonAddrMapsLoopback(t *testing.T) {
	panel, _ := NewClient("http://panel.example:23333", "key")
	got, err := normalizeDaemonBase("http://127.0.0.1:24444", panel.base)
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://panel.example:24444" {
		t.Fatalf("addr = %q", got)
	}

	proxied, _ := NewClient("https://example.com/mcsm", "key")
	got, err = normalizeDaemonBase("example.com", proxied.base)
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://example.com/mcsm" {
		t.Fatalf("proxied addr = %q", got)
	}
}

func mustWrite(t *testing.T, filePath, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func containsStage(stages []string, want string) bool {
	for _, stage := range stages {
		if stage == want {
			return true
		}
	}
	return false
}

type fakeUpload struct {
	dir  string
	name string
	size int64
	buf  []byte
}

type fakeMCSM struct {
	server            *httptest.Server
	apiKey            string
	legacy            bool
	hideUserInstances bool
	noGlobal          bool
	permission        int
	mu                sync.Mutex
	remote            map[string][]byte
	dirs              map[string]bool
	uploads           map[string]string // password -> upload dir
	sessions          map[string]*fakeUpload
	nextID            int
	nextPass          int
}

func newFakeMCSM(t *testing.T, apiKey string) *fakeMCSM {
	t.Helper()
	fake := &fakeMCSM{
		apiKey:     apiKey,
		permission: 10,
		remote:     map[string][]byte{},
		dirs:       map[string]bool{"/": true},
		uploads:    map[string]string{},
		sessions:   map[string]*fakeUpload{},
	}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeMCSM) URL() string { return f.server.URL }
func (f *fakeMCSM) Close()      { f.server.Close() }

func (f *fakeMCSM) setRemote(remotePath string, data []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.remote[remotePath] = append([]byte(nil), data...)
	f.dirs[path.Dir(remotePath)] = true
}

func (f *fakeMCSM) remoteFile(remotePath string) []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]byte(nil), f.remote[remotePath]...)
}

func (f *fakeMCSM) authorized(r *http.Request) bool {
	return r.Header.Get("X-Request-Api-Key") == f.apiKey || r.URL.Query().Get("apikey") == f.apiKey
}

func (f *fakeMCSM) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := r.URL.Path
	switch {
	case p == "/api/auth/" && r.Method == http.MethodGet:
		if !f.authorized(r) {
			writeEnvelope(w, http.StatusForbidden, "API Key disabled")
			return
		}
		instances := []map[string]any{{
			"daemonId": "daemon", "instanceUuid": "instance",
			"nickname": "Test Server", "status": 3, "apiKey": "should-not-leak",
		}}
		if f.hideUserInstances {
			instances = []map[string]any{}
		}
		writeEnvelope(w, http.StatusOK, map[string]any{
			"instances": instances, "permission": f.permission,
		})
	case p == "/api/service/remote_services_instances_global" && r.Method == http.MethodGet:
		if f.noGlobal {
			http.NotFound(w, r)
			return
		}
		writeEnvelope(w, http.StatusOK, map[string]any{
			"daemon": map[string]any{
				"instances": []map[string]any{{
					"instanceUuid": "global-instance", "status": 3,
					"config": map[string]any{"nickname": "Global Server"},
				}},
				"maxPage": 1, "page": 1,
			},
		})
	case p == "/api/service/remote_services_system" && r.Method == http.MethodGet:
		writeEnvelope(w, http.StatusOK, []map[string]any{{"uuid": "daemon"}})
	case p == "/api/service/remote_service_instances" && r.Method == http.MethodGet:
		if r.URL.Query().Get("daemonId") != "daemon" {
			http.NotFound(w, r)
			return
		}
		writeEnvelope(w, http.StatusOK, map[string]any{
			"page": 1, "maxPage": 1,
			"data": []map[string]any{{
				"instanceUuid": "global-instance", "status": 0,
				"config": map[string]any{"nickname": "Global Server"},
			}},
		})
	case p == "/api/instance" && r.Method == http.MethodGet:
		if r.URL.Query().Get("daemonId") != "daemon" || r.URL.Query().Get("uuid") != "instance" {
			writeEnvelope(w, http.StatusForbidden, "instance not found")
			return
		}
		writeEnvelope(w, http.StatusOK, map[string]any{
			"instanceUuid": "instance", "status": 3,
			"config": map[string]any{"nickname": "Test Server"},
		})
	case strings.HasPrefix(p, "/api/protected_instance/"):
		writeEnvelope(w, http.StatusOK, true)
	case p == "/api/files/list":
		target := r.URL.Query().Get("target")
		if target == "" {
			target = "/"
		}
		items := []map[string]any{}
		seenDirs := map[string]bool{}
		for dir := range f.dirs {
			if path.Dir(dir) == target && dir != target && !seenDirs[dir] {
				seenDirs[dir] = true
				items = append(items, map[string]any{"name": path.Base(dir), "type": 0, "size": 0})
			}
		}
		for remotePath, data := range f.remote {
			if path.Dir(remotePath) == target {
				items = append(items, map[string]any{
					"name": path.Base(remotePath), "type": 1, "size": len(data),
				})
			}
		}
		writeEnvelope(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
	case p == "/api/files/mkdir":
		var body struct {
			Target string `json:"target"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.dirs[body.Target] = true
		writeEnvelope(w, http.StatusOK, true)
	case p == "/api/files" && r.Method == http.MethodDelete:
		var body struct {
			Targets []string `json:"targets"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		for _, target := range body.Targets {
			prefix := strings.TrimRight(target, "/") + "/"
			for remotePath := range f.remote {
				if remotePath == target || strings.HasPrefix(remotePath, prefix) {
					delete(f.remote, remotePath)
				}
			}
			for dir := range f.dirs {
				if dir == target || strings.HasPrefix(dir, prefix) {
					delete(f.dirs, dir)
				}
			}
		}
		writeEnvelope(w, http.StatusOK, true)
	case p == "/api/files/move" && r.Method == http.MethodPut:
		var body struct {
			Targets [][2]string `json:"targets"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		for _, pair := range body.Targets {
			data, ok := f.remote[pair[0]]
			if !ok {
				continue
			}
			f.remote[pair[1]] = data
			delete(f.remote, pair[0])
			f.dirs[path.Dir(pair[1])] = true
		}
		writeEnvelope(w, http.StatusOK, true)
	case p == "/api/files/compress" && r.Method == http.MethodPost:
		var body struct {
			Source  string   `json:"source"`
			Targets []string `json:"targets"`
			Type    int      `json:"type"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Type == 1 {
			entries := map[string][]byte{}
			for _, target := range body.Targets {
				base := path.Base(target)
				if data, ok := f.remote[target]; ok {
					entries[base] = data
					continue
				}
				prefix := target + "/"
				for remotePath, data := range f.remote {
					if strings.HasPrefix(remotePath, prefix) {
						entries[base+"/"+strings.TrimPrefix(remotePath, prefix)] = data
					}
				}
			}
			raw, _ := json.Marshal(entries)
			f.remote[body.Source] = raw
			f.dirs[path.Dir(body.Source)] = true
		} else {
			target := ""
			if len(body.Targets) > 0 {
				target = body.Targets[0]
			}
			var entries map[string][]byte
			_ = json.Unmarshal(f.remote[body.Source], &entries)
			for name, data := range entries {
				full := path.Join(target, name)
				f.remote[full] = data
				f.dirs[path.Dir(full)] = true
			}
		}
		writeEnvelope(w, http.StatusOK, true)
	case p == "/api/files/upload" && r.Method == http.MethodPost:
		f.nextPass++
		password := "pw" + strconv.Itoa(f.nextPass)
		f.uploads[password] = r.URL.Query().Get("upload_dir")
		writeEnvelope(w, http.StatusOK, map[string]any{"password": password, "addr": f.server.URL})
	case strings.HasPrefix(p, "/upload-new/") && f.legacy:
		http.NotFound(w, r)
	case strings.HasPrefix(p, "/upload/") && f.legacy:
		password := strings.TrimPrefix(p, "/upload/")
		dir, ok := f.uploads[password]
		if !ok {
			http.Error(w, "Access denied: No task found", http.StatusInternalServerError)
			return
		}
		name, content, err := readMultipartFile(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		f.remote[path.Join(dir, name)] = content
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "OK")
	case strings.HasPrefix(p, "/upload-new/"):
		password := strings.TrimPrefix(p, "/upload-new/")
		dir, ok := f.uploads[password]
		if !ok {
			http.Error(w, "Access denied: No task found", http.StatusInternalServerError)
			return
		}
		size, _ := strconv.ParseInt(r.URL.Query().Get("size"), 10, 64)
		name := r.URL.Query().Get("filename")
		f.nextID++
		id := "up" + strconv.Itoa(f.nextID)
		f.sessions[id] = &fakeUpload{dir: dir, name: name, size: size}
		if size == 0 {
			f.remote[path.Join(dir, name)] = []byte{}
		}
		writeRawJSON(w, map[string]any{"data": map[string]any{"id": id, "received": []any{}}, "status": 200})
	case strings.HasPrefix(p, "/upload-piece/"):
		id := strings.TrimPrefix(p, "/upload-piece/")
		session, ok := f.sessions[id]
		if !ok {
			http.Error(w, "Access denied: No task found", http.StatusInternalServerError)
			return
		}
		offset, _ := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
		_, content, err := readMultipartFile(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		need := int(offset) + len(content)
		if len(session.buf) < need {
			session.buf = append(session.buf, make([]byte, need-len(session.buf))...)
		}
		copy(session.buf[offset:], content)
		if int64(len(session.buf)) >= session.size {
			f.remote[path.Join(session.dir, session.name)] = append([]byte(nil), session.buf[:session.size]...)
			delete(f.sessions, id)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "OK")
	default:
		http.NotFound(w, r)
	}
}

func readMultipartFile(r *http.Request) (string, []byte, error) {
	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return "", nil, err
	}
	reader := multipart.NewReader(r.Body, params["boundary"])
	part, err := reader.NextPart()
	if err != nil {
		return "", nil, err
	}
	defer part.Close()
	content, err := io.ReadAll(part)
	return part.FileName(), content, err
}

func writeEnvelope(w http.ResponseWriter, status int, data any) {
	writeRawJSON(w, map[string]any{"status": status, "data": data})
}

func writeRawJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
