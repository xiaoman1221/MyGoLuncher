package aisearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"MyMinecraftGoLuncher/internal/httpx"
)

type orcaResponsesRequest struct {
	Model           string `json:"model"`
	Instructions    string `json:"instructions"`
	Input           string `json:"input"`
	MaxOutputTokens int    `json:"max_output_tokens,omitempty"`
}

type orcaContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type orcaOutput struct {
	Type    string        `json:"type"`
	Content []orcaContent `json:"content"`
}

type orcaResponsesResponse struct {
	OutputText string       `json:"output_text"`
	Output     []orcaOutput `json:"output"`
	Error      *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func orcaResponsesEndpoint(baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = DefaultOrcaRouterBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, "/responses") {
		return baseURL
	}
	return baseURL + "/responses"
}

func translateOrcaRouter(ctx context.Context, cfg Config, model, query string) (string, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv(OrcaRouterEnvKey))
	}
	if apiKey == "" {
		return "", errors.New("未配置 OrcaRouter API Key（也可设置 ORCA_KEY 环境变量）")
	}
	payload := orcaResponsesRequest{
		Model:           model,
		Instructions:    systemPrompt,
		Input:           query,
		MaxOutputTokens: 512,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	requestCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, orcaResponsesEndpoint(cfg.BaseURL), bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", httpx.UserAgent)
	resp, err := httpx.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("OrcaRouter 请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("OrcaRouter 请求失败: %s", orcaErrorMessage(body, resp.Status))
	}
	var result orcaResponsesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析 OrcaRouter 响应失败: %w", err)
	}
	if result.Error != nil && strings.TrimSpace(result.Error.Message) != "" {
		return "", fmt.Errorf("OrcaRouter 请求失败: %s", result.Error.Message)
	}
	text := strings.TrimSpace(result.OutputText)
	if text == "" {
		var parts []string
		for _, output := range result.Output {
			for _, content := range output.Content {
				if strings.TrimSpace(content.Text) == "" {
					continue
				}
				switch content.Type {
				case "", "output_text", "text", "input_text":
					parts = append(parts, content.Text)
				}
			}
		}
		text = strings.Join(parts, " ")
	}
	value := normalizeResponse(text)
	if value == "" {
		return "", errors.New("OrcaRouter 未返回搜索关键词")
	}
	return value, nil
}

func orcaErrorMessage(body []byte, status string) string {
	var failure struct {
		Error any `json:"error"`
	}
	_ = json.Unmarshal(body, &failure)
	var message string
	switch value := failure.Error.(type) {
	case string:
		message = value
	case map[string]any:
		if text, ok := value["message"].(string); ok {
			message = text
		}
	}
	if message == "" {
		var generic struct {
			Message string `json:"message"`
			Detail  string `json:"detail"`
		}
		if json.Unmarshal(body, &generic) == nil {
			message = firstNonEmpty(generic.Message, generic.Detail)
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
