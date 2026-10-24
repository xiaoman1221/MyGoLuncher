package loader

import (
	"strings"
	"testing"

	"MyMinecraftGoLuncher/internal/mirror"
)

func TestLoaderCandidatesUseBMCLAPIMirror(t *testing.T) {
	sources := []mirror.Source{mirror.BMCLAPI()}
	urls := loaderCandidates(sources, mirror.ResourceLoaderMeta, FabricMeta+"/versions/loader/1.21.1", "1.21.1")
	if len(urls) == 0 || !strings.Contains(urls[0], "/fabric-meta/v2/versions/loader/1.21.1") {
		t.Fatalf("fabric mirror candidate = %#v", urls)
	}
	if !strings.Contains(urls[len(urls)-1], "meta.fabricmc.net") {
		t.Fatalf("official fallback missing: %#v", urls)
	}

	artifact := loaderCandidates(sources, mirror.ResourceLoaderArtifact,
		"https://maven.minecraftforge.net/net/minecraftforge/forge/1.20.1-47.2.0/forge-1.20.1-47.2.0-installer.jar", "1.20.1")
	if len(artifact) == 0 || !strings.Contains(artifact[0], "/maven/net/minecraftforge/forge/") {
		t.Fatalf("forge mirror candidate = %#v", artifact)
	}
}
