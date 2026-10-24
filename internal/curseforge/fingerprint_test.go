package curseforge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFingerprintFiltersWhitespace(t *testing.T) {
	base := []byte("curseforge")
	withWhitespace := []byte("curse\tforge\n")
	if Fingerprint(base) != Fingerprint(withWhitespace) {
		t.Fatal("whitespace should not affect CurseForge fingerprint")
	}
	if Fingerprint(nil) != Fingerprint([]byte{}) {
		t.Fatal("empty content should be deterministic")
	}
	if Fingerprint([]byte("a")) == Fingerprint([]byte("b")) {
		t.Fatal("different contents should have different fingerprints")
	}
}

func TestFingerprintMatches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/fingerprints" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("x-api-key") != "key" {
			http.Error(w, "missing key", http.StatusForbidden)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"exactMatches": []map[string]any{{
					"id":   42,
					"file": map[string]any{"id": 100, "modId": 42, "fileName": "example.jar", "fileLength": 12},
				}},
			},
		})
	}))
	defer server.Close()
	client := New("key")
	client.baseURL = server.URL
	matches, err := client.FingerprintMatches(context.Background(), []uint32{123})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].File.ID != 100 || matches[0].ID != 42 {
		t.Fatalf("matches = %#v", matches)
	}
}
