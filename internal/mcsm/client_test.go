package mcsm

import (
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
	server   *httptest.Server
	apiKey   string
	legacy   bool
	mu       sync.Mutex
	remote   map[string][]byte
	dirs     map[string]bool
	uploads  map[string]string // password -> upload dir
	sessions map[string]*fakeUpload
	nextID   int
	nextPass int
}

func newFakeMCSM(t *testing.T, apiKey string) *fakeMCSM {
	t.Helper()
	fake := &fakeMCSM{
		apiKey:   apiKey,
		remote:   map[string][]byte{},
		dirs:     map[string]bool{"/": true},
		uploads:  map[string]string{},
		sessions: map[string]*fakeUpload{},
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
		writeEnvelope(w, http.StatusOK, map[string]any{
			"instances": []map[string]any{{
				"daemonId": "daemon", "instanceUuid": "instance",
				"nickname": "Test Server", "status": 3, "apiKey": "should-not-leak",
			}},
		})
	case p == "/api/instance" && r.Method == http.MethodGet:
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
			delete(f.remote, target)
			delete(f.dirs, target)
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
