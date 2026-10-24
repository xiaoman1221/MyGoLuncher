package httpx

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadAndVerify(t *testing.T) {
	payload := []byte("downloaded content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "18")
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	sum := sha512.Sum512(payload)
	dest := filepath.Join(t.TempDir(), "files", "example.bin")
	err := Download(context.Background(), File{
		URL:    server.URL + "/example.bin",
		Dest:   dest,
		SHA512: hex.EncodeToString(sum[:]),
		Size:   int64(len(payload)),
		Verify: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("content mismatch: %q", data)
	}

	// A second download with Verify should accept the cached file.
	if err := Download(context.Background(), File{
		URL: "http://127.0.0.1:1/unreachable", Dest: dest,
		SHA512: hex.EncodeToString(sum[:]), Verify: true,
	}); err != nil {
		t.Fatalf("cached file should be reused: %v", err)
	}
}

func TestDownloadChecksumMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("wrong"))
	}))
	defer server.Close()
	dest := filepath.Join(t.TempDir(), "bad.bin")
	err := Download(context.Background(), File{
		URL: server.URL, Dest: dest,
		SHA512: "0000000000000000000000000000000000000000000000000000000000000000" +
			"0000000000000000000000000000000000000000000000000000000000000000",
	})
	if err == nil {
		t.Fatal("expected checksum failure")
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Fatal("invalid file must not be written to its final path")
	}
}

func TestDownloadAnyFallsBack(t *testing.T) {
	payload := []byte("mirror payload")
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer good.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "broken", http.StatusBadGateway)
	}))
	defer bad.Close()

	sum := sha512.Sum512(payload)
	dest := filepath.Join(t.TempDir(), "fallback.bin")
	err := DownloadAny(context.Background(), []string{bad.URL, good.URL}, File{
		Dest: dest, SHA512: hex.EncodeToString(sum[:]), Size: int64(len(payload)),
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("unexpected fallback content: %q", data)
	}
}
