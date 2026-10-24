package aisearch

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestTranslateOrcaRouter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer orca-test-key" {
			t.Errorf("unexpected authorization: %s", r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		text := string(body)
		if !strings.Contains(text, "orcarouter/auto") || !strings.Contains(text, "钠") ||
			!strings.Contains(text, "instructions") || !strings.Contains(text, "input") {
			t.Errorf("unexpected request: %s", text)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output": []map[string]any{{
				"type": "message",
				"content": []map[string]string{{
					"type": "output_text", "text": "\"sodium optimization\"",
				}},
			}},
		})
	}))
	defer server.Close()

	got, err := TranslateQuery(context.Background(), Config{
		Provider: ProviderOrcaRouter,
		APIKey:   "orca-test-key",
		BaseURL:  server.URL + "/v1",
		Model:    DefaultOrcaRouterModel,
	}, "钠 优化")
	if err != nil {
		t.Fatal(err)
	}
	if got != "sodium optimization" {
		t.Fatalf("translated query = %q", got)
	}
}

func TestTranslateOrcaRouterEnvKeyAndOutputText(t *testing.T) {
	t.Setenv(OrcaRouterEnvKey, "orca-env-key")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer orca-env-key" {
			t.Errorf("env key not used: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"output_text": "sodium"})
	}))
	defer server.Close()

	got, err := TranslateQuery(context.Background(), Config{
		Provider: ProviderOrcaRouter,
		BaseURL:  server.URL,
		Model:    DefaultOrcaRouterModel,
	}, "钠")
	if err != nil {
		t.Fatal(err)
	}
	if got != "sodium" {
		t.Fatalf("translated query = %q", got)
	}
}

func TestOrcaProviderDefaults(t *testing.T) {
	if DefaultModelForProvider(ProviderOrcaRouter) != "orcarouter/auto" {
		t.Fatal("unexpected OrcaRouter default model")
	}
	if config, ok := LookupProvider("orcarouter"); !ok || config.WireAPI != "responses" || config.EnvKey != "ORCA_KEY" {
		t.Fatalf("provider config = %#v, %v", config, ok)
	}
	if ModelProvider("orcarouter/auto") != ProviderOrcaRouter {
		t.Fatal("model provider detection failed")
	}
}

func TestOrcaLoginPKCE(t *testing.T) {
	var gotCode, gotVerifier string
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/keys" {
			t.Errorf("unexpected token path: %s", r.URL.Path)
		}
		var payload struct {
			Code         string `json:"code"`
			CodeVerifier string `json:"code_verifier"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		gotCode = payload.Code
		gotVerifier = payload.CodeVerifier
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"key": "orca-key-1234567890abcdef"}})
	}))
	defer tokenServer.Close()

	authURLCh := make(chan string, 1)
	resultCh := make(chan struct {
		result OrcaLoginResult
		err    error
	}, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() {
		result, err := BeginOrcaLogin(ctx, OrcaLoginOptions{
			AuthURL:  "https://www.arouter.ai/auth",
			TokenURL: tokenServer.URL + "/api/v1/auth/keys",
			AppName:  "TestApp",
			Ref:      "ref_test",
			OpenBrowser: func(authURL string) error {
				authURLCh <- authURL
				return nil
			},
		})
		resultCh <- struct {
			result OrcaLoginResult
			err    error
		}{result, err}
	}()

	authURL := <-authURLCh
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	state := query.Get("state")
	challenge := query.Get("code_challenge")
	callbackURL := query.Get("callback_url")
	if state == "" || challenge == "" || callbackURL == "" {
		t.Fatalf("missing auth parameters: %s", authURL)
	}
	if query.Get("code_challenge_method") != "S256" || query.Get("app_name") != "TestApp" || query.Get("ref") != "ref_test" {
		t.Fatalf("unexpected auth query: %s", authURL)
	}

	// A callback with the wrong state must be rejected and must not complete.
	wrongURL := callbackURL + "?code=bad&state=wrong"
	resp, err := http.Get(wrongURL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("wrong state status = %d", resp.StatusCode)
	}
	select {
	case result := <-resultCh:
		t.Fatalf("wrong state completed the login: %#v", result)
	case <-time.After(150 * time.Millisecond):
	}

	goodURL := callbackURL + "?code=test-code&state=" + url.QueryEscape(state)
	resp, err = http.Get(goodURL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("callback status = %d", resp.StatusCode)
	}

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.result.APIKey != "orca-key-1234567890abcdef" {
			t.Fatalf("API key = %q", result.result.APIKey)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("login did not finish")
	}
	if gotCode != "test-code" {
		t.Fatalf("exchanged code = %q", gotCode)
	}
	sum := sha256.Sum256([]byte(gotVerifier))
	if base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
		t.Fatal("code_verifier does not match the challenge")
	}
}

func TestExtractOrcaKey(t *testing.T) {
	cases := map[string]string{
		`{"key":"orca-key-1234567890abcdef"}`:              "orca-key-1234567890abcdef",
		`{"data":{"api_key":"orca-key-1234567890abcdef"}}`: "orca-key-1234567890abcdef",
		`"orca-key-1234567890abcdef"`:                      "orca-key-1234567890abcdef",
		"orca-key-1234567890abcdef":                        "orca-key-1234567890abcdef",
		`{"error":"invalid"}`:                              "",
		`{"data":{"message":"authorized successfully"}}`:   "",
	}
	for body, want := range cases {
		if got := extractOrcaKey([]byte(body)); got != want {
			t.Fatalf("extractOrcaKey(%s) = %q, want %q", body, got, want)
		}
	}
}
