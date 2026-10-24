package aisearch

import (
	_ "embed"
	"encoding/json"
	"strings"
)

// Supported AI providers.
const (
	ProviderDeepSeek   = "deepseek"
	ProviderAnthropic  = "anthropic"
	ProviderOrcaRouter = "orcarouter"

	DefaultDeepSeekBaseURL   = "https://api.deepseek.com"
	DefaultDeepSeekModel     = "deepseek-chat"
	DefaultAnthropicBaseURL  = "https://api.anthropic.com"
	DefaultAnthropicModel    = "claude-opus-5-5"
	DefaultOrcaRouterBaseURL = "https://api.orcarouter.ai/v1"
	DefaultOrcaRouterModel   = "orcarouter/auto"
	OrcaRouterEnvKey         = "ORCA_KEY"
	OrcaRouterWireAPI        = "responses"
	OrcaAuthURL              = "https://www.arouter.ai/auth"
	OrcaTokenURL             = "https://api.arouter.ai/api/v1/auth/keys"
	OrcaAppName              = "MyGoLuncher"
	OrcaRef                  = "ref_00ee071bfd29fc8bb954"
)

// ProviderConfig is the provider registry entry used by the AI search client.
// It mirrors the common model-provider configuration fields: name, base_url,
// wire_api and env_key.
type ProviderConfig struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	BaseURL      string `json:"base_url"`
	WireAPI      string `json:"wire_api"`
	EnvKey       string `json:"env_key"`
	DefaultModel string `json:"default_model"`
}

//go:embed providers.json
var providerConfigData []byte

// ProviderConfigs is the provider configuration. The embedded providers.json
// is the source of truth; the code fallback keeps the launcher usable if the
// embedded file is ever malformed.
var ProviderConfigs = loadProviderConfigs()

func loadProviderConfigs() []ProviderConfig {
	var db struct {
		Providers []ProviderConfig `json:"providers"`
	}
	if err := json.Unmarshal(providerConfigData, &db); err == nil && len(db.Providers) > 0 {
		return db.Providers
	}
	return []ProviderConfig{
		{
			ID: ProviderDeepSeek, Name: "DeepSeek", BaseURL: DefaultDeepSeekBaseURL,
			WireAPI: "chat", EnvKey: "DEEPSEEK_API_KEY", DefaultModel: DefaultDeepSeekModel,
		},
		{
			ID: ProviderAnthropic, Name: "Anthropic Claude", BaseURL: DefaultAnthropicBaseURL,
			WireAPI: "messages", EnvKey: "ANTHROPIC_API_KEY", DefaultModel: DefaultAnthropicModel,
		},
		{
			ID: ProviderOrcaRouter, Name: "OrcaRouter", BaseURL: DefaultOrcaRouterBaseURL,
			WireAPI: OrcaRouterWireAPI, EnvKey: OrcaRouterEnvKey, DefaultModel: DefaultOrcaRouterModel,
		},
	}
}

// LookupProvider returns a provider configuration by id or alias.
func LookupProvider(id string) (ProviderConfig, bool) {
	id = normalizeProvider(id)
	for _, provider := range ProviderConfigs {
		if provider.ID == id {
			return provider, true
		}
	}
	return ProviderConfig{}, false
}

// ProviderIDs returns the canonical provider ids.
func ProviderIDs() []string {
	out := make([]string, 0, len(ProviderConfigs))
	for _, provider := range ProviderConfigs {
		out = append(out, provider.ID)
	}
	return out
}

// ModelProvider maps a model id such as "orcarouter/auto" to its provider.
func ModelProvider(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.HasPrefix(model, "orcarouter/"), strings.HasPrefix(model, "orca/"):
		return ProviderOrcaRouter
	case strings.HasPrefix(model, "claude"):
		return ProviderAnthropic
	case strings.HasPrefix(model, "deepseek"):
		return ProviderDeepSeek
	default:
		return ""
	}
}
