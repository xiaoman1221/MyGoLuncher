package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"MyMinecraftGoLuncher/internal/httpx"
)

// Default YSS OAuth2 settings. YSS uses a public desktop client with PKCE-free
// device-code authorization, so no client secret is needed.
const (
	DefaultYSSBaseURL  = "https://yss.1v.fit"
	DefaultYSSClientID = "yss-launcher"
	yssScope           = "openid profile yggdrasil"
)

// OAuth2Metadata is the subset of RFC 8414 metadata used by the launcher.
type OAuth2Metadata struct {
	Issuer                      string `json:"issuer"`
	AuthorizationEndpoint       string `json:"authorization_endpoint"`
	TokenEndpoint               string `json:"token_endpoint"`
	DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
	UserinfoEndpoint            string `json:"userinfo_endpoint"`
	YggdrasilAPILocation        string `json:"yggdrasil_api_location"`
}

// DeviceAuthorization is the device-code response.
type DeviceAuthorization struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type oauth2Profile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// OAuth2Yggdrasil is the YSS extension containing a ready-to-use Yggdrasil
// session returned by the token endpoint.
type OAuth2Yggdrasil struct {
	AccessToken       string          `json:"accessToken"`
	ClientToken       string          `json:"clientToken"`
	AvailableProfiles []oauth2Profile `json:"availableProfiles"`
	SelectedProfile   *oauth2Profile  `json:"selectedProfile"`
}

// OAuth2Tokens is a successful token-endpoint response.
type OAuth2Tokens struct {
	AccessToken  string           `json:"access_token"`
	TokenType    string           `json:"token_type"`
	ExpiresIn    int              `json:"expires_in"`
	RefreshToken string           `json:"refresh_token"`
	Scope        string           `json:"scope"`
	Yggdrasil    *OAuth2Yggdrasil `json:"yggdrasil"`
}

type oauth2ErrorPayload struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// oauth2SiteBase extracts the site root from an Authlib/OAuth2 server address.
func oauth2SiteBase(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = DefaultYSSBaseURL
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("YSS 授权地址格式无效")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("YSS 授权地址只支持 http/https")
	}
	if parsed.Host == "" {
		return "", errors.New("YSS 授权地址缺少主机名")
	}
	path := strings.TrimRight(parsed.Path, "/")
	for changed := true; changed; {
		changed = false
		for _, suffix := range []string{
			"/.well-known/oauth-authorization-server",
			"/api/v1/oauth2/token",
			"/api/v1/oauth2/device_authorization",
			"/api/v1/oauth2/userinfo",
			"/api/v1/oauth2",
			"/oauth/authorize",
			"/oauth/device",
			"/api/yggdrasil",
			"/authserver",
		} {
			if strings.HasSuffix(path, suffix) {
				path = strings.TrimSuffix(path, suffix)
				changed = true
			}
		}
	}
	parsed.Path = path
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func oauth2Resolve(base, endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return endpoint
	}
	if parsed.IsAbs() {
		return parsed.String()
	}
	root, err := url.Parse(base)
	if err != nil {
		return endpoint
	}
	return root.ResolveReference(parsed).String()
}

// DiscoverYSSOAuth2 reads the server metadata and falls back to YSS's
// conventional endpoint paths when the discovery document is unavailable.
func DiscoverYSSOAuth2(ctx context.Context, serverURL string) (OAuth2Metadata, error) {
	base, err := oauth2SiteBase(serverURL)
	if err != nil {
		return OAuth2Metadata{}, err
	}
	fallback := OAuth2Metadata{
		Issuer:                      base,
		AuthorizationEndpoint:       base + "/oauth/authorize",
		TokenEndpoint:               base + "/api/v1/oauth2/token",
		DeviceAuthorizationEndpoint: base + "/api/v1/oauth2/device_authorization",
		UserinfoEndpoint:            base + "/api/v1/oauth2/userinfo",
		YggdrasilAPILocation:        base + "/api/yggdrasil/",
	}
	meta := fallback
	var discovered OAuth2Metadata
	if err := httpx.GetJSON(ctx, base+"/.well-known/oauth-authorization-server", &discovered); err == nil {
		meta.Issuer = firstNonEmpty(discovered.Issuer, fallback.Issuer)
		meta.AuthorizationEndpoint = firstNonEmpty(discovered.AuthorizationEndpoint, fallback.AuthorizationEndpoint)
		meta.TokenEndpoint = firstNonEmpty(discovered.TokenEndpoint, fallback.TokenEndpoint)
		meta.DeviceAuthorizationEndpoint = firstNonEmpty(discovered.DeviceAuthorizationEndpoint, fallback.DeviceAuthorizationEndpoint)
		meta.UserinfoEndpoint = firstNonEmpty(discovered.UserinfoEndpoint, fallback.UserinfoEndpoint)
		meta.YggdrasilAPILocation = firstNonEmpty(discovered.YggdrasilAPILocation, fallback.YggdrasilAPILocation)
	}
	meta.AuthorizationEndpoint = oauth2Resolve(base, meta.AuthorizationEndpoint)
	meta.TokenEndpoint = oauth2Resolve(base, meta.TokenEndpoint)
	meta.DeviceAuthorizationEndpoint = oauth2Resolve(base, meta.DeviceAuthorizationEndpoint)
	meta.UserinfoEndpoint = oauth2Resolve(base, meta.UserinfoEndpoint)
	meta.YggdrasilAPILocation = oauth2Resolve(base, meta.YggdrasilAPILocation)
	if meta.TokenEndpoint == "" || meta.DeviceAuthorizationEndpoint == "" {
		return OAuth2Metadata{}, errors.New("YSS 授权服务器缺少令牌端点")
	}
	return meta, nil
}

