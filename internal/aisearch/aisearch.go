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
	"sort"
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
	// Type and gameplay terms.
	{"整合包", "modpack"}, {"模组包", "modpack"}, {"材质包", "resource pack"},
	{"资源包", "resource pack"}, {"数据包", "datapack"}, {"光影包", "shader"},
	{"模组", "mod"}, {"插件", "plugin"}, {"光影", "shader"}, {"优化", "optimization"},
	{"性能", "performance"}, {"家具", "furniture"}, {"农业", "farming"},
	{"食物", "food"}, {"生物", "mobs"}, {"维度", "dimension"}, {"地牢", "dungeon"},
	{"武器", "weapon"}, {"工具", "tools"}, {"附魔", "enchantment"}, {"背包", "backpack"},
	{"任务", "quest"}, {"小地图", "minimap"}, {"地图", "map"}, {"传送", "teleport"},
	{"联机", "multiplayer"}, {"服务器", "server"}, {"红石", "redstone"},
	{"自动化", "automation"}, {"能源", "energy"}, {"管道", "pipes"},
	{"电线", "cables"}, {"反应堆", "reactor"}, {"火箭", "rocket"},
	{"太空", "space"}, {"枪械", "guns"}, {"载具", "vehicles"}, {"飞机", "plane"},
	{"汽车", "car"}, {"铁路", "rail"}, {"村民", "villager"}, {"交易", "trading"},
	{"刷怪", "spawner"}, {"怪物", "monster"}, {"装备", "equipment"}, {"饰品", "trinkets"},
	{"药水", "potion"}, {"烹饪", "cooking"}, {"饮料", "drink"}, {"咖啡", "coffee"},
	{"建筑", "building"}, {"冒险", "adventure"}, {"科技", "tech"}, {"魔法", "magic"},
	{"动态光源", "dynamic lights"}, {"连接纹理", "connected textures"},
	{"区块加载", "chunk loading"}, {"一键整理", "inventory sorting"},
	{"物品管理器", "jei just enough items"}, {"配方查看", "recipe viewer"},

	// Popular mods and common Chinese nicknames.
	{"高清修复", "optifine"}, {"钠", "sodium"}, {"锂", "lithium"}, {"铷", "rubidium"},
	{"磷", "phosphor"}, {"镁", "magnesium"}, {"锰", "manganese"},
	{"苹果皮", "appleskin"}, {"旅行地图", "journeymap"}, {"玉", "jade"},
	{"暮色森林", "twilight forest"}, {"天境", "aether"}, {"机械动力", "create"},
	{"沉浸工程", "immersive engineering"}, {"通用机械", "mekanism"},
	{"热力膨胀", "thermal expansion"}, {"工业时代", "industrialcraft"},
	{"工业", "industrialcraft"}, {"植物魔法", "botania"}, {"神秘时代", "thaumcraft"},
	{"匠魂", "tinkers construct"}, {"农夫乐事", "farmers delight"},
	{"潘马斯农场", "pams harvestcraft"}, {"应用能源", "applied energistics"},
	{"精致存储", "refined storage"}, {"储物抽屉", "storage drawers"},
	{"简易存储", "simple storage network"}, {"等价交换", "projecte"},
	{"无尽贪婪", "avaritia"}, {"血魔法", "blood magic"}, {"星辉魔法", "astral sorcery"},
	{"深渊国度", "abyssalcraft"}, {"虚无世界", "divine rpg"}, {"龙之研究", "draconic evolution"},
	{"新生魔艺", "ars nouveau"}, {"神秘农业", "mystical agriculture"},
	{"方块宝可梦", "cobblemon"}, {"精灵宝可梦", "pixelmon"},
	{"亚历克斯的生物", "alexs mobs"}, {"冰与火", "ice and fire"},
	{"更好的下界", "better nether"}, {"更好的末地", "better end"},
	{"遥远地平线", "distant horizons"}, {"精致背包", "sophisticated backpacks"},
	{"旅行者背包", "travelers backpack"}, {"传送石碑", "waystones"},
	{"鼠标手势", "mouse tweaks"}, {"按键冲突", "controlling"},
	{"区块预生成", "chunky"}, {"性能分析", "spark"}, {"实体剔除", "entity culling"},
	{"合成修改", "crafttweaker"}, {"任务书", "ftb quests"},
	{"史诗战斗", "epic fight"}, {"战斗", "combat"}, {"神化", "apotheosis"},
	{"饰品栏", "curios"}, {"手册", "patchouli"}, {"滑翔伞", "paraglider"},
	{"创世神", "worldedit"}, {"建筑魔杖", "building gadgets"},
	{"车万女仆", "touhou little maid"}, {"女仆", "maid"},
	{"幸运方块", "lucky block"}, {"更多生物", "more mobs"},
	{"更多食物", "more food"}, {"更多附魔", "more enchantments"},
	{"更多箱子", "iron chests"}, {"铁箱子", "iron chests"},
}

func init() {
	// Replace longer phrases first so "物品管理器" wins over "物品".
	sort.SliceStable(aliases, func(i, j int) bool {
		return len([]rune(aliases[i].from)) > len([]rune(aliases[j].from))
	})
}

