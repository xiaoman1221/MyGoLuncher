package license

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const activationPrefix = "/api/v1/activation"

// AppVersion is the client version reported to the activation service.
var AppVersion = "1.0.5"

// Client talks to one LicensingGo product activation API.
type Client struct {
	BaseURL string
	APIKey  string
	Lang    string
	HTTP    *http.Client
}

// NewClient creates a client with a bounded network timeout.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		APIKey:  strings.TrimSpace(apiKey),
		Lang:    "zh-CN",
		HTTP:    &http.Client{Timeout: 15 * time.Second},
	}
}

// APIError is a failed HTTP response from LicensingGo.
type APIError struct {
	StatusCode int
	Code       string `json:"code"`
	Message    string `json:"message"`
	RetryAfter int    `json:"retry_after"`
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Code != "" {
		return e.Code
	}
	return fmt.Sprintf("授权服务返回 HTTP %d", e.StatusCode)
}

// UserMessage maps stable API error codes to actionable client messages.
// LicensingGo asks clients to branch on code, never on localized message text.
func (e *APIError) UserMessage() string {
	if e == nil {
		return "授权请求失败"
	}
	switch e.Code {
	case "client_outdated":
		return "当前启动器版本过旧，无法使用新版授权服务器，请更新启动器后重新激活"
	case "identity_mismatch":
		return "邮箱或客户名称与激活码不匹配，请核对购买时登记的信息"
	case "max_machines_reached":
		return "激活设备数已达上限，请先解绑旧设备或联系管理员"
	case "machine_already_bound":
		return "本机已绑定到该产品下的另一个激活码，请先用原激活码解绑"
	case "code_not_found":
		return "激活码不存在，或不属于当前产品"
	case "code_disabled":
		return "激活码已被停用，请联系客服"
	case "code_expired":
		return "激活码已过期，请续费"
	case "not_activated":
		return "本机当前没有有效激活记录"
	case "invalid_unbind_key":
		return "本地解绑密钥无效，请使用邮箱验证解绑或联系管理员"
	case "invalid_api_key", "product_disabled":
		return "当前产品的授权服务配置无效，请联系管理员"
	case "mutual_tls_required":
		return "授权服务器要求客户端证书，请联系管理员获取"
	case "email_not_configured":
		return "授权服务器未配置邮件服务，请联系管理员手动解绑"
	case "email_send_failed":
		return "验证码邮件发送失败，请稍后重试或联系管理员"
	case "verification_expired":
		return "验证码已过期，请重新申请"
	case "invalid_verification":
		return "验证码错误或尝试次数过多，请重新申请"
	case "too_many_requests":
		if e.RetryAfter > 0 {
			return fmt.Sprintf("请求过于频繁，请 %d 秒后重试", e.RetryAfter)
		}
		return "请求过于频繁，请稍后重试"
	case "server_error":
		return "授权服务器内部错误，请稍后重试"
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Code != "" {
		return e.Code
	}
	return fmt.Sprintf("授权服务返回 HTTP %d", e.StatusCode)
}

// DescribeError converts stable API failures into user-facing errors.
func DescribeError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return errors.New(apiErr.UserMessage())
	}
	return err
}

// ActivateRequest is sent to /activate.
type ActivateRequest struct {
	Code        string         `json:"code"`
	MachineCode string         `json:"machine_code"`
	MachineName string         `json:"machine_name,omitempty"`
	OS          string         `json:"os,omitempty"`
	Email       string         `json:"email"`
	Customer    string         `json:"customer"`
	IP          string         `json:"ip,omitempty"`
	Extra       map[string]any `json:"extra,omitempty"`
}

// ActivateResult is returned after a successful activation.
type ActivateResult struct {
	Status      string     `json:"status"`
	Message     string     `json:"message"`
	ExpiresAt   *time.Time `json:"expires_at"`
	Features    []string   `json:"features"`
	DeviceID    string     `json:"device_id"`
	MachineCode string     `json:"machine_code"`
	UnbindKey   string     `json:"unbind_key,omitempty"`
}

// ValidateResult is returned by /validate and /unbind. Validate uses HTTP 200
// for both valid and invalid codes and distinguishes them with Valid.
type ValidateResult struct {
	Valid       bool       `json:"valid"`
	Status      string     `json:"status"`
	Reason      string     `json:"reason"`
	Message     string     `json:"message"`
	ExpiresAt   *time.Time `json:"expires_at"`
	Features    []string   `json:"features"`
	MachineCode string     `json:"machine_code"`
	LastSeenAt  *time.Time `json:"last_seen_at"`
}