// BeginYSSDevice starts the device-code flow.
func BeginYSSDevice(ctx context.Context, serverURL, clientID string) (OAuth2Metadata, DeviceAuthorization, error) {
	clientID = firstNonEmpty(clientID, DefaultYSSClientID)
	meta, err := DiscoverYSSOAuth2(ctx, serverURL)
	if err != nil {
		return OAuth2Metadata{}, DeviceAuthorization{}, err
	}
	form := url.Values{
		"client_id": {clientID},
		"scope":     {yssScope},
	}
	body, status, err := postForm(ctx, meta.DeviceAuthorizationEndpoint, form)
	if err != nil {
		return OAuth2Metadata{}, DeviceAuthorization{}, err
	}
	if status < 200 || status >= 300 {
		if status == http.StatusNotFound {
			return OAuth2Metadata{}, DeviceAuthorization{}, errors.New("YSS 未启用启动器 OAuth2，或一键授权端点不可用")
		}
		return OAuth2Metadata{}, DeviceAuthorization{}, fmt.Errorf("YSS 设备码请求失败: %s", oauthError(body, status))
	}
	var device DeviceAuthorization
	if err := json.Unmarshal(body, &device); err != nil {
		return OAuth2Metadata{}, DeviceAuthorization{}, fmt.Errorf("解析 YSS 设备码失败: %w", err)
	}
	if device.DeviceCode == "" || device.UserCode == "" {
		return OAuth2Metadata{}, DeviceAuthorization{}, errors.New("YSS 设备码响应缺少必要字段")
	}
	base, _ := oauth2SiteBase(serverURL)
	device.VerificationURI = oauth2Resolve(base, device.VerificationURI)
	device.VerificationURIComplete = oauth2Resolve(base, device.VerificationURIComplete)
	if !oauth2URLAllowed(meta, device.VerificationURI) {
		return OAuth2Metadata{}, DeviceAuthorization{}, errors.New("YSS 授权页面地址与站点不一致")
	}
	if device.VerificationURIComplete != "" && !oauth2URLAllowed(meta, device.VerificationURIComplete) {
		// Never auto-open an unexpected host; the plain verification URI remains.
		device.VerificationURIComplete = ""
	}
	if device.ExpiresIn <= 0 {
		device.ExpiresIn = 600
	}
	if device.Interval <= 0 {
		device.Interval = 5
	}
	return meta, device, nil
}

func oauth2URLAllowed(meta OAuth2Metadata, raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	target, err := url.Parse(raw)
	if err != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") {
		return false
	}
	for _, endpoint := range []string{meta.Issuer, meta.TokenEndpoint, meta.YggdrasilAPILocation} {
		allowed, err := url.Parse(endpoint)
		if err == nil && strings.EqualFold(allowed.Host, target.Host) {
			return true
		}
	}
	return false
}

// PollYSSDevice waits for the user to approve the device code and converts the
// resulting Yggdrasil tokens into an Authlib account.
func PollYSSDevice(ctx context.Context, meta OAuth2Metadata, device DeviceAuthorization, clientID string) (Account, error) {
	clientID = firstNonEmpty(clientID, DefaultYSSClientID)
	interval := device.Interval
	if interval <= 0 {
		interval = 5
	}
	expiresIn := device.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 600
	}
	deadline := time.Now().Add(time.Duration(expiresIn) * time.Second)
	for {
		timer := time.NewTimer(time.Duration(interval) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return Account{}, ctx.Err()
		case <-timer.C:
		}
		if time.Now().After(deadline) {
			return Account{}, errors.New("YSS 设备码已过期，请重新发起授权")
		}
		form := url.Values{
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
			"device_code": {device.DeviceCode},
			"client_id":   {clientID},
		}
		body, status, err := postForm(ctx, meta.TokenEndpoint, form)
		if err != nil {
			return Account{}, err
		}
		if status >= 200 && status < 300 {
			var tokens OAuth2Tokens
			if err := json.Unmarshal(body, &tokens); err != nil {
				return Account{}, fmt.Errorf("解析 YSS 令牌响应失败: %w", err)
			}
			return yssAccountFromTokens(meta, tokens, clientID)
		}
		var payload oauth2ErrorPayload
		_ = json.Unmarshal(body, &payload)
		switch payload.Error {
		case "authorization_pending":
			continue
		case "slow_down":
			interval += 5
			continue
		case "access_denied":
			return Account{}, errors.New("YSS 授权已被拒绝")
		case "expired_token":
			return Account{}, errors.New("YSS 设备码已过期，请重新发起授权")
		default:
			return Account{}, fmt.Errorf("YSS 授权失败: %s", oauthError(body, status))
		}
	}
}

