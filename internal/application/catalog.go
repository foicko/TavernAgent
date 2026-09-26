package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	ctxpkg "tavernagent/internal/context"
)

// Catalog 是给两端客户端的「展示目录」（架构设计 §2.1）。
//
// 存在理由：这些枚举与标签必须与后端行为一致，而它们此前在 Web 端
// 另存了一份（providerPresets.ts / modelLabels.ts / ruleCheck.ts），
// 一旦分叉就会出现"界面可选、后端不认"。目录只承载**数据**，
// 不承载行为：真正的推导（预算、标记解析、能力判定）仍在后端各自模块里。
type Catalog struct {
	// Version 是内容哈希：客户端据此判断本地缓存是否过期，无需比对字段。
	Version         string             `json:"version"`
	CardMarkers     CardMarkerSpec     `json:"cardMarkers"`
	Attributes      []AttributeMeta    `json:"attributes"`
	CheckOutcomes   []CheckOutcomeMeta `json:"checkOutcomes"`
	Protocols       []OptionMeta       `json:"protocols"`
	DefaultProtocol string             `json:"defaultProtocol"`
	ThinkingEfforts []OptionHintMeta   `json:"thinkingEfforts"`
	Slots           []SlotMeta         `json:"slots"`
	ProviderPresets []ProviderPreset   `json:"providerPresets"`
	ContextPolicy   ContextPolicyMeta  `json:"contextPolicy"`
}

// CardMarkerSpec 描述卡片 UI 标记的语法版本与形态。
// 状态类标记由服务端在提交时剥离（domain.TurnContent.UIHints），
// 表现类标记由服务端在投影时切段（domain.InlineSegment）——客户端不解析语法。
type CardMarkerSpec struct {
	Grammar     int      `json:"grammar"`
	StateKinds  []string `json:"stateKinds"`
	ImageForms  []string `json:"imageForms"`
	EscapeHint  string   `json:"escapeHint"`
	StripNotice string   `json:"stripNotice"`
}

// AttributeMeta 是检定属性的展示元数据。
type AttributeMeta struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Abbr string `json:"abbr"`
}

// CheckOutcomeMeta 是检定结果的展示元数据。
type CheckOutcomeMeta struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Tone      string `json:"tone"` // crit-up | up | down | crit-down
	Symbol    string `json:"symbol"`
	IsSuccess bool   `json:"isSuccess"`
	IsCrit    bool   `json:"isCrit"`
}

// OptionMeta 是「值 → 人话标签」的通用选项。
type OptionMeta struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// OptionHintMeta 是带说明的选项（如思考强度）。
type OptionHintMeta struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Hint  string `json:"hint"`
}

// SlotMeta 是模型槽位：界面只说人话，不暴露 slot id。
type SlotMeta struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Hint  string `json:"hint"`
}

// ProviderPreset 把"新建连接"从多字段手填降到"选服务商 → 填密钥"。
// baseUrl 只需到版本根：三个协议适配器各自补全具体路径。
type ProviderPreset struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Kind     string   `json:"kind"`
	BaseURL  string   `json:"baseUrl"`
	Models   []string `json:"models"`
	KeyHint  string   `json:"keyHint,omitempty"`
	IsCustom bool     `json:"isCustom,omitempty"`
}

// ContextPolicyMeta 发布预算策略常量。
//
// 它只用于解释与展示（"为什么只给了这么多空间"）；输入预算的实际推导在
// context.InputBudget，模型配置接口会直接回传算好的 inputBudget，
// 客户端不必自己算。
type ContextPolicyMeta struct {
	DefaultWindow      int `json:"defaultWindow"`
	DefaultReservedOut int `json:"defaultReservedOutput"`
	MinMargin          int `json:"minMargin"`
	MarginDivisor      int `json:"marginDivisor"`
	ReservedOutputHint int `json:"reservedOutputHint"`
}

