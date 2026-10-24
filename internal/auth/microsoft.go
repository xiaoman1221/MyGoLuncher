package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"MyMinecraftGoLuncher/internal/httpx"
)

// DefaultMicrosoftClientID is the public Microsoft OAuth client ID associated
// with Prism Launcher. It can be overridden in settings. Microsoft currently
// allowlists Minecraft Services app IDs, so a public ID may stop working or be
// rate limited; an approved private client ID is the durable option.
const DefaultMicrosoftClientID = "c36a9fb6-4f2a-41ff-90bd-ae7cc92031eb"

const microsoftTenant = "https://login.microsoftonline.com/consumers/oauth2/v2.0"

// ErrLoginCancelled distinguishes a user/app cancellation from a timeout.
var ErrLoginCancelled = errors.New("Microsoft 登录已取消")

// DeviceCode is the public part of an OAuth device authorization request.
type DeviceCode struct {
	DeviceCode      string `json:"-"`
	UserCode        string `json:"userCode"`
	VerificationURI string `json:"verificationUri"`
	ExpiresIn       int    `json:"expiresIn"`
	Interval        int    `json:"interval"`
	Message         string `json:"message"`
}

// MSAToken is a Microsoft OAuth token response.
type MSAToken struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
}

// Profile is the authenticated Minecraft Java profile.
type Profile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// StartDeviceCode begins Microsoft device authorization.
func StartDeviceCode(ctx context.Context, clientID string) (DeviceCode, error) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return DeviceCode{}, errors.New("Microsoft Client ID 为空")
	}
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("scope", "XboxLive.signin offline_access")
	body, status, err := postForm(ctx, microsoftTenant+"/devicecode", form)
	if err != nil {
		return DeviceCode{}, err
	}
	if status != http.StatusOK {
		return DeviceCode{}, fmt.Errorf("Microsoft 设备码请求失败: %s", oauthError(body, status))
	}
	var payload struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		VerificationURL string `json:"verification_url"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
		Message         string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return DeviceCode{}, fmt.Errorf("解析 Microsoft 设备码失败: %w", err)
	}
	if payload.DeviceCode == "" || payload.UserCode == "" {
		return DeviceCode{}, errors.New("Microsoft 未返回有效的设备码")
	}
	uri := payload.VerificationURI
	if uri == "" {
		uri = payload.VerificationURL
	}
	info := DeviceCode{
		DeviceCode: payload.DeviceCode, UserCode: payload.UserCode,
		VerificationURI: uri, ExpiresIn: payload.ExpiresIn,
		Interval: payload.Interval, Message: payload.Message,
	}
	if info.Interval <= 0 {
		info.Interval = 5
	}
	if info.ExpiresIn <= 0 {
		info.ExpiresIn = 900
	}
	return info, nil
}

// PollDeviceCode waits for the user to finish signing in.
func PollDeviceCode(ctx context.Context, clientID, deviceCode string, interval int, expiresIn int) (MSAToken, error) {
	if interval <= 0 {
		interval = 5
	}
	if expiresIn <= 0 {
		expiresIn = 900
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(expiresIn)*time.Second)
	defer cancel()
	wait := time.Duration(interval) * time.Second
	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				return MSAToken{}, ErrLoginCancelled
			}
			return MSAToken{}, errors.New("Microsoft 登录超时，请重新发起登录")
		case <-time.After(wait):
		}
		form := url.Values{}
		form.Set("client_id", clientID)
		form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
		form.Set("device_code", deviceCode)
		body, status, err := postForm(ctx, microsoftTenant+"/token", form)
		if err != nil {
			if ctx.Err() != nil {
				return MSAToken{}, errors.New("Microsoft 登录超时，请重新发起登录")
			}
			return MSAToken{}, err
		}
		if status == http.StatusOK {
			var token struct {
				AccessToken  string `json:"access_token"`
				RefreshToken string `json:"refresh_token"`
				ExpiresIn    int    `json:"expires_in"`
			}
			if err := json.Unmarshal(body, &token); err != nil {
				return MSAToken{}, fmt.Errorf("解析 Microsoft 令牌失败: %w", err)
			}
			if token.AccessToken == "" {
				return MSAToken{}, errors.New("Microsoft 未返回访问令牌")
			}
			return MSAToken{
				AccessToken: token.AccessToken, RefreshToken: token.RefreshToken,
				ExpiresIn: token.ExpiresIn,
			}, nil
		}
		var payload struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		_ = json.Unmarshal(body, &payload)
		switch payload.Error {
		case "authorization_pending":
			continue
		case "slow_down":
			wait += 5 * time.Second
			continue
		case "expired_token":
			return MSAToken{}, errors.New("Microsoft 设备码已过期，请重新发起登录")
		case "authorization_declined":
			return MSAToken{}, errors.New("你已取消 Microsoft 登录")
		default:
			message := payload.ErrorDescription
			if message == "" {
				message = strings.TrimSpace(string(body))
			}
			return MSAToken{}, fmt.Errorf("Microsoft 登录失败: %s", message)
		}
	}
}

// RefreshMSA exchanges a refresh token for a new Microsoft access token.
func RefreshMSA(ctx context.Context, clientID, refreshToken string) (MSAToken, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return MSAToken{}, errors.New("Microsoft 登录已失效，请重新登录")
	}
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("scope", "XboxLive.signin offline_access")
	body, status, err := postForm(ctx, microsoftTenant+"/token", form)
	if err != nil {
		return MSAToken{}, err
	}
	if status != http.StatusOK {
		return MSAToken{}, fmt.Errorf("Microsoft 登录刷新失败: %s", oauthError(body, status))
	}
	var token struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &token); err != nil {
		return MSAToken{}, err
	}
	if token.AccessToken == "" {
		return MSAToken{}, errors.New("Microsoft 未返回新的访问令牌")
	}
	if token.RefreshToken == "" {
		token.RefreshToken = refreshToken
	}
	return MSAToken{AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, ExpiresIn: token.ExpiresIn}, nil
}

type xboxResponse struct {
	Token         string `json:"Token"`
	DisplayClaims struct {
		XUI []struct {
			UHS string `json:"uhs"`
			XID string `json:"xid"`
		} `json:"xui"`
	} `json:"DisplayClaims"`
}

// AuthenticateMinecraft runs the Xbox Live, XSTS and Minecraft Services chain.
func AuthenticateMinecraft(ctx context.Context, clientID, msaToken, refreshToken string) (Account, error) {
	xbl, err := xboxAuthenticate(ctx, msaToken)
	if err != nil {
		return Account{}, err
	}
	userHash := ""
	xuid := ""
	if len(xbl.DisplayClaims.XUI) > 0 {
		userHash = xbl.DisplayClaims.XUI[0].UHS
		xuid = xbl.DisplayClaims.XUI[0].XID
	}
	if userHash == "" {
		return Account{}, errors.New("Xbox 认证未返回用户标识")
	}
	xsts, err := xstsAuthorize(ctx, xbl.Token)
	if err != nil {
		return Account{}, err
	}
	if len(xsts.DisplayClaims.XUI) > 0 && xsts.DisplayClaims.XUI[0].XID != "" {
		xuid = xsts.DisplayClaims.XUI[0].XID
	}
	identityToken := "XBL3.0 x=" + userHash + ";" + xsts.Token
	mcToken, expiresIn, err := loginWithXbox(ctx, identityToken)
	if err != nil {
		return Account{}, err
	}
	profile, err := fetchProfile(ctx, mcToken)
	if err != nil {
		return Account{}, err
	}
	expiresAt := time.Now().Add(time.Duration(expiresIn) * time.Second)
	return Account{
		Kind:         KindMicrosoft,
		Name:         profile.Name,
		UUID:         strings.ReplaceAll(profile.ID, "-", ""),
		AccessToken:  mcToken,
		RefreshToken: refreshToken,
		XUID:         xuid,
		ExpiresAt:    expiresAt,
		AddedAt:      time.Now(),
	}, nil
}

func xboxAuthenticate(ctx context.Context, msaToken string) (xboxResponse, error) {
	payload := map[string]any{
		"Properties": map[string]string{
			"AuthMethod": "RPS",
			"SiteName":   "user.auth.xboxlive.com",
			"RpsTicket":  "d=" + msaToken,
		},
		"RelyingParty": "http://auth.xboxlive.com",
		"TokenType":    "JWT",
	}
	body, status, err := postJSON(ctx, "https://user.auth.xboxlive.com/user/authenticate", payload)
	if err != nil {
		return xboxResponse{}, err
	}
	if status != http.StatusOK {
		return xboxResponse{}, fmt.Errorf("Xbox Live 认证失败: %s", oauthError(body, status))
	}
	var response xboxResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return xboxResponse{}, fmt.Errorf("解析 Xbox Live 响应失败: %w", err)
	}
	if response.Token == "" {
		return xboxResponse{}, errors.New("Xbox Live 未返回令牌")
	}
	return response, nil
}

func xstsAuthorize(ctx context.Context, xblToken string) (xboxResponse, error) {
	payload := map[string]any{
		"Properties": map[string]any{
			"SandboxId":  "RETAIL",
			"UserTokens": []string{xblToken},
		},
		"RelyingParty": "rp://api.minecraftservices.com/",
		"TokenType":    "JWT",
	}
	body, status, err := postJSON(ctx, "https://xsts.auth.xboxlive.com/xsts/authorize", payload)
	if err != nil {
		return xboxResponse{}, err
	}
	if status != http.StatusOK {
		var failure struct {
			XErr     int    `json:"XErr"`
			Message  string `json:"Message"`
			Redirect string `json:"Redirect"`
		}
		_ = json.Unmarshal(body, &failure)
		switch failure.XErr {
		case 2148916233:
			return xboxResponse{}, errors.New("该 Microsoft 账户没有 Xbox 档案，请先在 Xbox 官网创建")
		case 2148916235:
			return xboxResponse{}, errors.New("Xbox Live 在当前国家/地区不可用")
		case 2148916238:
			return xboxResponse{}, errors.New("该账户是儿童账户，需要加入 Microsoft 家庭组后才能登录")
		default:
			message := strings.TrimSpace(failure.Message)
			if message == "" {
				message = strings.TrimSpace(string(body))
			}
			return xboxResponse{}, fmt.Errorf("XSTS 认证失败: %s", message)
		}
	}
	var response xboxResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return xboxResponse{}, fmt.Errorf("解析 XSTS 响应失败: %w", err)
	}
	if response.Token == "" {
		return xboxResponse{}, errors.New("XSTS 未返回令牌")
	}
	return response, nil
}

func loginWithXbox(ctx context.Context, identityToken string) (string, int, error) {
	body, status, err := postJSON(ctx, "https://api.minecraftservices.com/authentication/login_with_xbox", map[string]string{
		"identityToken": identityToken,
	})
	if err != nil {
		return "", 0, err
	}
	if status == http.StatusForbidden && strings.Contains(strings.ToLower(string(body)), "invalid app registration") {
		return "", 0, errors.New("Minecraft 拒绝了当前 OAuth Client ID（Invalid app registration）。请到设置中填写已获 Minecraft Services 许可的 Client ID，或稍后重试")
	}
	if status != http.StatusOK {
		return "", 0, fmt.Errorf("Minecraft 登录失败: %s", oauthError(body, status))
	}
	var payload struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", 0, fmt.Errorf("解析 Minecraft 登录响应失败: %w", err)
	}
	if payload.AccessToken == "" {
		return "", 0, errors.New("Minecraft 未返回访问令牌")
	}
	if payload.ExpiresIn <= 0 {
		payload.ExpiresIn = 86400
	}
	return payload.AccessToken, payload.ExpiresIn, nil
}

func fetchProfile(ctx context.Context, accessToken string) (Profile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.minecraftservices.com/minecraft/profile", nil)
	if err != nil {
		return Profile{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", httpx.UserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := httpx.Client.Do(req)
	if err != nil {
		return Profile{}, fmt.Errorf("获取 Minecraft 档案失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode == http.StatusNotFound {
		return Profile{}, errors.New("该 Microsoft 账户未拥有 Minecraft: Java Edition")
	}
	if resp.StatusCode != http.StatusOK {
		return Profile{}, fmt.Errorf("获取 Minecraft 档案失败: %s", oauthError(body, resp.StatusCode))
	}
	var profile Profile
	if err := json.Unmarshal(body, &profile); err != nil {
		return Profile{}, fmt.Errorf("解析 Minecraft 档案失败: %w", err)
	}
	if profile.ID == "" || profile.Name == "" {
		return Profile{}, errors.New("该账户没有可用的 Minecraft Java 档案")
	}
	return profile, nil
}

func postForm(ctx context.Context, endpoint string, values url.Values) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", httpx.UserAgent)
	resp, err := httpx.Client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("网络请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return body, resp.StatusCode, nil
}

func postJSON(ctx context.Context, endpoint string, payload any) ([]byte, int, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", httpx.UserAgent)
	resp, err := httpx.Client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("网络请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return body, resp.StatusCode, nil
}

func oauthError(body []byte, status int) string {
	var payload struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
		ErrorMessage     string `json:"errorMessage"`
		Message          string `json:"Message"`
		ErrorCode        string `json:"errorCode"`
	}
	if json.Unmarshal(body, &payload) == nil {
		for _, value := range []string{payload.ErrorDescription, payload.ErrorMessage, payload.Message, payload.Error, payload.ErrorCode} {
			if strings.TrimSpace(value) != "" {
				return value
			}
		}
	}
	text := strings.TrimSpace(string(body))
	if len(text) > 300 {
		text = text[:300]
	}
	if text == "" {
		text = "HTTP " + strconv.Itoa(status)
	}
	return text
}
