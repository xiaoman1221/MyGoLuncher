// Package aisearch turns free-form Chinese Minecraft queries into concise
// English search keywords using Claude, with a small offline alias fallback.
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
	"unicode"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"MyMinecraftGoLuncher/internal/httpx"
)

const systemPrompt = `You convert Minecraft launcher search queries into concise English keyword queries for Modrinth and CurseForge.

Rules:
- Return only the search keywords on one line.
- Do not add quotes, labels, explanations, or punctuation around the result.
- Preserve mod names, loader names, and version numbers.
- Translate Chinese mod nicknames and game terms into their common English names.
- If the input is already English, keep it unchanged unless a widely used synonym would search better.
- Keep the result under 12 words.`

// Config holds AI provider settings.
type Config struct {
	Provider string
	APIKey   string
	BaseURL  string
	Model    string
}

// DefaultModelForProvider returns the standard model for a provider.
func DefaultModelForProvider(provider string) string {
	if config, ok := LookupProvider(provider); ok {
		return config.DefaultModel
	}
	return DefaultDeepSeekModel
}

func normalizeProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "anthropic", "claude":
		return ProviderAnthropic
	case "deepseek":
		return ProviderDeepSeek
	case "orcarouter", "orca-router", "orca":
		return ProviderOrcaRouter
	default:
		return ""
	}
}

// TranslateQuery asks the selected provider for concise English keywords.
func TranslateQuery(ctx context.Context, cfg Config, query string) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", nil
	}
	model := strings.TrimSpace(cfg.Model)
	provider := normalizeProvider(cfg.Provider)
	if provider == "" {
		provider = ModelProvider(model)
	}
	if provider == "" {
		provider = ProviderDeepSeek
	}
	if model == "" {
		model = DefaultModelForProvider(provider)
	}
	switch provider {
	case ProviderAnthropic:
		return translateAnthropic(ctx, cfg, model, query)
	case ProviderOrcaRouter:
		return translateOrcaRouter(ctx, cfg, model, query)
	default:
		return translateDeepSeek(ctx, cfg, model, query)
	}
}

func translateAnthropic(ctx context.Context, cfg Config, model, query string) (string, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	}
	if apiKey == "" {
		return "", errors.New("未配置 Anthropic API Key")
	}
	options := []option.RequestOption{option.WithAPIKey(apiKey)}
	if baseURL := strings.TrimSpace(cfg.BaseURL); baseURL != "" {
		options = append(options, option.WithBaseURL(strings.TrimRight(baseURL, "/")))
	}
	client := anthropic.NewClient(options...)

	requestCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	message, err := client.Messages.New(requestCtx, anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: 4096,
		System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(query)),
		},
	})
	if err != nil {
		return "", err
	}
	var parts []string
	for _, block := range message.Content {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
			parts = append(parts, block.Text)
		}
	}
	result := normalizeResponse(strings.Join(parts, " "))
	if result == "" {
		return "", errors.New("AI 未返回搜索关键词")
	}
	return result, nil
}

type deepSeekMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type deepSeekRequest struct {
	Model       string            `json:"model"`
	Messages    []deepSeekMessage `json:"messages"`
	Temperature float64           `json:"temperature"`
	MaxTokens   int               `json:"max_tokens"`
	Stream      bool              `json:"stream"`
}

type deepSeekResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func deepSeekEndpoint(baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = DefaultDeepSeekBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return baseURL
	}
	return baseURL + "/chat/completions"
}

func translateDeepSeek(ctx context.Context, cfg Config, model, query string) (string, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
	}
	if apiKey == "" {
		return "", errors.New("未配置 DeepSeek API Key")
	}
	payload := deepSeekRequest{
		Model: model,
		Messages: []deepSeekMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: query},
		},
		Temperature: 0,
		MaxTokens:   512,
		Stream:      false,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	requestCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, deepSeekEndpoint(cfg.BaseURL), bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", httpx.UserAgent)
	resp, err := httpx.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("DeepSeek 请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var failure deepSeekResponse
		_ = json.Unmarshal(body, &failure)
		if failure.Error != nil && strings.TrimSpace(failure.Error.Message) != "" {
			return "", fmt.Errorf("DeepSeek 请求失败: %s", failure.Error.Message)
		}
		text := strings.TrimSpace(string(body))
		if len(text) > 300 {
			text = text[:300]
		}
		if text == "" {
			text = resp.Status
		}
		return "", fmt.Errorf("DeepSeek 请求失败: %s", text)
	}
	var result deepSeekResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析 DeepSeek 响应失败: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", errors.New("DeepSeek 未返回搜索关键词")
	}
	value := normalizeResponse(result.Choices[0].Message.Content)
	if value == "" {
		return "", errors.New("DeepSeek 未返回搜索关键词")
	}
	return value, nil
}

func normalizeResponse(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"'`“”‘’")
	value = strings.Join(strings.Fields(value), " ")
	if len([]rune(value)) > 160 {
		value = string([]rune(value)[:160])
	}
	return strings.TrimSpace(value)
}

var aliases = []struct {
	from string
	to   string
}{
	{"苹果皮", "appleskin"},
	{"暮色森林", "twilight forest"},
	{"机械动力", "create"},
	{"农夫乐事", "farmers delight"},
	{"匠魂", "tinkers construct"},
	{"植物魔法", "botania"},
	{"沉浸工程", "immersive engineering"},
	{"储物抽屉", "storage drawers"},
	{"动态光源", "dynamic lights"},
	{"连接纹理", "connected textures"},
	{"旅行地图", "journeymap"},
	{"小地图", "minimap"},
	{"一键整理", "inventory sorting"},
	{"区块加载", "chunk loading"},
	{"钠", "sodium"},
	{"锂", "lithium"},
	{"铷", "rubidium"},
	{"光影", "shader"},
	{"材质包", "resource pack"},
	{"资源包", "resource pack"},
	{"优化", "optimization"},
}

// NormalizeQuery applies the built-in Chinese alias table.
func NormalizeQuery(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}
	result := query
	for _, alias := range aliases {
		result = strings.ReplaceAll(result, alias.from, alias.to)
	}
	return strings.Join(strings.Fields(result), " ")
}

// HasCJK reports whether the query contains Chinese, Japanese, or Korean text.
func HasCJK(value string) bool {
	for _, r := range value {
		if unicode.Is(unicode.Han, r) ||
			unicode.Is(unicode.Hiragana, r) ||
			unicode.Is(unicode.Katakana, r) ||
			unicode.Is(unicode.Hangul, r) {
			return true
		}
	}
	return false
}