// BuildCatalog 组装展示目录，并写入内容哈希作为版本。
func BuildCatalog() Catalog {
	c := Catalog{
		CardMarkers: CardMarkerSpec{
			Grammar:    1,
			StateKinds: []string{"hud", "scene", "var"},
			ImageForms: []string{`<img cmd="key">`, "[IMG:key]"},
			EscapeHint: "[[ 表示一个字面量 [",
			StripNotice: "状态类标记（HUD/SCENE/VAR）由服务端剥离并结构化下发（uiHints），" +
				"表现类标记（图片）由服务端切段下发（segments）：客户端不解析语法。",
		},
		Attributes: []AttributeMeta{
			{Key: "strength", Name: "力量", Abbr: "STR"},
			{Key: "str", Name: "力量", Abbr: "STR"},
			{Key: "dexterity", Name: "敏捷", Abbr: "DEX"},
			{Key: "dex", Name: "敏捷", Abbr: "DEX"},
			{Key: "constitution", Name: "体质", Abbr: "CON"},
			{Key: "con", Name: "体质", Abbr: "CON"},
			{Key: "intelligence", Name: "智力", Abbr: "INT"},
			{Key: "int", Name: "智力", Abbr: "INT"},
			{Key: "wisdom", Name: "察觉", Abbr: "WIS"},
			{Key: "wis", Name: "察觉", Abbr: "WIS"},
			{Key: "perception", Name: "感知", Abbr: "PER"},
			{Key: "per", Name: "感知", Abbr: "PER"},
			{Key: "charisma", Name: "交涉", Abbr: "CHA"},
			{Key: "cha", Name: "交涉", Abbr: "CHA"},
			{Key: "luck", Name: "运气", Abbr: "LCK"},
			{Key: "lck", Name: "运气", Abbr: "LCK"},
			{Key: "agility", Name: "身手", Abbr: "AGI"},
			{Key: "agi", Name: "身手", Abbr: "AGI"},
		},
		CheckOutcomes: []CheckOutcomeMeta{
			{Key: "critical_success", Label: "大成功", Tone: "crit-up", Symbol: "✦", IsSuccess: true, IsCrit: true},
			{Key: "success", Label: "成功", Tone: "up", Symbol: "✓", IsSuccess: true},
			{Key: "critical_failure", Label: "大失败", Tone: "crit-down", Symbol: "✕", IsCrit: true},
			{Key: "failure", Label: "失败", Tone: "down", Symbol: "✕"},
		},
		Protocols: []OptionMeta{
			{Value: "openai-chat", Label: "OpenAI 兼容"},
			{Value: "openai-responses", Label: "OpenAI Responses"},
			{Value: "anthropic-messages", Label: "Anthropic"},
		},
		DefaultProtocol: "openai-chat",
		ThinkingEfforts: []OptionHintMeta{
			{Value: "", Label: "默认", Hint: "不发送思考参数，完全按供应商自己的默认行为"},
			{Value: "low", Label: "低", Hint: "思考最少，响应最快"},
			{Value: "medium", Label: "中", Hint: "速度与深度的折中"},
			{Value: "high", Label: "高", Hint: "思考最充分，响应最慢"},
		},
		Slots: []SlotMeta{
			{Key: "primary", Label: "对话生成", Hint: "推进剧情时使用"},
			{Key: "assist", Label: "导演协商", Hint: "导演模式下与模型协商大纲"},
			{Key: "reflection", Label: "后台记忆", Hint: "后台抽取记忆、维护摘要"},
		},
		ProviderPresets: []ProviderPreset{
			{ID: "deepseek", Label: "DeepSeek 官方", Kind: "openai-chat", BaseURL: "https://api.deepseek.com",
				Models: []string{"deepseek-chat", "deepseek-reasoner"}, KeyHint: "在 platform.deepseek.com 申请"},
			{ID: "openai", Label: "OpenAI 官方", Kind: "openai-responses", BaseURL: "https://api.openai.com/v1",
				Models: []string{"gpt-4o-mini", "gpt-4o"}, KeyHint: "在 platform.openai.com 申请"},
			{ID: "anthropic", Label: "Anthropic（Claude）", Kind: "anthropic-messages", BaseURL: "https://api.anthropic.com",
				Models: []string{"claude-sonnet-4-5", "claude-haiku-4-5"}, KeyHint: "在 console.anthropic.com 申请"},
			{ID: "ollama", Label: "本地 Ollama", Kind: "openai-chat", BaseURL: "http://127.0.0.1:11434/v1",
				Models: []string{"qwen3:8b", "llama3.1:8b"}, KeyHint: "本地服务，无需密钥"},
			{ID: "oneapi", Label: "本地 OneAPI / NewAPI", Kind: "openai-chat", BaseURL: "http://127.0.0.1:3000/v1",
				Models: []string{}, KeyHint: "填网关里生成的令牌"},
			{ID: "custom", Label: "自定义（手动填写）", Kind: "openai-chat", BaseURL: "",
				Models: []string{}, IsCustom: true},
		},
		ContextPolicy: ContextPolicyMeta{
			DefaultWindow:      ctxpkg.DefaultContextWindow,
			DefaultReservedOut: ctxpkg.DefaultReservedOutput,
			MinMargin:          512,
			MarginDivisor:      20,
			ReservedOutputHint: ctxpkg.DefaultReservedOutput,
		},
	}
	c.Version = catalogVersion(c)
	return c
}

// catalogVersion 用内容哈希做版本：字段变化即版本变化，客户端不必比对字段。
func catalogVersion(c Catalog) string {
	c.Version = ""
	raw, err := json.Marshal(c)
	if err != nil {
		// 目录是本进程内的常量数据，序列化失败属于编程错误；
		// 退化为固定串而不是 panic：目录不可用不该拖垮客户端启动。
		return "catalog-v1"
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:8])
}
