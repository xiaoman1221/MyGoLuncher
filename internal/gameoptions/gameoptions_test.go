package gameoptions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLanguageCode(t *testing.T) {
	cases := map[string]string{
		"zh-CN": "zh_cn",
		"zh-TW": "zh_tw",
		"en":    "en_us",
		"ja":    "ja_jp",
		"ko":    "ko_kr",
		"fr":    "",
	}
	for input, want := range cases {
		if got := LanguageCode(input); got != want {
			t.Fatalf("LanguageCode(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSetLanguagePreservesOtherOptions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "options.txt")
	if err := os.WriteFile(path, []byte("lang:en_us\nfov:70\nrenderDistance:12\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, err := SetLanguage(dir, "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if code != "zh_cn" {
		t.Fatalf("code = %q", code)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "lang:zh_cn\n") || !strings.Contains(text, "fov:70") || !strings.Contains(text, "renderDistance:12") {
		t.Fatalf("unexpected options.txt: %q", text)
	}
}

func TestSetLanguageCreatesFile(t *testing.T) {
	dir := t.TempDir()
	code, err := SetLanguage(dir, "ja")
	if err != nil {
		t.Fatal(err)
	}
	if code != "ja_jp" {
		t.Fatalf("code = %q", code)
	}
	data, err := os.ReadFile(filepath.Join(dir, "options.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "lang:ja_jp" {
		t.Fatalf("unexpected options.txt: %q", data)
	}
}

func TestSetLanguagePreservesCRLF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "options.txt")
	if err := os.WriteFile(path, []byte("lang:en_us\r\nfov:70\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := SetLanguage(dir, "ko"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "lang:ko_kr\r\n") || !strings.Contains(string(data), "fov:70\r\n") {
		t.Fatalf("CRLF not preserved: %q", data)
	}
}

func TestSetLanguageUnknownDoesNothing(t *testing.T) {
	dir := t.TempDir()
	if code, err := SetLanguage(dir, "fr"); err != nil || code != "" {
		t.Fatalf("unexpected result: %q %v", code, err)
	}
}
