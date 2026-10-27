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
	status, err := manager.Activate(context.Background(), "TEST-CODE", "buyer@example.com", "Acme Inc")
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

func TestManagerEmailUnbind(t *testing.T) {
	useTestProduct(t)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	var requestEmail, requestCustomer string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/activation/activate":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			requestEmail, _ = body["email"].(string)
			requestCustomer, _ = body["customer"].(string)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "active", "features": []string{"launch"}, "unbind_key": "unbind-1",
			})
		case "/api/v1/activation/unbind/request":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "sent", "message": "验证码已发送"})
		case "/api/v1/activation/unbind/confirm":
			_ = json.NewEncoder(w).Encode(map[string]any{"valid": true, "status": "unbound", "message": "解绑成功"})
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
	status, err := manager.Activate(context.Background(), "TEST-CODE", "Buyer@Example.com", " Acme Inc ")
	if err != nil {
		t.Fatal(err)
	}
	if requestEmail != "buyer@example.com" || requestCustomer != "Acme Inc" {
		t.Fatalf("activation identity = %q / %q", requestEmail, requestCustomer)
	}
	if status.Email != "buyer@example.com" || status.Customer != "Acme Inc" || !status.CanSelfUnbind {
		t.Fatalf("activation status = %#v", status)
	}
	if _, err := manager.RequestEmailUnbind(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	unbound, err := manager.ConfirmEmailUnbind(context.Background(), "buyer@example.com", "123456")
	if err != nil {
		t.Fatal(err)
	}
	if unbound.State != "inactive" || manager.Active() {
		t.Fatalf("email unbind status = %#v", unbound)
	}
	if raw, _ := secrets.Get(secretName); raw != "" {
		t.Fatal("email unbind did not delete local credentials")
	}
}

func TestActivationWithoutUnbindKeyStillActive(t *testing.T) {
	useTestProduct(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "active", "features": []string{}, "unbind_key": nil,
		})
	}))
	defer server.Close()
	oldURL := ServerURL
	ServerURL = server.URL
	defer func() { ServerURL = oldURL }()

	manager, err := NewManager(secretstore.NewMemory(), "test")
	if err != nil {
		t.Fatal(err)
	}
	status, err := manager.Activate(context.Background(), "TEST-CODE", "buyer@example.com", "Acme Inc")
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "active" || status.CanSelfUnbind {
		t.Fatalf("activation without unbind key = %#v", status)
	}
}

func TestActivationRequiresIdentity(t *testing.T) {
	manager, err := NewManager(secretstore.NewMemory(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Activate(context.Background(), "TEST-CODE", "", "Acme"); err == nil {
		t.Fatal("expected email validation error")
	}
	if _, err := manager.Activate(context.Background(), "TEST-CODE", "buyer@example.com", ""); err == nil {
		t.Fatal("expected customer validation error")
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
	if _, err := manager.Activate(context.Background(), "TEST-CODE", "buyer@example.com", "Acme Inc"); err != nil {
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
