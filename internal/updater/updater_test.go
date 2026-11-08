package updater

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func legacyFamily(artifact Artifact) *ArtifactFamily {
	return &ArtifactFamily{Legacy: &artifact}
}

func testZip(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create("index.html")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("<html>hot</html>")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestCheckAndApplyWebUpdate(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload := testZip(t)
	sum := sha256.Sum256(payload)
	var server *httptest.Server
	manifest := Manifest{
		Version: "1.1.0",
		Web: legacyFamily(Artifact{
			Version: "1.1.0", URL: "", SHA256: hex.EncodeToString(sum[:]), Size: int64(len(payload)),
		}),
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			raw, _ := json.Marshal(manifest)
			w.Header().Set("X-MGL-Signature", base64.StdEncoding.EncodeToString(ed25519.Sign(private, raw)))
			_, _ = w.Write(raw)
		case "/web.zip":
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	manifest.Web.Legacy.URL = server.URL + "/web.zip"

	oldKey, oldURL := PublicKey, ManifestURL
	PublicKey = public
	ManifestURL = server.URL + "/manifest.json"
	defer func() { PublicKey, ManifestURL = oldKey, oldURL }()

	info, err := Check(context.Background(), "1.0.0", "")
	if err != nil {
		t.Fatal(err)
	}
	if !info.Configured || !info.Available || !info.HasWeb || info.WebVersion != "1.1.0" {
		t.Fatalf("unexpected update info: %#v", info)
	}

	root := t.TempDir()
	dest, err := ApplyWeb(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if dest != filepath.Join(root, "web") {
		t.Fatalf("destination = %q", dest)
	}
	if body, err := os.ReadFile(filepath.Join(dest, "index.html")); err != nil || string(body) != "<html>hot</html>" {
		t.Fatalf("index.html = %q, %v", body, err)
	}
	if version := CurrentWebVersion(root); version != "1.1.0" {
		t.Fatalf("version = %q", version)
	}
}

func TestRejectsInvalidSignature(t *testing.T) {
	public, _, _ := ed25519.GenerateKey(rand.Reader)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manifest.json" {
			_, _ = w.Write([]byte(`{"version":"9.9.9"}`))
			w.Header().Set("X-MGL-Signature", base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize)))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	oldKey, oldURL := PublicKey, ManifestURL
	PublicKey = public
	ManifestURL = server.URL + "/manifest.json"
	defer func() { PublicKey, ManifestURL = oldKey, oldURL }()
	if _, err := Check(context.Background(), "1.0.0", ""); err == nil {
		t.Fatal("invalid signature was accepted")
	}
}

func TestArtifactFamilyNewManifest(t *testing.T) {
	data := `{
		"version":"2.0.0",
		"web":{"any":{"any":{"version":"2.0.0","url":"https://cdn.example/web.zip","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","size":10}}},
		"app":{"windows":{"x64":{"version":"2.0.0","url":"https://cdn.example/app.exe","sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","size":20}}}
	}`
	var manifest Manifest
	if err := json.Unmarshal([]byte(data), &manifest); err != nil {
		t.Fatal(err)
	}
	web, ok := manifest.Web.Select()
	if !ok || web.URL != "https://cdn.example/web.zip" {
		t.Fatalf("web = %#v, %v", web, ok)
	}
	if manifest.App == nil || manifest.App.Legacy != nil {
		t.Fatalf("app family = %#v", manifest.App)
	}
	if _, ok := manifest.App.Entries["windows"]["x64"]; !ok {
		t.Fatalf("windows/x64 missing: %#v", manifest.App.Entries)
	}
}

func TestArtifactFamilyLegacyManifest(t *testing.T) {
	data := `{"version":"1.1.0","web":{"version":"1.1.0","url":"https://cdn.example/web.zip","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","size":10}}`
	var manifest Manifest
	if err := json.Unmarshal([]byte(data), &manifest); err != nil {
		t.Fatal(err)
	}
	web, ok := manifest.Web.Select()
	if !ok || web.URL != "https://cdn.example/web.zip" {
		t.Fatalf("legacy web = %#v, %v", web, ok)
	}
}

func TestArtifactFamilyAliasAndNested(t *testing.T) {
	data := `{"version":"2.0.0","web":{"version":"2.0.0","url":"https://cdn.example/default.zip","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","size":10,"any":{"any":{"version":"2.0.0","url":"https://cdn.example/any.zip","sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","size":20}}}}`
	var manifest Manifest
	if err := json.Unmarshal([]byte(data), &manifest); err != nil {
		t.Fatal(err)
	}
	web, ok := manifest.Web.Select()
	if !ok || web.URL != "https://cdn.example/any.zip" {
		t.Fatalf("alias/nested web = %#v, %v", web, ok)
	}
	if manifest.Web.Legacy == nil || manifest.Web.Legacy.URL != "https://cdn.example/default.zip" {
		t.Fatalf("legacy alias = %#v", manifest.Web.Legacy)
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.1", "1.0.0", 1},
		{"1.1.0", "1.0.9", 1},
		{"1.0.0", "1.0.0", 0},
		{"v2.0.0", "1.9.9", 1},
		{"1.0.0-beta", "1.0.0", 0},
	}
	for _, test := range cases {
		if got := compareVersions(test.a, test.b); got != test.want {
			t.Fatalf("compareVersions(%q,%q) = %d, want %d", test.a, test.b, got, test.want)
		}
	}
}

func TestCheckAndApplyWebUpdateFromAPI(t *testing.T) {
	payload := testZip(t)
	sum := sha256.Sum256(payload)
	var server *httptest.Server
	manifest := Manifest{
		Version: "1.1.0",
		Force:   true,
		Web: legacyFamily(Artifact{
			Version: "1.1.0", SHA256: hex.EncodeToString(sum[:]), Size: int64(len(payload)),
		}),
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/update":
			if r.Header.Get("X-API-Key") != "test-product-key" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"code":"invalid_api_key"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(manifest)
		case "/web.zip":
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	manifest.Web.Legacy.URL = server.URL + "/web.zip"

	oldURL, oldKey, oldManifest := APIBaseURL, APIKey, ManifestURL
	ConfigureAPI(server.URL, "test-product-key")
	ManifestURL = ""
	defer func() {
		APIBaseURL, APIKey, ManifestURL = oldURL, oldKey, oldManifest
	}()

	info, err := Check(context.Background(), "1.0.0", "")
	if err != nil {
		t.Fatal(err)
	}
	if !info.Configured || !info.Available || !info.HasWeb || !info.Force || info.Tag != "force" {
		t.Fatalf("unexpected API update info: %#v", info)
	}
	root := t.TempDir()
	if _, err := ApplyWeb(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if version := CurrentWebVersion(root); version != "1.1.0" {
		t.Fatalf("version = %q", version)
	}
}

func TestAPINoUpdateIsNotAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"update_not_available"}`))
	}))
	defer server.Close()
	oldURL, oldKey, oldManifest := APIBaseURL, APIKey, ManifestURL
	ConfigureAPI(server.URL, "test-product-key")
	ManifestURL = ""
	defer func() { APIBaseURL, APIKey, ManifestURL = oldURL, oldKey, oldManifest }()

	info, err := Check(context.Background(), "1.0.0", "")
	if err != nil {
		t.Fatal(err)
	}
	if !info.Configured || info.Available {
		t.Fatalf("unexpected no-update info: %#v", info)
	}
}

func TestApplyUpdateScriptHasUTF8BOMAndHashCheck(t *testing.T) {
	content := applyUpdateScriptContent()
	if !strings.HasPrefix(content, "\ufeff") {
		t.Fatal("update script must start with a UTF-8 BOM for Windows PowerShell 5.1")
	}
	for _, required := range []string{"ExpectedSHA256", "Get-FileHash", "Start-Process"} {
		if !strings.Contains(content, required) {
			t.Fatalf("update script is missing %q", required)
		}
	}
}

func TestApplyUpdateScriptParsesWithWindowsPowerShell(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows PowerShell is only available on Windows")
	}
	path := filepath.Join(t.TempDir(), "apply-update.ps1")
	if err := os.WriteFile(path, []byte(applyUpdateScriptContent()), 0o600); err != nil {
		t.Fatal(err)
	}
	quoted := strings.ReplaceAll(path, "'", "''")
	script := fmt.Sprintf("$e=$null; [System.Management.Automation.Language.Parser]::ParseFile('%s',[ref]$null,[ref]$e)|Out-Null; if($e.Count){$e|%%{$_.Message}; exit 1}", quoted)
	output, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
	if err != nil {
		t.Fatalf("Windows PowerShell could not parse the update helper: %v\n%s", err, output)
	}
}
