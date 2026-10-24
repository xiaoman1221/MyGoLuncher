package lan

import (
	"context"
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
