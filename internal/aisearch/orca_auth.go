package aisearch

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"MyMinecraftGoLuncher/internal/httpx"
)

// OrcaLoginOptions configures the OrcaRouter PKCE one-click login.
type OrcaLoginOptions struct {
	AuthURL     string
	TokenURL    string
	AppName     string
	Ref         string
	HTTPClient  *http.Client
	OpenBrowser func(authURL string) error
}

// OrcaLoginResult contains the API key returned by the token exchange. It is
// passed only to the app layer and must never be sent to the frontend.
type OrcaLoginResult struct {
	APIKey string
}

type orcaCallback struct {
	key string
	err string
}

// BeginOrcaLogin opens the OrcaRouter authorization page, waits for the
// loopback callback, validates state, and exchanges the code for an API key.
func BeginOrcaLogin(ctx context.Context, opts OrcaLoginOptions) (OrcaLoginResult, error) {
	if opts.AuthURL == "" {
		opts.AuthURL = OrcaAuthURL
	}
	if opts.TokenURL == "" {
		opts.TokenURL = OrcaTokenURL
	}
	if opts.AppName == "" {
		opts.AppName = OrcaAppName
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = httpx.Client
	}
	verifier, err := randomURLToken(32)
	if err != nil {
		return OrcaLoginResult{}, fmt.Errorf("生成授权校验串失败: %w", err)
	}
	state, err := randomURLToken(24)
	if err != nil {
		return OrcaLoginResult{}, fmt.Errorf("生成授权状态失败: %w", err)
	}
	challenge := pkceChallenge(verifier)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return OrcaLoginResult{}, fmt.Errorf("无法启动本地授权回调: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/cb", port)

	callbacks := make(chan orcaCallback, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/cb", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		host, _, hostErr := net.SplitHostPort(r.Host)
		if hostErr != nil || (host != "127.0.0.1" && host != "localhost") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		query := r.URL.Query()
		gotState := query.Get("state")
		if len(gotState) > 512 || len(query.Get("code")) > 4096 {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if subtle.ConstantTimeCompare([]byte(gotState), []byte(state)) != 1 {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}
		if message := query.Get("error"); message != "" {
			description := firstNonEmpty(query.Get("error_description"), message)
			select {
			case callbacks <- orcaCallback{err: description}:
			default:
			}
			writeOrcaPage(w, false)
			return
		}
		code := strings.TrimSpace(query.Get("code"))
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}
		exchangeCtx, cancelExchange := context.WithTimeout(ctx, 25*time.Second)
		apiKey, exchangeErr := exchangeOrcaCode(exchangeCtx, opts, code, verifier)
		cancelExchange()
		if exchangeErr != nil {
			select {
			case callbacks <- orcaCallback{err: exchangeErr.Error()}:
			default:
			}
			writeOrcaPage(w, false)
			return
		}
		select {
		case callbacks <- orcaCallback{key: apiKey}:
		default:
		}
		writeOrcaPage(w, true)
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = server.Serve(listener) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	authURL, err := buildOrcaAuthURL(opts, callbackURL, challenge, state)
	if err != nil {
		return OrcaLoginResult{}, err
	}
	if opts.OpenBrowser == nil {
		return OrcaLoginResult{}, errors.New("无法打开系统浏览器")
	}
	if err := opts.OpenBrowser(authURL); err != nil {
		return OrcaLoginResult{}, fmt.Errorf("无法打开系统浏览器: %w", err)
	}

	var callback orcaCallback
	select {
	case <-ctx.Done():
		return OrcaLoginResult{}, fmt.Errorf("OrcaRouter 授权已取消或超时: %w", ctx.Err())
	case callback = <-callbacks:
	}
	if callback.err != "" {
		return OrcaLoginResult{}, fmt.Errorf("OrcaRouter 授权失败: %s", callback.err)
	}
	if callback.key == "" {
		return OrcaLoginResult{}, errors.New("OrcaRouter 未返回 API Key")
	}
	return OrcaLoginResult{APIKey: callback.key}, nil
}

func buildOrcaAuthURL(opts OrcaLoginOptions, callbackURL, challenge, state string) (string, error) {
	authorizeURL, err := url.Parse(opts.AuthURL)
	if err != nil {
		return "", fmt.Errorf("OrcaRouter 授权地址无效: %w", err)
	}
	query := authorizeURL.Query()
	query.Set("callback_url", callbackURL)
	query.Set("code_challenge", challenge)
	query.Set("code_challenge_method", "S256")
	query.Set("state", state)
	query.Set("app_name", opts.AppName)
	query.Set("scope", "api")
	if strings.TrimSpace(opts.Ref) != "" {
		query.Set("ref", strings.TrimSpace(opts.Ref))
	}
	authorizeURL.RawQuery = query.Encode()
	return authorizeURL.String(), nil
}

func exchangeOrcaCode(ctx context.Context, opts OrcaLoginOptions, code, verifier string) (string, error) {
	payload, err := json.Marshal(map[string]string{
		"code":          code,
		"code_verifier": verifier,
	})
	if err != nil {
		return "", err
	}
	requestCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, opts.TokenURL, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", httpx.UserAgent)
	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("OrcaRouter 换取密钥失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("OrcaRouter 换取密钥失败: %s", orcaExchangeError(body, resp.Status))
	}
	key := extractOrcaKey(body)
	if key == "" {
		return "", errors.New("OrcaRouter 未返回 API Key")
	}
	return key, nil
}

func extractOrcaKey(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return ""
	}
	var text string
	if json.Unmarshal(body, &text) == nil {
		return cleanOrcaKey(text)
	}
	var value any
	if json.Unmarshal(body, &value) == nil {
		if key := findOrcaKey(value, 0); key != "" {
			return cleanOrcaKey(key)
		}
	}
	if !strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[") && !strings.HasPrefix(trimmed, "\"") {
		return cleanOrcaKey(trimmed)
	}
	return ""
}

