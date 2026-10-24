package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestOAuth2SiteBase(t *testing.T) {
	cases := map[string]string{
		"https://yss.1v.fit":                      "https://yss.1v.fit",
		"https://yss.1v.fit/":                     "https://yss.1v.fit",
		"https://yss.1v.fit/api/yggdrasil/":       "https://yss.1v.fit",
		"https://yss.1v.fit/api/v1/oauth2/token":  "https://yss.1v.fit",
		"https://example.com/site/api/yggdrasil/": "https://example.com/site",
	}
	for input, want := range cases {
		got, err := oauth2SiteBase(input)
		if err != nil {
			t.Fatalf("oauth2SiteBase(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("oauth2SiteBase(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestYSSDeviceFlowAndRefresh(t *testing.T) {
	tokenCalls := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(OAuth2Metadata{
				Issuer:                      server.URL,
				AuthorizationEndpoint:       server.URL + "/oauth/authorize",
				TokenEndpoint:               server.URL + "/token",
				DeviceAuthorizationEndpoint: server.URL + "/device",
				YggdrasilAPILocation:        server.URL + "/api/yggdrasil/",
			})
		case "/device":
			_ = json.NewEncoder(w).Encode(DeviceAuthorization{
				DeviceCode:              "device-code",
				UserCode:                "ABCD-EFGH",
				VerificationURI:         server.URL + "/oauth/device",
				VerificationURIComplete: server.URL + "/oauth/device?user_code=ABCD-EFGH",
				ExpiresIn:               10,
				Interval:                1,
			})
		case "/token":
			_ = r.ParseForm()
			switch r.Form.Get("grant_type") {
			case "urn:ietf:params:oauth:grant-type:device_code":
				tokenCalls++
				if tokenCalls == 1 {
					w.WriteHeader(http.StatusBadRequest)
					_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
					return
				}
			case "refresh_token":
			default:
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "unsupported_grant_type"})
				return
			}
			_ = json.NewEncoder(w).Encode(OAuth2Tokens{
				AccessToken:  "site-token",
				TokenType:    "Bearer",
				ExpiresIn:    3600,
				RefreshToken: "refresh-token",
				Scope:        "openid profile yggdrasil",
				Yggdrasil: &OAuth2Yggdrasil{
					AccessToken: "ygg-token",
					ClientToken: "yss-launcher",
					SelectedProfile: &oauth2Profile{
						ID:   "069a79f4-44e9-4726-a5be-fca90e38aaf5",
						Name: "Steve",
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	meta, err := DiscoverYSSOAuth2(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, device, err := BeginYSSDevice(context.Background(), server.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	if device.UserCode != "ABCD-EFGH" || device.VerificationURIComplete == "" {
		t.Fatalf("unexpected device response: %#v", device)
	}

	account, err := PollYSSDevice(context.Background(), meta, device, "")
	if err != nil {
		t.Fatal(err)
	}
	if account.Kind != KindAuthlib || !account.OAuth2 || account.Name != "Steve" {
		t.Fatalf("unexpected account: %#v", account)
	}
	if account.AccessToken != "ygg-token" || account.ClientToken != "yss-launcher" ||
		account.RefreshToken != "refresh-token" || account.ServerURL != server.URL+"/api/yggdrasil" {
		t.Fatalf("unexpected tokens: %#v", account)
	}

	refreshed, err := RefreshYSSAccount(context.Background(), account)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.AccessToken != "ygg-token" || refreshed.RefreshToken != "refresh-token" {
		t.Fatalf("unexpected refreshed account: %#v", refreshed)
	}
}

func TestYSSOAuthDisabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	_, _, err := BeginYSSDevice(context.Background(), server.URL, "")
	if err == nil || !strings.Contains(err.Error(), "未启用") {
		t.Fatalf("expected a disabled-OAuth2 error, got %v", err)
	}
}

func TestOAuth2URLAllowed(t *testing.T) {
	meta := OAuth2Metadata{
		Issuer:               "https://yss.example.com",
		TokenEndpoint:        "https://yss.example.com/api/v1/oauth2/token",
		YggdrasilAPILocation: "https://yss.example.com/api/yggdrasil/",
	}
	if !oauth2URLAllowed(meta, "https://yss.example.com/oauth/device") {
		t.Fatal("same-host URL should be allowed")
	}
	if oauth2URLAllowed(meta, "https://evil.example.com/oauth/device") {
		t.Fatal("foreign host should be rejected")
	}
	if oauth2URLAllowed(meta, "javascript:alert(1)") {
		t.Fatal("non-http URL should be rejected")
	}
}

func TestOAuth2ResolveRelative(t *testing.T) {
	base := "https://example.com/site"
	if got := oauth2Resolve(base, "/api/v1/oauth2/token"); got != "https://example.com/api/v1/oauth2/token" {
		t.Fatalf("relative endpoint = %q", got)
	}
	if got := oauth2Resolve(base, "https://other.example.com/token"); got != "https://other.example.com/token" {
		t.Fatalf("absolute endpoint = %q", got)
	}
	u, err := url.Parse("https://example.com/site")
	if err != nil || u.Host == "" {
		t.Fatal(err)
	}
}