func yssAccountFromTokens(meta OAuth2Metadata, tokens OAuth2Tokens, clientID string) (Account, error) {
	if tokens.Yggdrasil == nil || strings.TrimSpace(tokens.Yggdrasil.AccessToken) == "" {
		return Account{}, errors.New("YSS 未返回 Yggdrasil 令牌，请确认站点已开启 yggdrasil scope")
	}
	profile := tokens.Yggdrasil.SelectedProfile
	if profile == nil && len(tokens.Yggdrasil.AvailableProfiles) > 0 {
		profile = &tokens.Yggdrasil.AvailableProfiles[0]
	}
	if profile == nil || strings.TrimSpace(profile.ID) == "" || strings.TrimSpace(profile.Name) == "" {
		return Account{}, errors.New("该 YSS 账号没有可用的 Minecraft 档案")
	}
	serverURL := meta.YggdrasilAPILocation
	normalized, err := NormalizeAuthlibURL(serverURL)
	if err != nil {
		return Account{}, fmt.Errorf("YSS Yggdrasil 地址无效: %w", err)
	}
	var expiresAt time.Time
	if tokens.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second)
	}
	return Account{
		Kind:                KindAuthlib,
		Name:                strings.TrimSpace(profile.Name),
		UUID:                strings.ReplaceAll(strings.TrimSpace(profile.ID), "-", ""),
		AccessToken:         tokens.Yggdrasil.AccessToken,
		ClientToken:         tokens.Yggdrasil.ClientToken,
		RefreshToken:        tokens.RefreshToken,
		ServerURL:           normalized,
		ExpiresAt:           expiresAt,
		AddedAt:             time.Now(),
		OAuth2:              true,
		OAuth2TokenEndpoint: meta.TokenEndpoint,
		OAuth2ClientID:      clientID,
	}, nil
}

// RefreshYSSAccount uses the stored OAuth2 refresh token to obtain a new
// Yggdrasil session. YSS rotates refresh tokens on every use.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func RefreshYSSAccount(ctx context.Context, account Account) (Account, error) {
	if !account.OAuth2 || strings.TrimSpace(account.OAuth2TokenEndpoint) == "" || strings.TrimSpace(account.RefreshToken) == "" {
		return Account{}, errors.New("该账户不支持 YSS 一键授权刷新")
	}
	clientID := firstNonEmpty(account.OAuth2ClientID, DefaultYSSClientID)
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {account.RefreshToken},
		"client_id":     {clientID},
	}
	body, status, err := postForm(ctx, account.OAuth2TokenEndpoint, form)
	if err != nil {
		return Account{}, err
	}
	if status < 200 || status >= 300 {
		return Account{}, fmt.Errorf("YSS 授权已失效，请重新授权: %s", oauthError(body, status))
	}
	var tokens OAuth2Tokens
	if err := json.Unmarshal(body, &tokens); err != nil {
		return Account{}, fmt.Errorf("解析 YSS 刷新响应失败: %w", err)
	}
	if tokens.Yggdrasil == nil || strings.TrimSpace(tokens.Yggdrasil.AccessToken) == "" {
		return Account{}, errors.New("YSS 刷新响应未包含 Yggdrasil 令牌，请重新授权")
	}
	profile := tokens.Yggdrasil.SelectedProfile
	if profile == nil && len(tokens.Yggdrasil.AvailableProfiles) > 0 {
		profile = &tokens.Yggdrasil.AvailableProfiles[0]
	}
	account.AccessToken = tokens.Yggdrasil.AccessToken
	if tokens.Yggdrasil.ClientToken != "" {
		account.ClientToken = tokens.Yggdrasil.ClientToken
	}
	if tokens.RefreshToken != "" {
		account.RefreshToken = tokens.RefreshToken
	}
	if tokens.ExpiresIn > 0 {
		account.ExpiresAt = time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second)
	}
	if profile != nil && profile.ID != "" && profile.Name != "" {
		account.UUID = strings.ReplaceAll(strings.TrimSpace(profile.ID), "-", "")
		account.Name = strings.TrimSpace(profile.Name)
	}
	return account, nil
}
