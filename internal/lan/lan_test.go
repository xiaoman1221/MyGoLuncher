package lan

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestParseAnnouncement(t *testing.T) {
	ann, err := parseAnnouncement([]byte("[MOTD]§aHello §lWorld[/MOTD][AD]25566[/AD]"))
	if err != nil {
		t.Fatal(err)
	}
	if ann.Port != 25566 || ann.MOTD != "Hello World" {
		t.Fatalf("announcement = %#v", ann)
	}
	for _, payload := range []string{"", "[AD]0[/AD]", "[MOTD]x[/MOTD]", "[AD]70000[/AD]"} {
		if _, err := parseAnnouncement([]byte(payload)); err == nil {
			t.Fatalf("%q should be rejected", payload)
		}
	}
}

func TestPresenceRoundTrip(t *testing.T) {
	raw, err := encodePresence("self", Identity{
		Name: "Alex", Instance: "1.21 Fabric", MCVersion: "1.21",
		InstanceID: "instance-1", Loader: "fabric", LoaderVersion: "0.16",
		SyncPort: 45678, SyncToken: "secret-token",
		SyncDirs: []string{"mods", "shaderpacks"},
		ToolPort: 45679, ToolToken: "tool-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	packet, err := decodePresence(raw, "other")
	if err != nil {
		t.Fatal(err)
	}
	if packet.Name != "Alex" || packet.Instance != "1.21 Fabric" || packet.MCVersion != "1.21" {
		t.Fatalf("packet = %#v", packet)
	}
	if packet.InstanceID != "instance-1" || packet.Loader != "fabric" || packet.LoaderVersion != "0.16" ||
		packet.SyncPort != 45678 || packet.SyncToken != "secret-token" || len(packet.SyncDirs) != 2 ||
		packet.ToolPort != 45679 || packet.ToolToken != "tool-token" {
		t.Fatalf("sync/tool fields = %#v", packet)
	}
	if _, err := decodePresence(raw, "self"); err == nil {
		t.Fatal("self packet should be ignored")
	}
	if _, err := decodePresence([]byte(`{"app":"Other"}`), "other"); err == nil {
		t.Fatal("foreign packet should be ignored")
	}
}

func TestSyncTarget(t *testing.T) {
	service := New()
	service.peers["peer"] = &peerEntry{
		Peer: Peer{
			ID: "peer", Address: "10.0.0.5", Instance: "1.21", MCVersion: "1.21",
			Loader: "fabric", SyncPort: 45678, SyncDirs: []string{"mods"}, CanSync: true,
		},
		syncToken: "token",
	}
	target, ok := service.SyncTarget("peer")
	if !ok || target.Address != "10.0.0.5" || target.Port != 45678 || target.Token != "token" {
		t.Fatalf("target = %#v, %v", target, ok)
	}
	service.peers["peer"].CanSync = false
	if _, ok := service.SyncTarget("peer"); ok {
		t.Fatal("peer without sharing should not be a sync target")
	}
}

func TestToolTarget(t *testing.T) {
	service := New()
	service.peers["peer"] = &peerEntry{
		Peer:      Peer{ID: "peer", Address: "10.0.0.5", Name: "Alex", CanChat: true},
		toolPort:  45679,
		toolToken: "tool-token",
	}
	target, ok := service.ToolTarget("peer")
	if !ok || target.Address != "10.0.0.5" || target.Port != 45679 || target.Token != "tool-token" || target.Name != "Alex" {
		t.Fatalf("target = %#v, %v", target, ok)
	}
	service.peers["peer"].CanChat = false
	if _, ok := service.ToolTarget("peer"); ok {
		t.Fatal("peer without LAN tools should not be a tool target")
	}
}

func TestPingMinecraft(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		if _, err := readPacket(conn); err != nil { // handshake
			return
		}
		if _, err := readPacket(conn); err != nil { // status request
			return
		}
		status := map[string]any{
			"version": map[string]any{"name": "1.21.1"},
			"players": map[string]any{
				"max": 20, "online": 2,
				"sample": []map[string]string{{"name": "Alex"}, {"name": "Steve"}},
			},
			"description": map[string]any{
				"text":  "§aHello ",
				"extra": []map[string]string{{"text": "LAN"}},
			},
		}
		raw, _ := json.Marshal(status)
		payload := &bytes.Buffer{}
		writeVarInt(payload, 0)
		writeString(payload, string(raw))
		_ = writePacket(conn, payload.Bytes())
	}()

	address := listener.Addr().(*net.TCPAddr)
	result, err := pingMinecraft(context.Background(), "127.0.0.1", address.Port, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if result.Version != "1.21.1" || result.Online != 2 || result.Max != 20 {
		t.Fatalf("result = %#v", result)
	}
	if result.MOTD != "Hello LAN" {
		t.Fatalf("MOTD = %q", result.MOTD)
	}
	if len(result.Players) != 2 || result.Players[1].Name != "Steve" {
		t.Fatalf("players = %#v", result.Players)
	}
}

func TestSnapshotCorrelatesPeersAndSorts(t *testing.T) {
	service := New()
	now := time.Now()
	service.running = true
	service.broadcast = true
	service.worlds["b:25565"] = &worldEntry{World: World{
		ID: "b:25565", Host: "10.0.0.2", Port: 25565, Address: "10.0.0.2:25565", Source: "lan",
	}, lastSeen: now}
	service.worlds["a:25565"] = &worldEntry{World: World{
		ID: "a:25565", Host: "10.0.0.1", Port: 25565, Address: "10.0.0.1:25565", Source: "scan",
	}, lastSeen: now}
	service.peers["peer"] = &peerEntry{Peer: Peer{
		ID: "peer", Name: "Alex", Address: "10.0.0.2",
	}, lastSeen: now}
	snapshot := service.Snapshot()
	if len(snapshot.Worlds) != 2 || snapshot.Worlds[0].Address != "10.0.0.1:25565" {
		t.Fatalf("worlds = %#v", snapshot.Worlds)
	}
	if snapshot.Worlds[1].HostName != "Alex" {
		t.Fatalf("host name not correlated: %#v", snapshot.Worlds[1])
	}
	if !snapshot.Running || !snapshot.Broadcast || len(snapshot.Peers) != 1 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestSnapshotPlayersIsAlwaysArray(t *testing.T) {
	service := New()
	service.running = true
	service.worlds["w"] = &worldEntry{
		// A ping without a player sample must still serialize as [].
		World:    World{ID: "w", Host: "10.0.0.2", Port: 25565, Address: "10.0.0.2:25565", Source: "scan"},
		lastSeen: time.Now(),
	}
	snapshot := service.Snapshot()
	if snapshot.Worlds[0].Players == nil {
		t.Fatal("snapshot players must not be nil")
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"players":null`) {
		t.Fatalf("snapshot contains a null player list: %s", raw)
	}
}

func TestExpire(t *testing.T) {
	service := New()
	service.running = true
	old := time.Now().Add(-3 * time.Minute)
	service.worlds["lan"] = &worldEntry{World: World{Source: "lan"}, lastSeen: old}
	service.worlds["scan"] = &worldEntry{World: World{Source: "scan"}, lastSeen: old}
	service.peers["peer"] = &peerEntry{Peer: Peer{ID: "peer"}, lastSeen: old}
	service.expire()
	if _, ok := service.worlds["lan"]; ok {
		t.Fatal("LAN world should expire")
	}
	if _, ok := service.worlds["scan"]; ok {
		t.Fatal("scanned world should have expired after two minutes")
	}
	if _, ok := service.peers["peer"]; ok {
		t.Fatal("peer should expire")
	}
}

func TestReadVarIntLimits(t *testing.T) {
	if _, err := readVarInt(strings.NewReader("")); err == nil {
		t.Fatal("empty varint should fail")
	}
	if _, err := readVarInt(bytes.NewReader([]byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x01})); err == nil {
		t.Fatal("oversized varint should fail")
	}
	payload := &bytes.Buffer{}
	writeVarInt(payload, 300)
	value, err := readVarInt(io.NopCloser(payload))
	if err != nil || value != 300 {
		t.Fatalf("varint = %d, %v", value, err)
	}
}
