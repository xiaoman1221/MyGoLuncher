package secretstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMemoryStore(t *testing.T) {
	store := NewMemory()
	if err := store.Set("token", "value"); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get("token")
	if err != nil || got != "value" {
		t.Fatalf("Get = %q, %v", got, err)
	}
	if err := store.Delete("token"); err != nil {
		t.Fatal(err)
	}
	got, err = store.Get("token")
	if err != nil || got != "" {
		t.Fatalf("deleted value = %q, %v", got, err)
	}
}

func TestFileStoreRejectsInvalidKeyFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, keyFileName), []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileStore(dir); err == nil {
		t.Fatal("expected an error for an invalid key file")
	}
}

func TestFileStoreRoundTripAndPersistence(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("accounts", `{"secret":"value"}`); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("apiKey", "sk-test"); err != nil {
		t.Fatal(err)
	}
	// The data file must not contain the plaintext secret.
	raw, err := os.ReadFile(filepath.Join(dir, dataFileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "sk-test") {
		t.Fatal("secret file contains plaintext")
	}

	reopened, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Get("accounts")
	if err != nil || got != `{"secret":"value"}` {
		t.Fatalf("persisted value = %q, %v", got, err)
	}
	if err := reopened.Delete("apiKey"); err != nil {
		t.Fatal(err)
	}
	if got, _ := reopened.Get("apiKey"); got != "" {
		t.Fatalf("deleted value = %q", got)
	}
}
