package lan

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestScanSubnetsLifecycle(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("active LAN scanning is disabled on CI")
	}
	service := New()
	if err := service.Start(context.Background(), Config{Broadcast: false}); err != nil {
		t.Skipf("LAN listener unavailable: %v", err)
	}
	defer service.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var updates int64
	if err := service.ScanSubnets(ctx, func(done, total int) {
		if done > total {
			t.Errorf("progress %d > total %d", done, total)
		}
		atomic.AddInt64(&updates, 1)
	}); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt64(&updates) == 0 {
		t.Fatal("scan did not report progress")
	}
}

func TestScanSubnetsStopDuringScan(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("active LAN scanning is disabled on CI")
	}
	service := New()
	if err := service.Start(context.Background(), Config{Broadcast: false}); err != nil {
		t.Skipf("LAN listener unavailable: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- service.ScanSubnets(context.Background(), func(int, int) {})
	}()
	time.Sleep(150 * time.Millisecond)
	service.Stop()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("scan did not stop")
	}
}

func TestScanHostRequiresMinecraftStatus(t *testing.T) {
	// A plain TCP listener must not be classified as a Minecraft server.
	open, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		conn, err := open.Accept()
		if err == nil {
			_, _ = conn.Write([]byte("this is not a Minecraft status packet"))
			_ = conn.Close()
		}
	}()
	service := New()
	port := open.Addr().(*net.TCPAddr).Port
	service.scanHost(context.Background(), "127.0.0.1", port)
	_ = open.Close()
	if len(service.Snapshot().Worlds) != 0 {
		t.Fatalf("non-Minecraft TCP listener was classified as a server: %#v", service.Snapshot().Worlds)
	}

	// A real Server List Ping response is required before a world is added.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go serveStatusPing(t, listener)
	port = listener.Addr().(*net.TCPAddr).Port
	service.scanHost(context.Background(), "127.0.0.1", port)
	worlds := service.Snapshot().Worlds
	if len(worlds) != 1 || worlds[0].Version != "1.21.1" || worlds[0].Source != "scan" {
		t.Fatalf("valid SLP response was not recorded: %#v", worlds)
	}
}

func serveStatusPing(t *testing.T, listener net.Listener) {
	t.Helper()
	conn, err := listener.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	if _, err := readPacket(conn); err != nil {
		return
	}
	if _, err := readPacket(conn); err != nil {
		return
	}
	status := map[string]any{
		"version":     map[string]any{"name": "1.21.1"},
		"players":     map[string]any{"max": 20, "online": 0},
		"description": map[string]any{"text": "Scanned"},
	}
	raw, _ := json.Marshal(status)
	payload := &bytes.Buffer{}
	writeVarInt(payload, 0)
	writeString(payload, string(raw))
	_ = writePacket(conn, payload.Bytes())
}

func TestScanSubnetsRejectsConcurrentScan(t *testing.T) {
	service := New()
	service.running = true
	service.mu.Lock()
	service.scanStop = func() {}
	service.mu.Unlock()
	defer func() {
		service.mu.Lock()
		service.scanStop = nil
		service.mu.Unlock()
	}()
	if err := service.ScanSubnets(context.Background(), func(int, int) {}); err == nil {
		t.Fatal("concurrent scan should be rejected")
	}
}
