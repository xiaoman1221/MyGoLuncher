package license

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"MyMinecraftGoLuncher/internal/secretstore"
)

func useTestProduct(t *testing.T) {
	t.Helper()
	oldKey, oldServer := ProductAPIKey, ProductServerURL
	ProductAPIKey = "lg_test_product_key_0123456789"
	ProductServerURL = "https://license.example.invalid"
	t.Cleanup(func() { ProductAPIKey, ProductServerURL = oldKey, oldServer })
}

func TestManagerActivateValidateUnbind(t *testing.T) {
	useTestProduct(t)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	expires := now.Add(30 * 24 * time.Hour)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") == "" {
			http.Error(w, "missing key", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/v1/activation/activate":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "active", "message": "ok", "expires_at": expires,
				"features": []string{"launch"}, "unbind_key": "unbind-1",
			})
		case "/api/v1/activation/validate":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"valid": true, "status": "active", "expires_at": expires, "features": []string{"launch"},
			})
		case "/api/v1/activation/unbind":
			_ = json.NewEncoder(w).Encode(map[string]any{"valid": true, "status": "inactive", "message": "ok"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	oldURL := ServerURL
	ServerURL = server.URL
	defer func() { ServerURL = oldURL }()

	secrets := secretstore.NewMemory()
	manager, err := NewManager(secrets, "test")
	if err != nil {
		t.Fatal(err)
	}
	manager.now = func() time.Time { return now }
	status, err := manager.Activate(context.Background(), "TEST-CODE")
	if err != nil {
		t.Fatal(err)
	}
	if !manager.Active() || status.State != "active" || status.Code != "TEST-****-CODE" {
		t.Fatalf("activation status = %#v", status)
	}
	if raw, _ := secrets.Get(secretName); raw == "" {
		t.Fatal("activation was not persisted in the secret store")
	}
	validated, err := manager.Validate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !validated.Activated || validated.State != "active" {
		t.Fatalf("validation status = %#v", validated)
	}
	unbound, err := manager.Unbind(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if unbound.State != "inactive" || manager.Active() {
		t.Fatalf("unbind status = %#v", unbound)
	}
	if raw, _ := secrets.Get(secretName); raw != "" {
		t.Fatal("local activation was not deleted")
	}
}

func TestManagerUsesGraceWhenValidationFails(t *testing.T) {
	useTestProduct(t)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/activation/activate" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "active", "expires_at": now.Add(30 * 24 * time.Hour),
				"features": []string{}, "unbind_key": "unbind-1",
			})
			return
		}
		http.Error(w, "offline", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	oldURL := ServerURL
	ServerURL = server.URL
	defer func() { ServerURL = oldURL }()

	manager, err := NewManager(secretstore.NewMemory(), "test")
	if err != nil {
		t.Fatal(err)
	}
	manager.now = func() time.Time { return now }
	if _, err := manager.Activate(context.Background(), "TEST-CODE"); err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	manager.creds.LastValidated = now.Add(-24 * time.Hour)
	manager.mu.Unlock()
	status, err := manager.Validate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "grace" || !manager.Active() || status.GraceUntil == "" {
		t.Fatalf("grace status = %#v", status)
	}
}
