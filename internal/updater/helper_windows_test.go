//go:build windows

package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceExecutableNativeHelper(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "MyGoLuncher.exe")
	source := filepath.Join(dir, "MyGoLuncher-new.exe")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("new"))
	if err := replaceExecutable(0, target, source, hex.EncodeToString(sum[:]), func(string, ...any) {}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("target = %q", got)
	}
	backup, err := os.ReadFile(target + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != "old" {
		t.Fatalf("backup = %q", backup)
	}
}

func TestReplaceExecutableRestoresOnHashMismatch(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "MyGoLuncher.exe")
	source := filepath.Join(dir, "MyGoLuncher-new.exe")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := replaceExecutable(0, target, source, "0000000000000000000000000000000000000000000000000000000000000000", func(string, ...any) {})
	if err == nil {
		t.Fatal("expected a hash mismatch")
	}
	got, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "old" {
		t.Fatalf("target was not restored: %q", got)
	}
}

func TestRunHelperIfRequestedFlagDetection(t *testing.T) {
	if RunHelperIfRequested(nil) {
		t.Fatal("normal launch must start the UI")
	}
	if !RunHelperIfRequested([]string{"--mgl-apply-update"}) {
		t.Fatal("helper flag must prevent the UI from starting")
	}
}
