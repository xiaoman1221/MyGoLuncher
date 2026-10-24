//go:build windows

package secretstore

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestRegistryStoreRoundTrip(t *testing.T) {
	if os.Getenv("MYGOLUNCHER_LIVE_TEST") == "" {
		t.Skip("set MYGOLUNCHER_LIVE_TEST=1 to run the registry test")
	}
	store := newRegistryStore()
	name := fmt.Sprintf("test-%d", time.Now().UnixNano())
	defer store.Delete(name)
	value := "secret-测试-" + name
	if err := store.Set(name, value); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(name)
	if err != nil {
		t.Fatal(err)
	}
	if got != value {
		t.Fatalf("Get = %q, want %q", got, value)
	}
	if err := store.Delete(name); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Get(name); got != "" {
		t.Fatalf("deleted value = %q", got)
	}
}
