// Package hitokoto fetches a random "一言" sentence for the home page.
package hitokoto

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"MyMinecraftGoLuncher/internal/httpx"
)

// DefaultAPIURL is the default HitokotoGo endpoint.
const DefaultAPIURL = "https://hitokotogo.yhdzz.cn/v2"

// Sentence is a decoded API response.
type Sentence struct {
	ID      int     `json:"id"`
	UUID    string  `json:"uuid"`
	Text    string  `json:"hitokoto"`
	Type    string  `json:"type"`
	From    string  `json:"from"`
	FromWho *string `json:"from_who"`
}

// View is the flat representation exposed to the frontend.
type View struct {
	Text    string `json:"text"`
	Type    string `json:"type"`
	From    string `json:"from"`
	FromWho string `json:"fromWho"`
}

// NormalizeURL validates an API address and applies the default when empty.
func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = DefaultAPIURL
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("一言地址格式无效")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("一言地址只支持 http/https")
	}
	if parsed.Host == "" {
		return "", errors.New("一言地址缺少主机名")
	}
	parsed.Fragment = ""
	return parsed.String(), nil
}

// Fetch requests one random sentence.
func Fetch(ctx context.Context, apiURL string) (View, error) {
	endpoint, err := NormalizeURL(apiURL)
	if err != nil {
		return View{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var sentence Sentence
	if err := httpx.GetJSON(ctx, endpoint, &sentence); err != nil {
		return View{}, fmt.Errorf("获取一言失败: %w", err)
	}
	text := strings.TrimSpace(sentence.Text)
	if text == "" {
		return View{}, errors.New("一言服务返回了空内容")
	}
	fromWho := ""
	if sentence.FromWho != nil {
		fromWho = strings.TrimSpace(*sentence.FromWho)
	}
	return View{
		Text:    text,
		Type:    strings.TrimSpace(sentence.Type),
		From:    strings.TrimSpace(sentence.From),
		FromWho: fromWho,
	}, nil
}