// EmailUnbindResult is returned after requesting an email verification code.
type EmailUnbindResult struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (c *Client) post(ctx context.Context, path string, body any, out any) error {
	if c == nil || c.HTTP == nil {
		return errors.New("授权客户端未初始化")
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.APIKey)
	req.Header.Set("Accept-Language", c.Lang)
	req.Header.Set("User-Agent", "MyGoLuncher/"+AppVersion)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("无法连接授权服务: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := &APIError{StatusCode: resp.StatusCode}
		_ = json.Unmarshal(data, apiErr)
		// Rate-limit responses use {"error":"too many requests",...} rather
		// than the normal {"code":"...","message":"..."} envelope.
		var rateLimit struct {
			Error      string `json:"error"`
			RetryAfter int    `json:"retry_after"`
		}
		_ = json.Unmarshal(data, &rateLimit)
		if apiErr.Code == "" && strings.EqualFold(strings.TrimSpace(rateLimit.Error), "too many requests") {
			apiErr.Code = "too_many_requests"
		}
		if apiErr.RetryAfter == 0 {
			apiErr.RetryAfter = rateLimit.RetryAfter
		}
		if retry := resp.Header.Get("Retry-After"); retry != "" {
			if seconds, parseErr := strconv.Atoi(retry); parseErr == nil {
				apiErr.RetryAfter = seconds
			}
		}
		if apiErr.Code == "" {
			apiErr.Code = "http_error"
			if apiErr.Message == "" {
				apiErr.Message = strings.TrimSpace(string(data))
			}
		}
		return apiErr
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("解析授权服务响应失败: %w", err)
	}
	return nil
}

// Activate binds this machine to an activation code. unbind_key is returned
// only on the first bind (or a rebind) and must be persisted.
func (c *Client) Activate(ctx context.Context, req ActivateRequest) (ActivateResult, error) {
	var result ActivateResult
	req.Code = strings.TrimSpace(req.Code)
	req.MachineCode = strings.TrimSpace(req.MachineCode)
	req.Email = strings.TrimSpace(req.Email)
	req.Customer = strings.TrimSpace(req.Customer)
	if req.Code == "" || req.MachineCode == "" {
		return result, errors.New("激活码和机器码不能为空")
	}
	if req.Email == "" || !strings.Contains(req.Email, "@") {
		return result, errors.New("请填写激活码绑定的邮箱")
	}
	if req.Customer == "" {
		return result, errors.New("请填写激活码绑定的客户名称")
	}
	err := c.post(ctx, activationPrefix+"/activate", req, &result)
	return result, err
}

// Validate checks the binding. A non-nil error means the request failed; an
// invalid license is returned with Valid=false and Reason set.
func (c *Client) Validate(ctx context.Context, code, machineCode string) (ValidateResult, error) {
	var result ValidateResult
	err := c.post(ctx, activationPrefix+"/validate", map[string]string{
		"code": strings.TrimSpace(code), "machine_code": strings.TrimSpace(machineCode),
	}, &result)
	return result, err
}

// Unbind releases this machine using the one-time unbind key.
func (c *Client) Unbind(ctx context.Context, code, machineCode, unbindKey string) (ValidateResult, error) {
	var result ValidateResult
	err := c.post(ctx, activationPrefix+"/unbind", map[string]string{
		"code": strings.TrimSpace(code), "machine_code": strings.TrimSpace(machineCode), "unbind_key": unbindKey,
	}, &result)
	return result, err
}

// RequestEmailUnbind sends a six-digit verification code to the email bound
// to the activation code.
func (c *Client) RequestEmailUnbind(ctx context.Context, code, email, machineCode string) (EmailUnbindResult, error) {
	var result EmailUnbindResult
	email = strings.TrimSpace(email)
	if strings.TrimSpace(code) == "" || strings.TrimSpace(machineCode) == "" {
		return result, errors.New("激活码和机器码不能为空")
	}
	if email == "" || !strings.Contains(email, "@") {
		return result, errors.New("请填写激活码绑定的邮箱")
	}
	err := c.post(ctx, activationPrefix+"/unbind/request", map[string]string{
		"code": strings.TrimSpace(code), "email": email, "machine_code": strings.TrimSpace(machineCode),
	}, &result)
	return result, err
}

// ConfirmEmailUnbind releases the machine with the emailed verification code.
func (c *Client) ConfirmEmailUnbind(ctx context.Context, code, email, machineCode, verificationCode string) (ValidateResult, error) {
	var result ValidateResult
	email = strings.TrimSpace(email)
	verificationCode = strings.TrimSpace(verificationCode)
	if strings.TrimSpace(code) == "" || strings.TrimSpace(machineCode) == "" {
		return result, errors.New("激活码和机器码不能为空")
	}
	if email == "" || !strings.Contains(email, "@") {
		return result, errors.New("请填写激活码绑定的邮箱")
	}
	if len(verificationCode) != 6 {
		return result, errors.New("请输入 6 位邮箱验证码")
	}
	for _, char := range verificationCode {
		if char < '0' || char > '9' {
			return result, errors.New("邮箱验证码只能包含数字")
		}
	}
	err := c.post(ctx, activationPrefix+"/unbind/confirm", map[string]string{
		"code": strings.TrimSpace(code), "email": email, "machine_code": strings.TrimSpace(machineCode),
		"verification_code": verificationCode,
	}, &result)
	return result, err
}
