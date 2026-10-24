package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"MyMinecraftGoLuncher/internal/httpx"
	"MyMinecraftGoLuncher/internal/store"
	"MyMinecraftGoLuncher/internal/util"
)

const injectorMetadataURL = "https://authlib-injector.yushi.moe/artifact/latest.json"

type authlibProfile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type authlibResponse struct {
	AccessToken       string           `json:"accessToken"`
	ClientToken       string           `json:"clientToken"`
	SelectedProfile   *authlibProfile  `json:"selectedProfile"`
	AvailableProfiles []authlibProfile `json:"availableProfiles"`
}

// NormalizeAuthlibURL accepts an API root or a common authserver URL.
func NormalizeAuthlibURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("请填写外置登录服务器地址")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("外置登录地址格式无效")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("外置登录地址只支持 http/https")
	}
	if parsed.Host == "" {
		return "", errors.New("外置登录地址缺少主机名")
	}
	path := strings.TrimRight(parsed.Path, "/")
	for _, suffix := range []string{"/authserver/authenticate", "/authserver/refresh", "/authserver/validate", "/authserver"} {
		if strings.HasSuffix(path, suffix) {
			path = strings.TrimSuffix(path, suffix)
			break
		}
	}
	parsed.Path = path
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

// LoginAuthlib authenticates against an Authlib-Injector compatible server.
func LoginAuthlib(ctx context.Context, serverURL, username, password string) (Account, error) {
	serverURL, err := NormalizeAuthlibURL(serverURL)
	if err != nil {
		return Account{}, err
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return Account{}, errors.New("请输入第三方账户用户名")
	}
	if password == "" {
		return Account{}, errors.New("请输入第三方账户密码")
	}
	clientToken := RandomClientToken()
	payload := map[string]any{
		"agent":       map[string]any{"name": "Minecraft", "version": 1},
		"username":    username,
		"password":    password,
		"clientToken": clientToken,
		"requestUser": false,
	}
	body, status, err := postJSON(ctx, serverURL+"/authserver/authenticate", payload)
	if err != nil {
		return Account{}, err
	}
	if status < 200 || status >= 300 {
		return Account{}, fmt.Errorf("外置登录失败: %s", oauthError(body, status))
	}
	var response authlibResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return Account{}, fmt.Errorf("解析外置登录响应失败: %w", err)
	}
	profile := response.SelectedProfile
	if profile == nil && len(response.AvailableProfiles) > 0 {
		profile = &response.AvailableProfiles[0]
	}
	if response.AccessToken == "" || profile == nil || profile.ID == "" {
		return Account{}, errors.New("外置登录服务器未返回已选择的角色")
	}
	if response.ClientToken != "" {
		clientToken = response.ClientToken
	}
	return Account{
		Kind: KindAuthlib, Name: profile.Name, UUID: strings.ReplaceAll(profile.ID, "-", ""),
		AccessToken: response.AccessToken, ClientToken: clientToken,
		ServerURL: serverURL, AddedAt: time.Now(),
	}, nil
}

// RefreshAuthlib rotates an expired access token.
func RefreshAuthlib(ctx context.Context, account Account) (Account, error) {
	if account.Kind != KindAuthlib || account.ServerURL == "" {
		return Account{}, errors.New("不是有效的外置登录账户")
	}
	payload := map[string]any{
		"accessToken": account.AccessToken,
		"clientToken": account.ClientToken,
		"requestUser": false,
	}
	body, status, err := postJSON(ctx, account.ServerURL+"/authserver/refresh", payload)
	if err != nil {
		return Account{}, err
	}
	if status < 200 || status >= 300 {
		return Account{}, fmt.Errorf("外置登录刷新失败，请重新登录: %s", oauthError(body, status))
	}
	var response authlibResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return Account{}, err
	}
	if response.AccessToken == "" {
		return Account{}, errors.New("外置登录服务器未返回新的令牌")
	}
	account.AccessToken = response.AccessToken
	if response.ClientToken != "" {
		account.ClientToken = response.ClientToken
	}
	if response.SelectedProfile != nil {
		account.Name = response.SelectedProfile.Name
		account.UUID = strings.ReplaceAll(response.SelectedProfile.ID, "-", "")
	}
	return account, nil
}

// ValidateAuthlib checks whether a stored token is still accepted.
func ValidateAuthlib(ctx context.Context, account Account) (bool, error) {
	if account.ServerURL == "" || account.AccessToken == "" {
		return false, nil
	}
	payload := map[string]any{"accessToken": account.AccessToken, "clientToken": account.ClientToken}
	body, status, err := postJSON(ctx, account.ServerURL+"/authserver/validate", payload)
	if err != nil {
		return false, err
	}
	switch {
	case status >= 200 && status < 300:
		return true, nil
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return false, nil
	default:
		return false, fmt.Errorf("外置登录验证失败: %s", oauthError(body, status))
	}
}

type injectorMetadata struct {
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
	Checksums   struct {
		SHA256 string `json:"sha256"`
	} `json:"checksums"`
}

// FindInjector returns a previously downloaded authlib-injector jar.
func FindInjector(paths store.Paths) string {
	matches, _ := filepath.Glob(filepath.Join(paths.Cache, "authlib-injector", "authlib-injector-*.jar"))
	if len(matches) == 0 {
		return ""
	}
	// Prefer the most recently modified artifact.
	best := matches[0]
	var bestTime time.Time
	for _, match := range matches {
		if info, err := os.Stat(match); err == nil && info.ModTime().After(bestTime) {
			best, bestTime = match, info.ModTime()
		}
	}
	return best
}

// EnsureInjector downloads the latest authlib-injector agent and returns its path.
func EnsureInjector(ctx context.Context, paths store.Paths) (string, error) {
	dir := filepath.Join(paths.Cache, "authlib-injector")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	metadataPath := filepath.Join(dir, "latest.json")
	var metadata injectorMetadata
	if stat, err := os.Stat(metadataPath); err == nil && time.Since(stat.ModTime()) < 24*time.Hour {
		_ = util.ReadJSON(metadataPath, &metadata)
	}
	if metadata.DownloadURL == "" {
		fetched, err := fetchInjectorMetadata(ctx)
		if err != nil {
			// A previously downloaded version is better than failing when the
			// metadata endpoint is temporarily unavailable.
			if util.ReadJSON(metadataPath, &metadata) != nil || metadata.DownloadURL == "" {
				return "", err
			}
		} else {
			metadata = fetched
			_ = util.WriteJSON(metadataPath, metadata)
		}
	}
	jarPath := filepath.Join(dir, "authlib-injector-"+metadata.Version+".jar")
	if util.Exists(jarPath) {
		return jarPath, nil
	}
	if metadata.DownloadURL == "" {
		return "", errors.New("authlib-injector 下载地址缺失")
	}
	if err := httpx.Download(ctx, httpx.File{
		URL: metadata.DownloadURL, Dest: jarPath, SHA256: metadata.Checksums.SHA256,
	}); err != nil {
		return "", fmt.Errorf("下载 authlib-injector 失败: %w", err)
	}
	return jarPath, nil
}

func fetchInjectorMetadata(ctx context.Context) (injectorMetadata, error) {
	var metadata injectorMetadata
	if err := httpx.GetJSON(ctx, injectorMetadataURL, &metadata); err != nil {
		return injectorMetadata{}, fmt.Errorf("获取 authlib-injector 版本失败: %w", err)
	}
	if metadata.DownloadURL == "" || metadata.Version == "" {
		return injectorMetadata{}, errors.New("authlib-injector 版本信息无效")
	}
	return metadata, nil
}
