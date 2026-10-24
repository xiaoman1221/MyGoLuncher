package mirror

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBMCLAPIURLMapping(t *testing.T) {
	source := BMCLAPI()
	manifest, ok := source.URL(ResourceManifest, OfficialManifest, "")
	if !ok || manifest != "https://bmclapi2.bangbang93.com/mc/game/version_manifest_v2.json" {
		t.Fatalf("unexpected manifest URL: %q %v", manifest, ok)
	}
	version, _ := source.URL(ResourceVersion, "https://piston-meta.mojang.com/v1/packages/x/1.20.1.json", "1.20.1")
	if version != "https://bmclapi2.bangbang93.com/version/1.20.1/json" {
		t.Fatalf("unexpected version URL: %s", version)
	}
	library, _ := source.URL(ResourceLibrary, "https://libraries.minecraft.net/com/example/lib/1.0/lib-1.0.jar", "")
	if library != "https://bmclapi2.bangbang93.com/libraries/com/example/lib/1.0/lib-1.0.jar" {
		t.Fatalf("unexpected library URL: %s", library)
	}
	asset, _ := source.URL(ResourceAsset, "https://resources.download.minecraft.net/aa/aabbccddeeff", "")
	if asset != "https://bmclapi2.bangbang93.com/assets/aa/aabbccddeeff" {
		t.Fatalf("unexpected asset URL: %s", asset)
	}
	if _, ok := source.URL(ResourceAssetIndex, "https://piston-meta.mojang.com/x/index.json", ""); ok {
		t.Fatal("BMCLAPI should not claim asset-index support")
	}
	fabric, ok := source.URL(ResourceLibrary, "https://maven.fabricmc.net/net/fabricmc/fabric-loader/1.0/fabric-loader-1.0.jar", "")
	if !ok || fabric != "https://bmclapi2.bangbang93.com/maven/net/fabricmc/fabric-loader/1.0/fabric-loader-1.0.jar" {
		t.Fatalf("BMCLAPI should rewrite loader Maven hosts: %q %v", fabric, ok)
	}
}

func TestCustomMirrorsAndCandidates(t *testing.T) {
	custom, err := NormalizeCustom(CustomConfig{ID: "my", Name: "My Mirror", URL: "mirror.example.com/", Kind: "bmclapi"})
	if err != nil {
		t.Fatal(err)
	}
	if custom.BaseURL != "https://mirror.example.com" || custom.Kind != KindCustomBMCLAPI || !custom.Custom {
		t.Fatalf("unexpected custom source: %#v", custom)
	}
	proxy, err := NormalizeCustom(CustomConfig{Name: "Proxy", URL: "https://proxy.example.com/mc/", Kind: "proxy"})
	if err != nil {
		t.Fatal(err)
	}
	if proxy.Kind != KindCustomProxy {
		t.Fatalf("unexpected proxy kind: %s", proxy.Kind)
	}
	mapped, ok := proxy.URL(ResourceAssetIndex, "https://piston-meta.mojang.com/v1/packages/x/5.json", "")
	if !ok || mapped != "https://proxy.example.com/mc/v1/packages/x/5.json" {
		t.Fatalf("unexpected proxy URL: %q %v", mapped, ok)
	}
	queryMapped, ok := proxy.URL(ResourceJavaMeta, "https://api.adoptium.net/v3/assets/latest/17/hotspot?os=windows&architecture=x64", "")
	if !ok || queryMapped != "https://proxy.example.com/mc/v3/assets/latest/17/hotspot?os=windows&architecture=x64" {
		t.Fatalf("proxy lost query: %q %v", queryMapped, ok)
	}

	sources := []Source{Official(), BMCLAPI(), custom, proxy}
	urls := CandidateURLs(sources, ResourceAsset, "https://resources.download.minecraft.net/aa/hash", "")
	if len(urls) != 4 {
		t.Fatalf("expected four ranked candidates, got %#v", urls)
	}
	foundOfficial := false
	for _, candidate := range urls {
		if candidate == "https://resources.download.minecraft.net/aa/hash" {
			foundOfficial = true
		}
	}
	if !foundOfficial {
		t.Fatalf("official fallback missing: %#v", urls)
	}
	if len(ManifestURLs(sources)) != 4 {
		t.Fatalf("unexpected manifest candidates: %#v", ManifestURLs(sources))
	}
}

