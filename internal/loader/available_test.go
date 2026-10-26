package loader

import "testing"

func TestParseMavenLoaderVersionsFiltersGameVersion(t *testing.T) {
	values := []string{
		"1.20.1-47.2.0",
		"1.20.1-47.1.0",
		"1.19.4-45.0.0",
		"1.20.1-47.3.0-beta",
	}
	got := parseMavenLoaderVersions(values, "1.20.1", false, map[string]string{
		"1.20.1-latest":      "47.2.0",
		"1.20.1-recommended": "47.1.0",
	})
	if len(got) != 2 {
		t.Fatalf("expected 2 stable matching versions, got %#v", got)
	}
	if got[0].Version != "47.2.0" || !got[0].Recommended {
		t.Fatalf("unexpected first version: %#v", got[0])
	}
	if got[1].Version != "47.1.0" || !got[1].Recommended {
		t.Fatalf("unexpected second version: %#v", got[1])
	}
}

func TestParseMavenLoaderVersionsNeoForge(t *testing.T) {
	values := []string{
		"21.1.80",
		"21.1.79",
		"20.4.237",
		"21.1.81-beta",
	}
	got := parseMavenLoaderVersions(values, "1.21.1", true, nil)
	if len(got) != 2 {
		t.Fatalf("expected 2 stable NeoForge versions, got %#v", got)
	}
	if got[0].Version != "21.1.80" || got[1].Version != "21.1.79" {
		t.Fatalf("unexpected NeoForge versions: %#v", got)
	}
}