func findOrcaKey(value any, depth int) string {
	if depth > 4 {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case map[string]any:
		for _, name := range []string{"key", "api_key", "apiKey", "token", "access_token", "secret"} {
			if text, ok := typed[name].(string); ok && strings.TrimSpace(text) != "" {
				return text
			}
		}
		for _, name := range []string{"data", "result"} {
			if child, ok := typed[name]; ok {
				if key := findOrcaKey(child, depth+1); key != "" {
					return key
				}
			}
		}
	}
	return ""
}

func cleanOrcaKey(value string) string {
	value = strings.TrimSpace(strings.Trim(value, "\"'"))
	if len(value) < 16 || len(value) > 4096 {
		return ""
	}
	if strings.ContainsAny(value, " \t\r\n<>") {
		return ""
	}
	switch strings.ToLower(value) {
	case "success", "authorized", "unauthorized", "true", "false", "ok":
		return ""
	}
	return value
}

func orcaExchangeError(body []byte, status string) string {
	var payload struct {
		Error     any    `json:"error"`
		Message   string `json:"message"`
		Detail    string `json:"detail"`
		ErrorText string `json:"error_description"`
	}
	_ = json.Unmarshal(body, &payload)
	message := firstNonEmpty(payload.Message, payload.Detail, payload.ErrorText)
	switch value := payload.Error.(type) {
	case string:
		message = firstNonEmpty(message, value)
	case map[string]any:
		if text, ok := value["message"].(string); ok {
			message = firstNonEmpty(message, text)
		}
	}
	if message == "" {
		message = strings.TrimSpace(string(body))
	}
	if message == "" {
		message = status
	}
	runes := []rune(message)
	if len(runes) > 300 {
		message = string(runes[:300])
	}
	return message
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomURLToken(size int) (string, error) {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func writeOrcaPage(w http.ResponseWriter, success bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	message := "授权失败，请返回启动器重试。"
	if success {
		message = "授权成功，请返回启动器。"
	}
	_, _ = io.WriteString(w, "<!doctype html><html><head><meta charset=\"utf-8\"><title>OrcaRouter</title></head><body style=\"font-family:sans-serif;padding:40px;text-align:center\"><h2>"+message+"</h2><p>You can close this page.</p></body></html>")
}