// traditionalToSimplified covers the traditional characters used by common
// Minecraft mod names. A full conversion table is unnecessary; this keeps
// queries copied from Traditional Chinese clients searchable.
var traditionalToSimplified = strings.NewReplacer(
	"機", "机", "動", "动", "業", "业", "農", "农", "樂", "乐", "鈉", "钠",
	"鋰", "锂", "銣", "铷", "鎂", "镁", "錳", "锰", "蘋", "苹", "熱", "热",
	"脹", "胀", "馬", "马", "應", "应", "緻", "致", "儲", "储", "屜", "屉",
	"簡", "简", "價", "价", "換", "换", "無", "无", "盡", "尽", "貪", "贪",
	"輝", "辉", "淵", "渊", "國", "国", "虛", "虚", "龍", "龙", "藝", "艺",
	"塊", "块", "寶", "宝", "夢", "梦", "靈", "灵", "亞", "亚", "歷", "历",
	"與", "与", "遙", "遥", "遠", "远", "線", "线", "傳", "传", "標", "标",
	"勢", "势", "鍵", "键", "衝", "冲", "區", "区", "預", "预", "實", "实",
	"體", "体", "務", "务", "書", "书", "詩", "诗", "戰", "战", "鬥", "斗",
	"話", "话", "飾", "饰", "欄", "栏", "冊", "册", "傘", "伞", "創", "创",
	"築", "筑", "車", "车", "萬", "万", "僕", "仆", "運", "运", "鐵", "铁",
	"經", "经", "濟", "济", "記", "记", "憶", "忆", "點", "点", "選", "选",
	"單", "单", "開", "开", "關", "关", "軟", "软", "驗", "验", "證", "证",
)

func isNoiseSearchToken(token string) bool {
	switch strings.ToLower(token) {
	case "minecraft", "mc", "mod", "mods", "plugin", "plugins", "addon", "addons",
		"模组", "插件", "我的世界":
		return true
	}
	return false
}

// NormalizeQuery applies the built-in Chinese alias table. Chinese punctuation,
// full-width characters and common Traditional Chinese variants are normalized
// so queries copied from a client or chat still match.
func NormalizeQuery(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}
	query = strings.NewReplacer(
		"，", " ", "。", " ", "！", " ", "？", " ", "、", " ",
		"：", " ", "；", " ", "（", " ", "）", " ", "【", " ", "】", " ",
		"Ａ", "A", "Ｂ", "B", "Ｃ", "C", "Ｄ", "D", "Ｅ", "E", "Ｆ", "F",
		"Ｇ", "G", "Ｈ", "H", "Ｉ", "I", "Ｊ", "J", "Ｋ", "K", "Ｌ", "L",
		"Ｍ", "M", "Ｎ", "N", "Ｏ", "O", "Ｐ", "P", "Ｑ", "Q", "Ｒ", "R",
		"Ｓ", "S", "Ｔ", "T", "Ｕ", "U", "Ｖ", "V", "Ｗ", "W", "Ｘ", "X",
		"Ｙ", "Y", "Ｚ", "Z", "０", "0", "１", "1", "２", "2", "３", "3",
		"４", "4", "５", "5", "６", "6", "７", "7", "８", "8", "９", "9",
	).Replace(query)
	result := traditionalToSimplified.Replace(query)
	for _, alias := range aliases {
		result = strings.ReplaceAll(result, alias.from, alias.to)
	}

	tokens := strings.Fields(result)
	seen := map[string]bool{}
	unique := make([]string, 0, len(tokens))
	meaningfulLatin := false
	hasCJK := false
	for _, token := range tokens {
		lower := strings.ToLower(token)
		if seen[lower] {
			continue
		}
		seen[lower] = true
		unique = append(unique, token)
		if HasCJK(token) {
			hasCJK = true
		} else if !isNoiseSearchToken(token) {
			meaningfulLatin = true
		}
	}
	// "sodium 好用的" should search "sodium", not a phrase that no project
	// title contains. Keep the Chinese token when it is the only signal.
	if meaningfulLatin && hasCJK {
		filtered := unique[:0]
		for _, token := range unique {
			if !HasCJK(token) {
				filtered = append(filtered, token)
			}
		}
		unique = filtered
	}
	// Drop filler words only when a real keyword remains.
	if len(unique) > 1 {
		filtered := unique[:0]
		for _, token := range unique {
			if !isNoiseSearchToken(token) {
				filtered = append(filtered, token)
			}
		}
		if len(filtered) > 0 {
			unique = filtered
		}
	}
	return strings.Join(unique, " ")
}

// QueryCandidates returns the primary normalized query followed by the
// original query. Searching both means an unlisted Chinese name can still hit
// projects that use Chinese titles, while known aliases produce better English
// results first.
func QueryCandidates(query string) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, 2)
	add := func(value string) {
		value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
		if value == "" || seen[strings.ToLower(value)] {
			return
		}
		seen[strings.ToLower(value)] = true
		out = append(out, value)
	}
	add(NormalizeQuery(query))
	add(query)
	return out
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
