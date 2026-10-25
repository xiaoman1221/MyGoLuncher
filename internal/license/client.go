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
var AppVersion = "1.0.0"

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

// ActivateRequest is sent to /activate.
type ActivateRequest struct {
	Code        string         `json:"code"`
	MachineCode string         `json:"machine_code"`
	MachineName string         `json:"machine_name,omitempty"`
	OS          string         `json:"os,omitempty"`
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
	if strings.TrimSpace(req.Code) == "" || strings.TrimSpace(req.MachineCode) == "" {
		return result, errors.New("激活码和机器码不能为空")
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
