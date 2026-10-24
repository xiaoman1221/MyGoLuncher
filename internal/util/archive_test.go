package util

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSafeJoinRejectsTraversal(t *testing.T) {
	base := t.TempDir()
	for _, name := range []string{"../escape.txt", "a/../../escape.txt"} {
		if _, err := SafeJoin(base, name); err == nil {
			t.Errorf("SafeJoin(%q) should fail", name)
		}
	}
	target, err := SafeJoin(base, "mods/example.jar")
	if err != nil {
		t.Fatal(err)
	}
	if target != filepath.Join(base, "mods", "example.jar") {
		t.Fatalf("unexpected safe path: %s", target)
	}
	// Absolute-looking entries are normalized inside the destination.
	absolute, err := SafeJoin(base, "/absolute.txt")
	if err != nil {
		t.Fatal(err)
	}
	if absolute != filepath.Join(base, "absolute.txt") {
		t.Fatalf("absolute entry escaped destination: %s", absolute)
	}
}

func TestExtractZipRejectsZipSlip(t *testing.T) {
	var buffer bytes.Buffer
	zw := zip.NewWriter(&buffer)
	w, err := zw.Create("../escaped.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("bad")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "evil.zip")
	if err := os.WriteFile(archive, buffer.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "out")
	if err := ExtractZip(archive, dest, ""); err == nil {
		t.Fatal("expected zip traversal to be rejected")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dest), "escaped.txt")); !os.IsNotExist(err) {
		t.Fatal("zip traversal wrote outside the destination")
	}
}

func TestExtractZipPrefix(t *testing.T) {
	var buffer bytes.Buffer
	zw := zip.NewWriter(&buffer)
	for name, data := range map[string]string{
		"pack/config/a.txt": "a",
		"pack/mods/b.jar":   "b",
		"other/c.txt":       "c",
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "pack.zip")
	if err := os.WriteFile(archive, buffer.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "out")
	if err := ExtractZip(archive, dest, "pack"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"config/a.txt", "mods/b.jar"} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(name))); err != nil {
			t.Fatalf("expected %s to be extracted: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dest, "other")); !os.IsNotExist(err) {
		t.Fatal("content outside the prefix should not be extracted")
	}
}
