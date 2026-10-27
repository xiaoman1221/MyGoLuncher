package modfilter

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestIsClientOnly(t *testing.T) {
	cases := []struct {
		name    string
		entries map[string]string
		client  bool
	}{
		{
			name:    "fabric client",
			entries: map[string]string{"fabric.mod.json": `{"id":"sodium","environment":"client"}`},
			client:  true,
		},
		{
			name:    "fabric both",
			entries: map[string]string{"fabric.mod.json": `{"id":"lithium","environment":"*"}`},
			client:  false,
		},
		{
			name:    "fabric no environment",
			entries: map[string]string{"fabric.mod.json": `{"id":"example"}`},
			client:  false,
		},
		{
			name:    "fabric client entrypoints only",
			entries: map[string]string{"fabric.mod.json": `{"id":"sodium","entrypoints":{"client":["net.example.Client"]}}`},
			client:  true,
		},
		{
			name:    "fabric common entrypoints",
			entries: map[string]string{"fabric.mod.json": `{"id":"lithium","entrypoints":{"main":["net.example.Main"],"client":["net.example.Client"]}}`},
			client:  false,
		},
		{
			name:    "quilt client",
			entries: map[string]string{"quilt.mod.json": `{"minecraft":{"environment":"client"}}`},
			client:  true,
		},
		{
			name: "forge ignore server",
			entries: map[string]string{
				"META-INF/mods.toml": "modLoader=\"javafml\"\ndisplayTest=\"IGNORE_SERVER_VERSION\"\n",
			},
			client: true,
		},
		{
			name: "neoforge ignore all",
			entries: map[string]string{
				"META-INF/neoforge.mods.toml": "displayTest='IGNORE_ALL_VERSION'",
			},
			client: true,
		},
		{
			name: "neoforge side client",
			entries: map[string]string{
				"META-INF/neoforge.mods.toml": "modLoader='javafml'\nside=\"CLIENT\"\n",
			},
			client: true,
		},
		{
			name:    "forge normal",
			entries: map[string]string{"META-INF/mods.toml": `displayTest="MATCH_VERSION"`},
			client:  false,
		},
		{
			name:    "legacy mcmod",
			entries: map[string]string{"mcmod.info": `[{"modid":"example","clientSideOnly":true}]`},
			client:  true,
		},
		{
			name:    "manifest side",
			entries: map[string]string{"META-INF/MANIFEST.MF": "Manifest-Version: 1.0\r\nModSide: CLIENT\r\n"},
			client:  true,
		},
		{
			name:    "forge manifest side",
			entries: map[string]string{"META-INF/MANIFEST.MF": "Manifest-Version: 1.0\r\nFMLSide: CLIENT\r\n"},
			client:  true,
		},
		{
			name:    "fabric bom client",
			entries: map[string]string{"fabric.mod.json": "\ufeff{\"environment\":\"client\"}"},
			client:  true,
		},
		{
			name:    "unknown",
			entries: map[string]string{"README.txt": "no metadata"},
			client:  false,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			jar := filepath.Join(t.TempDir(), "mod.jar")
			writeJar(t, jar, test.entries)
			got, reason := IsClientOnly(jar)
			if got != test.client {
				t.Fatalf("IsClientOnly = %v (%s), want %v", got, reason, test.client)
			}
		})
	}
}

func TestIsModJar(t *testing.T) {
	if !IsModJar("mods/Sodium-1.0.JAR") {
		t.Fatal("jar should be recognized")
	}
	if IsModJar("mods/readme.txt") {
		t.Fatal("text file should not be recognized")
	}
}

func writeJar(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for name, content := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			file.Close()
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			file.Close()
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
