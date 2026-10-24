package mirror

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLoaderAndLibraryMirrorURLs(t *testing.T) {
	bmcl := BMCLAPI()
	cases := []struct {
		resource Resource
		original string
		want     string
	}{
		{ResourceLoaderMeta, "https://meta.fabricmc.net/v2/versions/loader/1.21.1", BMCLAPIBase + "/fabric-meta/v2/versions/loader/1.21.1"},
		{ResourceLoaderMeta, "https://maven.neoforged.net/releases/net/neoforged/neoforge/maven-metadata.xml", BMCLAPIBase + "/maven/net/neoforged/neoforge/maven-metadata.xml"},
		{ResourceLoaderArtifact, "https://maven.minecraftforge.net/net/minecraftforge/forge/1.20.1-47.2.0/forge-1.20.1-47.2.0-installer.jar", BMCLAPIBase + "/maven/net/minecraftforge/forge/1.20.1-47.2.0/forge-1.20.1-47.2.0-installer.jar"},
		{ResourceLibrary, "https://maven.fabricmc.net/net/fabricmc/fabric-loader/0.16.10/fabric-loader-0.16.10.jar", BMCLAPIBase + "/maven/net/fabricmc/fabric-loader/0.16.10/fabric-loader-0.16.10.jar"},
	}
	for _, tc := range cases {
		got, ok := bmcl.URL(tc.resource, tc.original, "1.21.1")
		if !ok || got != tc.want {
			t.Errorf("URL(%s) = %q, %v; want %q", tc.resource, got, ok, tc.want)
		}
	}
}

func TestJavaSourcesIncludeTuna(t *testing.T) {
	sources := JavaSources(nil)
	found := false
	for _, source := range sources {
		if source.Kind == KindTunaAdoptium {
			found = true
		}
	}
	if !found {
		t.Fatal("JavaSources should include the Tuna Adoptium mirror")
	}
}

func TestTunaAdoptiumURL(t *testing.T) {
	source := TunaAdoptium()
	original := "https://github.com/adoptium/temurin17-binaries/releases/download/jdk-17.0.20.1%2B1/OpenJDK17U-jre_x64_windows_hotspot_17.0.20.1_1.zip"
	got, ok := source.URL(ResourceJavaArtifact, original, "17")
	if !ok {
		t.Fatal("Tuna Adoptium URL was not resolved")
	}
	want := TunaAdoptiumBase + "/17/jre/x64/windows/OpenJDK17U-jre_x64_windows_hotspot_17.0.20.1_1.zip"
	if got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}
}

func TestRankURLsFallsBackWhenAllFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	urls := []string{server.URL + "/a", server.URL + "/b"}
	got := RankURLs(context.Background(), urls, time.Second)
	if len(got) != 2 || got[0] != urls[0] || got[1] != urls[1] {
		t.Fatalf("fallback order = %#v", got)
	}
}
