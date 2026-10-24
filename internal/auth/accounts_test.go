package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"MyMinecraftGoLuncher/internal/secretstore"
)

func TestAccountStoreLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	store, err := Open(path, "Steve")
	if err != nil {
		t.Fatal(err)
	}
	if len(store.List()) != 1 || store.List()[0].Kind != KindOffline {
		t.Fatalf("expected one offline account: %#v", store.List())
	}
	original := store.List()[0]
	if store.List()[0].UUID != OfflineUUID("Steve") {
		t.Fatalf("offline UUID mismatch: %s", store.List()[0].UUID)
	}

	alex, err := store.AddOfflineAccount("Alex")
	if err != nil {
		t.Fatal(err)
	}
	if len(store.List()) != 2 || store.SelectedID() != alex.ID {
		t.Fatalf("second offline account was not added/selected: %#v", store.List())
	}
	offline := store.List()[0]
	renamed, err := store.RenameOfflineAccount(offline.ID, "Steve2")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "Steve2" || renamed.UUID != OfflineUUID("Steve2") {
		t.Fatalf("offline rename failed: %#v", renamed)
	}
	if _, err := store.RenameOfflineAccount(alex.ID, "Steve2"); err == nil {
		t.Fatal("duplicate offline names should be rejected")
	}
	// Adding the same offline name selects the existing identity.
	again, err := store.AddOfflineAccount("Alex")
	if err != nil || again.ID != alex.ID {
		t.Fatalf("duplicate offline add should select the existing profile: %#v %v", again, err)
	}

	msaA, err := store.Add(Account{
		Kind: KindMicrosoft, Name: "CraftPlayerA",
		UUID: "069a79f444e94726a5befca90e38aaf5", AccessToken: "mc", RefreshToken: "refresh",
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.SelectedID() != msaA.ID {
		t.Fatal("new Microsoft account was not selected")
	}
	msaB, err := store.Add(Account{
		Kind: KindMicrosoft, Name: "CraftPlayerB",
		UUID: "11111111111111111111111111111111", AccessToken: "mc2", RefreshToken: "refresh2",
	})
	if err != nil || msaB.ID == msaA.ID {
		t.Fatalf("second Microsoft account failed: %#v %v", msaB, err)
	}
	// Re-authenticating the same Microsoft identity updates the existing row.
	merged, err := store.Add(Account{
		Kind: KindMicrosoft, Name: "CraftPlayerA", UUID: "069a79f4-44e9-4726-a5be-fca90e38aaf5",
		AccessToken: "mc3", RefreshToken: "refresh3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if merged.ID != msaA.ID {
		t.Fatalf("duplicate Microsoft account was not merged: %#v", store.List())
	}
	authlib, err := store.Add(Account{
		Kind: KindAuthlib, Name: "ThirdParty", UUID: "22222222222222222222222222222222",
		AccessToken: "third", ClientToken: "client", ServerURL: "https://example.com/api/yggdrasil",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(store.List()) != 5 || store.SelectedID() != authlib.ID {
		t.Fatalf("unexpected account set: %#v", store.List())
	}

	reopened, err := Open(path, "Ignored")
	if err != nil {
		t.Fatal(err)
	}
	if len(reopened.List()) != 5 || reopened.SelectedID() != authlib.ID {
		t.Fatalf("accounts did not persist: %#v", reopened.List())
	}

	// Remove every offline account. With other accounts remaining, reopening
	// must not silently recreate an offline account.
	for _, item := range reopened.List() {
		if item.Kind == KindOffline {
			if err := reopened.Remove(item.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(reopened.List()) != 3 {
		t.Fatalf("expected only premium accounts, got %#v", reopened.List())
	}
	reopened, err = Open(path, "Ignored")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range reopened.List() {
		if item.Kind == KindOffline {
			t.Fatalf("offline account was unexpectedly recreated: %#v", reopened.List())
		}
	}

	// Any account type can be removed, but the final account is protected.
	for len(reopened.List()) > 1 {
		if err := reopened.Remove(reopened.SelectedID()); err != nil {
			t.Fatal(err)
		}
	}
	if len(reopened.List()) != 1 {
		t.Fatalf("expected one remaining account: %#v", reopened.List())
	}
	if err := reopened.Remove(reopened.List()[0].ID); err == nil {
		t.Fatal("the final account must not be removable")
	}
	_ = original
}

func TestNormalizeAuthlibURL(t *testing.T) {
	cases := map[string]string{
		"https://littleskin.cn/api/yggdrasil/":                      "https://littleskin.cn/api/yggdrasil",
		"https://example.com/api/yggdrasil/authserver/authenticate": "https://example.com/api/yggdrasil",
		"littleskin.cn/api/yggdrasil":                               "https://littleskin.cn/api/yggdrasil",
		"http://127.0.0.1:8080/":                                    "http://127.0.0.1:8080",
		"https://example.com/authserver":                            "https://example.com",
	}
	for input, want := range cases {
		got, err := NormalizeAuthlibURL(input)
		if err != nil {
			t.Errorf("NormalizeAuthlibURL(%q) error: %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("NormalizeAuthlibURL(%q) = %q, want %q", input, got, want)
		}
	}
	if _, err := NormalizeAuthlibURL("ftp://example.com"); err == nil {
		t.Fatal("expected unsupported scheme to fail")
	}
}

func TestAuthlibLoginAndRefresh(t *testing.T) {
	var refreshed bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/yggdrasil/authserver/authenticate":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode auth body: %v", err)
			}
			if payload["username"] != "alex" || payload["password"] != "secret" {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"ForbiddenOperationException","errorMessage":"Bad credentials"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accessToken": "access-1", "clientToken": "client-1",
				"selectedProfile": map[string]string{"id": "069a79f444e94726a5befca90e38aaf5", "name": "Alex"},
			})
		case "/api/yggdrasil/authserver/validate":
			w.WriteHeader(http.StatusNoContent)
		case "/api/yggdrasil/authserver/refresh":
			refreshed = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accessToken": "access-2", "clientToken": "client-1",
				"selectedProfile": map[string]string{"id": "069a79f444e94726a5befca90e38aaf5", "name": "Alex"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	account, err := LoginAuthlib(context.Background(), server.URL+"/api/yggdrasil", "alex", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if account.Kind != KindAuthlib || account.Name != "Alex" || account.AccessToken != "access-1" || account.ServerURL != server.URL+"/api/yggdrasil" {
		t.Fatalf("unexpected account: %#v", account)
	}
	if valid, err := ValidateAuthlib(context.Background(), account); err != nil || !valid {
		t.Fatalf("validation failed: %v %v", valid, err)
	}
	refreshedAccount, err := RefreshAuthlib(context.Background(), account)
	if err != nil {
		t.Fatal(err)
	}
	if !refreshed || refreshedAccount.AccessToken != "access-2" {
		t.Fatalf("refresh did not work: %#v", refreshedAccount)
	}
	if _, err := LoginAuthlib(context.Background(), server.URL+"/api/yggdrasil", "alex", "wrong"); err == nil || !strings.Contains(err.Error(), "Bad credentials") {
		t.Fatalf("expected authentication error, got %v", err)
	}
}

func TestOAuthErrorParsing(t *testing.T) {
	message := oauthError([]byte(`{"error":"invalid_grant","error_description":"expired"}`), 400)
	if message != "expired" {
		t.Fatalf("unexpected oauth error: %q", message)
	}
	message = oauthError([]byte(`{"error":"ForbiddenOperationException","errorMessage":"Bad credentials"}`), 403)
	if message != "Bad credentials" {
		t.Fatalf("unexpected authlib error: %q", message)
	}
}

func TestSameAuthlibServer(t *testing.T) {
	if !sameAuthlibServer("https://yss.1v.fit", "https://yss.1v.fit/api/yggdrasil/") {
		t.Fatal("site root and API location should match")
	}
	if sameAuthlibServer("https://yss.1v.fit", "https://littleskin.cn/api/yggdrasil") {
		t.Fatal("different hosts should not match")
	}
}

func TestSameAuthlibIdentityUpdatesExistingAccount(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "accounts.json"), "Steve")
	if err != nil {
		t.Fatal(err)
	}
	passwordAccount, err := store.Add(Account{
		Kind: KindAuthlib, Name: "Steve", UUID: "abc123",
		ServerURL: "https://yss.1v.fit", AccessToken: "password-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	oauthAccount, err := store.Add(Account{
		Kind: KindAuthlib, Name: "Steve", UUID: "abc123",
		ServerURL: "https://yss.1v.fit/api/yggdrasil", AccessToken: "oauth-token",
		OAuth2: true, OAuth2TokenEndpoint: "https://yss.1v.fit/api/v1/oauth2/token",
	})
	if err != nil {
		t.Fatal(err)
	}
	if oauthAccount.ID != passwordAccount.ID {
		t.Fatalf("OAuth2 login should update the existing identity: %s != %s", oauthAccount.ID, passwordAccount.ID)
	}
	if count := len(store.List()); count != 2 {
		t.Fatalf("expected offline + authlib account, got %d", count)
	}
}

func TestAccountsMigrateToSecretStore(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "accounts.json")
	raw := `{"accounts":[{"id":"x","kind":"microsoft","name":"Steve","uuid":"abc","accessToken":"secret-token"}],"selectedId":"x"}`
	if err := os.WriteFile(legacy, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	secrets := secretstore.NewMemory()
	store, err := OpenWithSecrets(secrets, legacy, "Steve")
	if err != nil {
		t.Fatal(err)
	}
	if accounts := store.List(); len(accounts) != 1 || accounts[0].Name != "Steve" {
		t.Fatalf("unexpected accounts: %#v", accounts)
	}
	stored, err := secrets.Get(accountsSecretName)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stored, "secret-token") {
		t.Fatalf("account tokens were not moved to the secret store: %s", stored)
	}
	if _, err := os.Stat(legacy); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy accounts.json still exists: %v", err)
	}

	reopened, err := OpenWithSecrets(secrets, legacy, "Ignored")
	if err != nil {
		t.Fatal(err)
	}
	if accounts := reopened.List(); len(accounts) != 1 || accounts[0].Name != "Steve" {
		t.Fatalf("encrypted accounts were not persisted: %#v", accounts)
	}
}

func TestOfflineUUID(t *testing.T) {
	first := OfflineUUID("Steve")
	second := OfflineUUID("Steve")
	if first != second || len(first) != 32 {
		t.Fatalf("offline UUID is not stable: %s %s", first, second)
	}
	if OfflineUUID("Alex") == first {
		t.Fatal("different player names should produce different UUIDs")
	}
	// Java sets the version-3 nibble and RFC 4122 variant.
	if first[12] != '3' {
		t.Fatalf("UUID version nibble should be 3: %s", first)
	}
	if !strings.Contains("89ab", string(first[16])) {
		t.Fatalf("UUID variant nibble is invalid: %s", first)
	}
}