func TestResourceMirrorURLMapping(t *testing.T) {
	source := FindResourceSource("mcim", nil)
	cases := map[string]string{
		"https://cdn.modrinth.com/data/abc/versions/1/pack.mrpack": "https://mod.mcimirror.top/data/abc/versions/1/pack.mrpack",
		"https://api.modrinth.com/v2/version/abc":                  "https://mod.mcimirror.top/modrinth/v2/version/abc",
		"https://edge.forgecdn.net/files/123/456/pack.zip":         "https://mod.mcimirror.top/files/123/456/pack.zip",
		"https://api.curseforge.com/v1/mods/1/files/2":             "https://mod.mcimirror.top/curseforge/v1/mods/1/files/2",
	}
	for original, want := range cases {
		got, ok := source.RewriteResourceURL(original)
		if !ok || got != want {
			t.Fatalf("RewriteResourceURL(%q) = %q, %v; want %q", original, got, ok, want)
		}
	}
	if _, ok := source.RewriteResourceURL("https://example.com/file.jar"); ok {
		t.Fatal("MCIM should not claim unknown hosts")
	}

	custom, err := NormalizeResourceCustom(CustomConfig{Name: "Proxy", URL: "https://proxy.example.com/", Kind: "proxy"})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := custom.RewriteResourceURL("https://cdn.modrinth.com/data/a/b.jar?x=1")
	if !ok || got != "https://proxy.example.com/data/a/b.jar?x=1" {
		t.Fatalf("unexpected proxy rewrite: %q %v", got, ok)
	}
	urls := ResourceURLs(source, "https://cdn.modrinth.com/data/a/b.jar")
	if len(urls) != 2 || urls[1] != "https://cdn.modrinth.com/data/a/b.jar" {
		t.Fatalf("official fallback missing: %#v", urls)
	}
	if FindResourceSource("missing", nil).ID != "official" {
		t.Fatal("unknown resource mirror should fall back to official")
	}
}

func TestSpeedTestAndRanking(t *testing.T) {
	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 64<<10)))
	}))
	defer fast.Close()
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(40 * time.Millisecond)
		_, _ = w.Write([]byte(strings.Repeat("y", 4096)))
	}))
	defer slow.Close()
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer broken.Close()

	sources := []Source{
		{ID: "slow", Name: "Slow", BaseURL: slow.URL, Kind: KindCustomProxy, Custom: true},
		{ID: "fast", Name: "Fast", BaseURL: fast.URL, Kind: KindCustomProxy, Custom: true},
		{ID: "broken", Name: "Broken", BaseURL: broken.URL, Kind: KindCustomProxy, Custom: true},
	}
	results := Test(context.Background(), sources, func(source Source) (string, bool) {
		return source.URL(ResourceManifest, OfficialManifest, "")
	}, 3*time.Second)
	if len(results) != 3 || !results[0].OK || results[0].Name != "Fast" {
		t.Fatalf("unexpected ranking: %#v", results)
	}
	if results[2].OK || results[2].Name != "Broken" {
		t.Fatalf("broken source should rank last: %#v", results)
	}
	ranked := Ranked(results)
	if len(ranked) != 2 || ranked[0].Name != "Fast" {
		t.Fatalf("unexpected ranked sources: %#v", ranked)
	}
	if HumanSpeed(2048) != "2.0 KB/s" {
		t.Fatalf("unexpected speed label: %s", HumanSpeed(2048))
	}
}

func TestNormalizeCustomRejectsInvalidURL(t *testing.T) {
	if _, err := NormalizeCustom(CustomConfig{URL: "ftp://example.com"}); err == nil {
		t.Fatal("expected unsupported scheme to fail")
	}
	if _, err := NormalizeCustom(CustomConfig{URL: ""}); err == nil {
		t.Fatal("expected empty URL to fail")
	}
}
