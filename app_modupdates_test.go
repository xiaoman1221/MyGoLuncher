package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveOldModKeepsRollbackJar(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "example.jar")
	if err := os.WriteFile(oldPath, []byte("old-mod"), 0o644); err != nil {
		t.Fatal(err)
	}
	rollback, err := archiveOldMod(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	if rollback != oldPath+".old" {
		t.Fatalf("rollback path = %q", rollback)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatal("old jar should have been renamed")
	}
	data, err := os.ReadFile(rollback)
	if err != nil || string(data) != "old-mod" {
		t.Fatalf("rollback contents = %q, err=%v", data, err)
	}
}

func TestArchiveOldModUsesUniqueSuffix(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "example.jar")
	if err := os.WriteFile(oldPath, []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := archiveOldMod(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldPath, []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := archiveOldMod(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	if second == first || second != oldPath+".old.1" {
		t.Fatalf("second rollback path = %q, first = %q", second, first)
	}
	if data, _ := os.ReadFile(first); string(data) != "first" {
		t.Fatalf("first backup overwritten: %q", data)
	}
	if data, _ := os.ReadFile(second); string(data) != "second" {
		t.Fatalf("second backup contents: %q", data)
	}
}
