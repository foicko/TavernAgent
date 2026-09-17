// 模型配置端口：配置 Go 对象、配置存储与供应商构建器。
// 与 Store 分离：配置存储在文件（settings.json + secrets.json），DB 只存剧情权威记录。
//
// 配置模型（v2）：**模型实例**是唯一携带连接信息的地方（协议、地址、模型名、
// 密钥、温度、窗口）；三个槽位（主线/辅助/反思）只保存 enabled + modelId 引用。
package ports

import (
	"errors"
	"strings"
)

// ProviderSlot 是模型槽位。
type ProviderSlot string

const (
	SlotPrimary    ProviderSlot = "primary"
	SlotAssist     ProviderSlot = "assist"
	SlotReflection ProviderSlot = "reflection"
)

// KnownSlots 是固定槽位集合（顺序即界面顺序）。
func KnownSlots() []string {
	return []string{string(SlotPrimary), string(SlotAssist), string(SlotReflection)}
}

// ProviderConfig 是**解析后**的单个槽位供应商配置：槽位 + 它引用到的实例字段。
// 它是运行时与台账的视图（generation/预算/用量都读它），不是磁盘形态。
// APIKey 只在写入时携带；读取（LoadCatalog(false)）不脱敏，接口层才脱敏。
type ProviderConfig struct {
	Slot            string   `json:"slot"`
	Enabled         bool     `json:"enabled"`
	Kind            string   `json:"kind"` // openai-chat | openai-responses | anthropic-messages
	BaseURL         string   `json:"baseUrl,omitempty"`
	Model           string   `json:"model,omitempty"`
	APIKey          string   `json:"apiKey,omitempty"`
	HasAPIKey       bool     `json:"hasApiKey"`
	ModelID         string   `json:"modelId,omitempty"` // 生效的实例 ID（跟随主线时是主线的 ID）
	Temperature     *float64 `json:"temperature,omitempty"`
	MaxTokens       int      `json:"maxTokens,omitempty"`
	ContextWindow   int      `json:"contextWindow,omitempty"`
	ReasoningEffort string   `json:"reasoningEffort,omitempty"` // low | medium | high；空 = 默认（不发送）
}

// ModelInstance 是一个可复用的模型实例：用户新建一次，多个槽位引用它。
type ModelInstance struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Kind            string   `json:"kind"`
	BaseURL         string   `json:"baseUrl,omitempty"`
	Model           string   `json:"model,omitempty"`
	APIKey          string   `json:"apiKey,omitempty"` // 只在写入时携带
	HasAPIKey       bool     `json:"hasApiKey"`
	Temperature     *float64 `json:"temperature,omitempty"`
	MaxTokens       int      `json:"maxTokens,omitempty"`
	ContextWindow   int      `json:"contextWindow,omitempty"`
	ReasoningEffort string   `json:"reasoningEffort,omitempty"` // low | medium | high；空 = 默认（不发送）
}

// EffortLevels 返回可显式设置的思考强度档位（不含表示"默认"的空串）。
// 语义按连接所配协议映射：Responses → reasoning.effort；Chat Completions →
// reasoning_effort；Anthropic → extended thinking 的 token 预算。
func EffortLevels() []string {
	return []string{"low", "medium", "high"}
}

// NormalizeEffort 归一化思考强度：去空白并转小写；空串合法，表示"默认"（不发送）。
// 无法识别的值返回 ok=false，由调用方决定忽略（读盘）还是报错（接口写入）。
func NormalizeEffort(raw string) (string, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "", true
	}
	for _, level := range EffortLevels() {
		if level == value {
			return value, true
		}
	}
	return "", false
}

// SlotBinding 是槽位的保存形态与解析结果的合体：
//   - 保存形态只有 Enabled + ModelID（ModelID 为空 = 跟随主线）；
//   - 其余字段是解析后用于界面直接显示的实例信息（ModelID 为空时 ResolvedModelID 指向主线实例）。
type SlotBinding struct {
	Slot            string   `json:"slot"`
	Enabled         bool     `json:"enabled"`
	ModelID         string   `json:"modelId,omitempty"`
	ResolvedModelID string   `json:"resolvedModelId,omitempty"`
	Model           string   `json:"model,omitempty"`
	Kind            string   `json:"kind,omitempty"`
	BaseURL         string   `json:"baseUrl,omitempty"`
	APIKey          string   `json:"apiKey,omitempty"`
	HasAPIKey       bool     `json:"hasApiKey"`
	Temperature     *float64 `json:"temperature,omitempty"`
	MaxTokens       int      `json:"maxTokens,omitempty"`
	ContextWindow   int      `json:"contextWindow,omitempty"`
	ReasoningEffort string   `json:"reasoningEffort,omitempty"`
}

// ModelCatalog 是一次读取的完整配置视图：实例列表 + 三个槽位。
type ModelCatalog struct {
	Models []ModelInstance `json:"models"`
	Slots  []SlotBinding   `json:"slots"`
}

// ModelConfigStore 持久化模型实例与槽位指派。实现需保证 API Key 与普通设置分离存放。
type ModelConfigStore interface {
	// LoadCatalog 返回全部实例与槽位；masked=true 时 APIKey 为脱敏值。
	LoadCatalog(masked bool) (*ModelCatalog, error)
	// SaveModel 新建或更新一个实例，返回脱敏后的结果（ID 为空时由实现分配）。
	SaveModel(instance ModelInstance) (ModelInstance, error)
	// DeleteModel 删除实例；仍被槽位引用时返回 *ModelInUseError。
	DeleteModel(id string) error
	// SaveSlot 保存槽位指派：空 modelID 表示跟随主线。
	SaveSlot(slot string, enabled bool, modelID string) error
}

// ErrModelNotFound 表示槽位引用了不存在的实例。
var ErrModelNotFound = errors.New("模型实例不存在")

// ModelInUseError 表示实例仍被槽位引用（删除被拒绝）。
type ModelInUseError struct {
	Slots []string
}

func (e *ModelInUseError) Error() string {
	return "模型实例仍被槽位使用: " + strings.Join(e.Slots, ", ")
}

// ProviderBuilder 从配置构造具体供应商（由组合根注入，避免应用层依赖具体适配器）。
type ProviderBuilder func(cfg ProviderConfig) (ModelProvider, error)

// MaskAPIKey 脱敏密钥：只保留末四位，如 sk-…abcd。
func MaskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "…" + key[len(key)-4:]
}
