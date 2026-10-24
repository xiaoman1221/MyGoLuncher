package aisearch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeQuery(t *testing.T) {
	cases := map[string]string{
		"钠 优化":   "sodium optimization",
		"小地图":    "minimap",
		"苹果皮":    "appleskin",
		"sodium": "sodium",
	}
	for input, want := range cases {
		if got := NormalizeQuery(input); got != want {
			t.Fatalf("NormalizeQuery(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestHasCJK(t *testing.T) {
	if !HasCJK("小地图") || !HasCJK("マインクラフト") || !HasCJK("마인크래프트") {
		t.Fatal("CJK text should be detected")
	}
	if HasCJK("sodium optimization") {
		t.Fatal("Latin text should not be detected as CJK")
	}
}

func TestTranslateQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "钠") {
			t.Errorf("request did not contain the query: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":            "msg_test",
			"type":          "message",
			"role":          "assistant",
			"model":         "claude-opus-5-5",
			"content":       []map[string]string{{"type": "text", "text": "\"sodium optimization\""}},
			"stop_reason":   "end_turn",
			"stop_sequence": nil,
			"usage":         map[string]int{"input_tokens": 10, "output_tokens": 5},
		})
	}))
	defer server.Close()

	got, err := TranslateQuery(context.Background(), Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "claude-opus-5-5",
	}, "钠 优化")
	if err != nil {
		t.Fatal(err)
	}
	if got != "sodium optimization" {
		t.Fatalf("translated query = %q", got)
	}
}

func TestTranslateQueryDeepSeek(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Error("missing bearer token")
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "deepseek-chat") || !strings.Contains(string(body), "钠") {
			t.Errorf("unexpected request: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]string{"role": "assistant", "content": "sodium optimization"},
			}},
		})
	}))
	defer server.Close()

	got, err := TranslateQuery(context.Background(), Config{
		Provider: ProviderDeepSeek,
		APIKey:   "test-key",
		BaseURL:  server.URL,
		Model:    DefaultDeepSeekModel,
	}, "钠 优化")
	if err != nil {
		t.Fatal(err)
	}
	if got != "sodium optimization" {
		t.Fatalf("translated query = %q", got)
	}
}

func TestQueryCandidatesChineseAliases(t *testing.T) {
	cases := map[string][]string{
		"钠 优化":        {"sodium optimization", "钠 优化"},
		"机械动力 模组":     {"create mod", "机械动力 模组"},
		"物品管理器":       {"jei just enough items", "物品管理器"},
		"未知中文模组":      {"未知中文mod", "未知中文模组"},
		"Create 机械动力": {"Create create", "Create 机械动力"},
	}
	for query, want := range cases {
		got := QueryCandidates(query)
		if len(got) != len(want) {
			t.Fatalf("QueryCandidates(%q) = %#v, want %#v", query, got, want)
		}
		for index := range want {
			if got[index] != want[index] {
				t.Fatalf("QueryCandidates(%q) = %#v, want %#v", query, got, want)
			}
		}
	}
}

func TestProviderDefaults(t *testing.T) {
	if DefaultModelForProvider(ProviderDeepSeek) != "deepseek-chat" {
		t.Fatal("unexpected DeepSeek default model")
	}
	if DefaultModelForProvider(ProviderAnthropic) != "claude-opus-5-5" {
		t.Fatal("unexpected Anthropic default model")
	}
	if DefaultModelForProvider(ProviderOrcaRouter) != "orcarouter/auto" {
		t.Fatal("unexpected OrcaRouter default model")
	}
}

func TestTranslateQueryRequiresKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("DEEPSEEK_API_KEY", "")
	t.Setenv("ORCA_KEY", "")
	if _, err := TranslateQuery(context.Background(), Config{}, "钠"); err == nil {
		t.Fatal("missing API key should fail")
	}
}
