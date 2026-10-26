package launcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"MyMinecraftGoLuncher/internal/mojang"
	"MyMinecraftGoLuncher/internal/store"
)

func TestParseServerAddress(t *testing.T) {
	tests := []struct {
		raw    string
		host   string
		port   int
		want   string
		hasErr bool
	}{
		{"example.com", "example.com", 25565, "example.com:25565", false},
		{"example.com:25566", "example.com", 25566, "example.com:25566", false},
		{"127.0.0.1:25565", "127.0.0.1", 25565, "127.0.0.1:25565", false},
		{"[::1]:25565", "::1", 25565, "[::1]:25565", false},
		{"::1", "::1", 25565, "[::1]:25565", false},
		{"example.com:", "example.com", 25565, "example.com:25565", false},
		{"", "", 0, "", true},
		{"host name", "", 0, "", true},
		{"example.com:0", "", 0, "", true},
		{"example.com:70000", "", 0, "", true},
		{"http://example.com", "", 0, "", true},
		{"host/path", "", 0, "", true},
	}
	for _, test := range tests {
		address, err := ParseServerAddress(test.raw)
		if test.hasErr {
			if err == nil {
				t.Fatalf("%q: expected error", test.raw)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%q: %v", test.raw, err)
		}
		if address.Host != test.host || address.Port != test.port || address.String() != test.want {
			t.Fatalf("%q: got %#v (%s)", test.raw, address, address.String())
		}
	}
}

func TestBuildArgumentsQuickPlay(t *testing.T) {
	l := &Launcher{Paths: store.PathsFor(t.TempDir())}
	merged := mojang.Version{
		Type: "release",
		Arguments: mojang.Arguments{Game: []mojang.Argument{{Value: []string{
			"--username", "${auth_player_name}",
			"--quickPlayMultiplayer", "${quickPlayMultiplayer}",
		}}}},
	}
	address, err := ParseServerAddress("192.168.1.20:25566")
	if err != nil {
		t.Fatal(err)
	}
	_, game, err := l.buildArguments(merged, Options{
		Instance: store.Instance{MemoryMB: 2048},
		Settings: store.Settings{PlayerName: "Steve"},
		Server:   address,
	}, "/tmp/game", "/tmp/natives", "/tmp/natives", "/tmp/classes", "/tmp/assets", "1.21.1")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(game, " ")
	if !strings.Contains(joined, "--quickPlayMultiplayer 192.168.1.20:25566") {
		t.Fatalf("quick play argument missing: %s", joined)
	}
	if strings.Contains(joined, "--server") {
		t.Fatalf("legacy arguments should not be added: %s", joined)
	}
}

func TestBuildArgumentsLegacyDirectConnect(t *testing.T) {
	l := &Launcher{Paths: store.PathsFor(t.TempDir())}
	merged := mojang.Version{
		Type:               "release",
		MinecraftArguments: "--username ${auth_player_name} --uuid ${auth_uuid}",
	}
	address, err := ParseServerAddress("lan.local:25565")
	if err != nil {
		t.Fatal(err)
	}
	_, game, err := l.buildArguments(merged, Options{
		Instance: store.Instance{MemoryMB: 2048},
		Settings: store.Settings{PlayerName: "Steve"},
		Server:   address,
	}, "/tmp/game", "/tmp/natives", "/tmp/natives", "/tmp/classes", "/tmp/assets", "1.7.10")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(game, " ")
	if !strings.Contains(joined, "--server lan.local --port 25565") {
		t.Fatalf("legacy direct connect arguments missing: %s", joined)
	}
}

func TestModernForgeClasspathSkipsVanillaClient(t *testing.T) {
	paths := store.PathsFor(t.TempDir())
	client := mojang.New(paths)
	l := New(client, paths)
	libPath := "net/minecraftforge/fmlloader/1.0/fmlloader-1.0.jar"
	fullLib := filepath.Join(paths.Libraries, filepath.FromSlash(libPath))
	if err := os.MkdirAll(filepath.Dir(fullLib), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullLib, []byte("jar"), 0o644); err != nil {
		t.Fatal(err)
	}
	fullClient := client.ClientJar("1.19.2")
	if err := os.MkdirAll(filepath.Dir(fullClient), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullClient, []byte("client"), 0o644); err != nil {
		t.Fatal(err)
	}
	child := mojang.Version{
		ID: "1.19.2-forge-43.5.2", MainClass: "cpw.mods.bootstraplauncher.BootstrapLauncher",
		Libraries: []mojang.Library{{
			Name:      "net.minecraftforge:fmlloader:1.0",
			Downloads: mojang.LibraryDownloads{Artifact: &mojang.Download{Path: libPath}},
		}},
	}
	parent := mojang.Version{
		ID: "1.19.2", MainClass: "net.minecraft.client.main.Main",
		Downloads: map[string]mojang.Download{"client": {Size: 6}},
	}
	classpath, err := l.buildClasspath(child, []mojang.Version{child, parent}, true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(classpath, fullClient) {
		t.Fatalf("modern Forge classpath contains vanilla client jar: %s", classpath)
	}
	if !strings.Contains(classpath, fullLib) {
		t.Fatalf("modern Forge classpath lost loader library: %s", classpath)
	}
	classpath, err = l.buildClasspath(child, []mojang.Version{child, parent}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(classpath, fullClient) {
		t.Fatalf("vanilla/Fabric classpath lost client jar: %s", classpath)
	}
}
