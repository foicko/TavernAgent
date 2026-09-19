// Package domain 定义领域实体、事件与纯状态求值。
// 领域层不依赖 HTTP、GUI、模型 SDK 或数据库具体实现（设计原则 8）。
package domain

import (
	"encoding/json"
	"strings"
	"time"
)

// ---- 标识与通用 ----

// ID 为不透明字符串，由服务端用 UUIDv7 生成。
// 外部 JSON 字段使用 camelCase，数据库列使用 snake_case（技术契约 §2）。

type NodeKind string

const (
	NodeKindRoot                NodeKind = "root"
	NodeKindTurn                NodeKind = "turn"
	NodeKindConfigurationChange NodeKind = "configuration_change"
	NodeKindMemoryChange        NodeKind = "memory_change"
	NodeKindDirectorEvent       NodeKind = "director_event"
)

// TemplateKind 标识角色、玩家、世界书与规则模板。
type TemplateKind string

const (
	TemplateCharacter TemplateKind = "character"
	TemplatePlayer    TemplateKind = "player"
	TemplateLorebook  TemplateKind = "lorebook"
	TemplateRules     TemplateKind = "rules"
)

// ---- 会话与模板 ----

// Session 是一个会话的根，包含根节点引用。
type Session struct {
	SessionID  string    `json:"sessionId"`
	RootNodeID string    `json:"rootNodeId"`
	Title      string    `json:"title"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	// RulesetVersion 是会话创建时采用的规则版本。
	// 下沉到会话而不是用编译期常量：规则升级应当通过显式迁移事件进行，
	// 新旧数据不能混在同一个值里（技术契约 §7）。空串表示旧数据，
	// 读取方回退到当前版本常量。
	RulesetVersion string `json:"rulesetVersion,omitempty"`
	// CharacterID 是会话归属的角色卡 ID（M4l 双向硬绑定，技术契约 §11.2）。
	// 空串表示旧数据（迁移前的会话）——校验方跳过匹配检查，
	// 写路径不受影响；新会话创建时由角色卡的稳定 ID 填充。
	CharacterID string `json:"characterId,omitempty"`
}

// TemplateVersion 是角色、玩家、世界书与规则模板的一个不可变版本。
// 模板内容版本化：现有会话通过 configuration_change 节点迁移，不读取最新文件。
type TemplateVersion struct {
	TemplateVersionID string       `json:"templateVersionId"`
	Kind              TemplateKind `json:"kind"`
	SchemaVersion     int          `json:"schemaVersion"`
	Content           string       `json:"content"`
	ContentHash       string       `json:"contentHash"`
}

// ---- 世界书 ----

// LorebookEntry 是世界书的一条设定条目。
// 触发方式为首版关键词命中：条目的任一 key 出现在当前输入或最近正文中即注入。
// 条件秘密使用独立 secrets 定义，不放入公开世界书索引。
type LorebookEntry struct {
	EntryID       string   `json:"entryId,omitempty"`
	Title         string   `json:"title,omitempty"`
	Keys          []string `json:"keys"`
	SecondaryKeys []string `json:"secondaryKeys,omitempty"` // 次要关键词（主词命中且任一次要词也命中时方才激活）
	Content       string   `json:"content"`
	Enabled       bool     `json:"enabled"` // 禁用条目不参与命中
}

// Lorebook 是世界书（V2/V3 character_book 的规范化形态）。
type Lorebook struct {
	RefID   string          `json:"refId,omitempty"`
	Name    string          `json:"name,omitempty"`
	Schema  string          `json:"schema,omitempty"` // v2 | v3
	Entries []LorebookEntry `json:"entries,omitempty"`
}

// EnabledEntries 返回启用中的条目（Enabled 为真且内容非空）。
func (l Lorebook) EnabledEntries() []LorebookEntry {
	out := make([]LorebookEntry, 0, len(l.Entries))
	for _, e := range l.Entries {
		if !e.Enabled || strings.TrimSpace(e.Content) == "" {
			continue
		}
		out = append(out, e)
	}
	return out
}

// ---- 剧情节点与分支 ----

// PlotNode 是剧情树中的一个已提交节点。已提交内容不可变（C05）。
type PlotNode struct {
	NodeID        string    `json:"nodeId"`
	SessionID     string    `json:"sessionId"`
	ParentID      string    `json:"parentId"` // root 为 ""
	Kind          NodeKind  `json:"kind"`
	Depth         int       `json:"depth"`
	TurnNumber    int       `json:"turnNumber"` // 非对话事件不增加 turnNumber
	SchemaVersion int       `json:"schemaVersion"`
	CreatedAt     time.Time `json:"createdAt"`
	// Content 承载具体内容：turn 节点为 TurnContent，其他节点为 JSON 表示。
	ContentJSON string `json:"contentJson"`
}

// TurnContent 是一轮玩家输入及其完整回复（在同一节点提交，C01）。
type TurnContent struct {
	InputKind string `json:"inputKind"` // text | option
	InputText string `json:"inputText"`
	// InputNote 是玩家的「注记」：对本次演绎的要求，不是角色的言行。
	// 它与 InputText 分开存，回看时才能分清“当时发生了什么”与“当时要求怎么演”。
	InputNote string        `json:"inputNote,omitempty"`
	OptionRef *OptionRef    `json:"optionRef,omitempty"`
	ActionRef string        `json:"actionRef,omitempty"`
	Blocks    []TextBlock   `json:"blocks"`
	Options   []Option      `json:"options"`
	Mood      *Mood         `json:"mood,omitempty"`
	Checks    []CheckResult `json:"checks,omitempty"`
	// Changes 是本轮生效的状态变化摘要（由领域事件推导，不由模型书写）。
	Changes []StateChange `json:"changes,omitempty"`
	// InjectedMemories 是本次生成实际注入上下文的记忆（文本快照）。
	// 这是“为什么它会这么演”的主要依据：没有它，读者无法判断角色是“不知道”
	// 还是“知道了没用上”。
	InjectedMemories []MemoryRef `json:"injectedMemories,omitempty"`
	// OptionsMode 是本次采用的选项呈现模式（auto/always/never）。
	// 记下来是为了可归因：回看旧回合时应当能分辨“当时本来就没有选项”，
	// 而不是“选项渲染丢了”。
	OptionsMode string `json:"optionsMode,omitempty"`
	// SuppressedOptions 是模型给出了、但按“只在关键节点”规则未呈现的选项条数。
	//
	// 记下它而不是默默丢掉：读者要能分清“这一轮本来没有抉择”与“系统收起了台阶”。
	// 两者对“我现在该做什么”的含义完全不同。
	SuppressedOptions int             `json:"suppressedOptions,omitempty"`
	Provenance        *TurnProvidence `json:"provenance,omitempty"`
}

// TextBlock 是已提交正文的有类型块。
type TextBlock struct {
	Kind      string `json:"kind"` // narration | dialogue | inner_monologue
	SpeakerID string `json:"speakerId,omitempty"`
	Text      string `json:"text"`
}

// Option 是已提交的候选选项。
type Option struct {
	OptionID  string `json:"optionId"`
	Intent    string `json:"intent"` // aggressive | clever | emotional | chaotic
	Text      string `json:"text"`
	ActionRef string `json:"actionRef,omitempty"`
}

// OptionRef 引用生成选项的节点与选项。
type OptionRef struct {
	NodeID   string `json:"nodeId"`
	OptionID string `json:"optionId"`
}

// TurnProvidence 记录生成配置，用于重放与计量。
type TurnProvidence struct {
	TurnID      string `json:"turnId"`
	AttemptID   string `json:"attemptId"`
	Mode        string `json:"mode"` // structured | narrative
	ModelConfig string `json:"modelConfig,omitempty"`
}

// Mood 是提交后驱动的角色情绪。
type Mood struct {
	CharacterID string `json:"characterId"`
	MoodCode    string `json:"moodCode"`
	Text        string `json:"text"`
}

// Branch 是当前写入位置；客户端的 viewNodeId 是查看位置。
type Branch struct {
	BranchID     string `json:"branchId"`
	SessionID    string `json:"sessionId"`
	Name         string `json:"name"`
	HeadNodeID   string `json:"headNodeId"`
	Version      int64  `json:"version"`
	ActiveTurnID string `json:"-"` // runtime-only lock; never exported in story packs
}

// ---- 领域事件 ----

// EventType 是已提交事件的类型集合。
type EventType string

const (
	EventRelationshipDelta EventType = "relationship_delta"
	EventMoodSet           EventType = "mood_set"
	EventGoalSet           EventType = "goal_set"
	EventMemoryAdd         EventType = "memory_add"
	EventMemoryRevised     EventType = "memory_revised"
	EventMemoryPinned      EventType = "memory_pinned"
	EventMemoryHidden      EventType = "memory_hidden"
	EventPromisePropose    EventType = "promise_propose"
	EventPromiseSettle     EventType = "promise_settle"
	EventItemTransfer      EventType = "item_transfer"
	EventItemConsume       EventType = "item_consume"
	EventItemGrant         EventType = "item_grant"
	EventScenePropose      EventType = "scene_propose"
	EventMilestonePropose  EventType = "milestone_propose"
	EventSecretUnlock      EventType = "secret_unlock"
	EventConfiguration     EventType = "configuration_change"
)

// DomainEvent 保存已接受的实际变化，不保存原始提议充当事件（C06）。
type DomainEvent struct {
	EventID        string    `json:"eventId"`
	NodeID         string    `json:"nodeId"`
	EventIndex     int       `json:"eventIndex"`
	Type           EventType `json:"type"`
	PayloadJSON    string    `json:"payloadJson"`
	RulesetVersion string    `json:"rulesetVersion,omitempty"`
}

// ---- 状态快照 ----

// StateSnapshot 是某一节点上的可重建状态投影（只可用于当前节点祖先上的快照）。
type StateSnapshot struct {
	NodeID          string `json:"nodeId"`
	SnapshotVersion int64  `json:"snapshotVersion"`
	RulesetVersion  string `json:"rulesetVersion"`
	StateJSON       string `json:"stateJson"`
	StateHash       string `json:"stateHash"`
}

// ---- 回合请求 ----

// TurnStatus 是回合请求的状态机（技术契约 §3）。
type TurnStatus string

const (
	TurnAccepted             TurnStatus = "accepted"
	TurnQueued               TurnStatus = "queued"
	TurnPreparing            TurnStatus = "preparing"
	TurnGenerating           TurnStatus = "generating"
	TurnValidating           TurnStatus = "validating"
	TurnAwaitingApproval     TurnStatus = "awaiting_approval"
	TurnAwaitingContinuation TurnStatus = "awaiting_continuation"
	TurnCommitted            TurnStatus = "committed"
	TurnCancelled            TurnStatus = "cancelled"
	TurnFailed               TurnStatus = "failed"
	TurnConflicted           TurnStatus = "conflicted"
)

// TurnRequest 是幂等的回合请求。
type TurnRequest struct {
	TurnID          string     `json:"turnId"`
	SessionID       string     `json:"sessionId"`
	BranchID        string     `json:"branchId"`
	IdempotencyKey  string     `json:"idempotencyKey"`
	PayloadHash     string     `json:"payloadHash"`
	ExpectedHeadID  string     `json:"expectedHeadId"`
	ExpectedVersion int64      `json:"expectedVersion"`
	AfterTurnID     string     `json:"afterTurnId,omitempty"`
	Status          TurnStatus `json:"status"`
	ActiveAttemptID string     `json:"activeAttemptId,omitempty"`
	Mode            string     `json:"mode"`
	// RulesetVersion 是本次尝试采用的规则版本（受理时从会话取值并固定）。
	// 与 TurnAttempt.ConfigVersion 同一意图：重放与审计要知道当时按哪套规则结算。
	RulesetVersion string    `json:"rulesetVersion,omitempty"`
	InputJSON      string    `json:"inputJson,omitempty"` // 受理时的输入载荷（kind/text/optionRef）
	ResultNodeID   string    `json:"resultNodeId,omitempty"`
	FailureCode    string    `json:"failureCode,omitempty"`
	FailureMessage string    `json:"failureMessage,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// TurnInput 是回合输入载荷（持久化于 TurnRequest.InputJSON）。
// 选项呈现模式：直接决定"这一轮要不要给玩家选项"。
//
// 默认 auto（只在关键节点给）。选项给得太密会把玩家训练成"点选项"而不是扮演，
// 而扮演才是这类产品的留存来源；但完全去掉又会让卡住的玩家没有台阶。
const (
	// OptionsAuto 只在关键节点（真的需要表态、且选择会改变走向）给选项。
	OptionsAuto = "auto"
	// OptionsAlways 每轮都给，适合习惯先看候选项再决定的玩法。
	OptionsAlways = "always"
	// OptionsNever 从不给。服务端会强制清空选项，是硬保证而非提示词约定。
	OptionsNever = "never"
)

// OptionsModes 返回全部合法选项模式。
func OptionsModes() []string { return []string{OptionsAuto, OptionsAlways, OptionsNever} }

// NormalizeOptionsMode 把外部传入的取值归一到合法集合；未知值一律回到 auto。
func NormalizeOptionsMode(mode string) string {
	switch mode {
	case OptionsAlways, OptionsNever:
		return mode
	default:
		return OptionsAuto
	}
}

type TurnInput struct {
	Kind string `json:"kind"` // text | option
	Text string `json:"text"`
	// Note 是玩家的「注记」：对本次演绎的要求（节奏、侧重、克制程度……），
	// 不是角色的言行，也不会出现在正文里。
	//
	// 单独开一个通道而不是让玩家写进 Text，是因为二者语义完全不同：写进 Text
	// 会被当作角色说过的话，污染剧情且无法与真正的台词区分。
	Note string `json:"note,omitempty"`
	// Options 是本回合的选项呈现模式（auto/always/never，空值等同 auto）。
	// 它是“演绎方式”的指令而不是剧情输入，但必须随请求一起走：模型看不到
	// 客户端的偏好，服务端也不能凭猜。
	Options   string     `json:"options,omitempty"`
	OptionRef *OptionRef `json:"optionRef,omitempty"`
	ActionRef string     `json:"actionRef,omitempty"` // explicit player action, checked by ruleset
}

// TurnAttempt 是一次生成或重生成尝试。
type TurnAttempt struct {
	AttemptID     string    `json:"attemptId"`
	TurnID        string    `json:"turnId"`
	AttemptNo     int       `json:"attemptNo"`
	BaseHeadID    string    `json:"baseHeadId"`
	BaseVersion   int64     `json:"baseVersion"`
	ConfigVersion string    `json:"configVersion,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
}

// DraftFrame 是逐行 JSON 帧协议中已完整解析的一帧。
type DraftFrame struct {
	AttemptID   string `json:"attemptId"`
	FrameSeq    int    `json:"frameSeq"`
	Payload     string `json:"payload"`
	PayloadHash string `json:"payloadHash"`
}

// ---- 动作收据 ----

// ReceiptStatus 是动作收据的状态。
type ReceiptStatus string

const (
	ReceiptPrepared  ReceiptStatus = "prepared"
	ReceiptCommitted ReceiptStatus = "committed"
	ReceiptAbandoned ReceiptStatus = "abandoned"
)

// ActionReceipt 是准备阶段保存的检定/动作结果（C08：客户端和模型不能指定结果）。
type ActionReceipt struct {
	ReceiptID string `json:"receiptId"`
	TurnID    string `json:"turnId"`
	ActionID  string `json:"actionId"`
	// RollID 是行动实例的标识（契约 §5.2）。同一个行动实例可以被多个回合
	// 引用（重生成会为新的 turnId 建独立收据），但结果只掷一次。
	RollID         string        `json:"rollId"`
	BaseHeadID     string        `json:"baseHeadId"`
	RulesetVersion string        `json:"rulesetVersion"`
	ResultJSON     string        `json:"resultJson"`
	Status         ReceiptStatus `json:"status"`
}

// CheckResultOf 解析收据里保存的检定结果。
func (r *ActionReceipt) CheckResultOf() (CheckResult, error) {
	var out CheckResult
	if r == nil || r.ResultJSON == "" {
		return out, errNoCheckResult
	}
	if err := json.Unmarshal([]byte(r.ResultJSON), &out); err != nil {
		return out, err
	}
	if out.RollID == "" {
		out.RollID = r.RollID
	}
	return out, nil
}

// ---- 记忆 ----

// MemoryKind 是记忆记录的类别。
type MemoryKind string

const (
	MemoryObserved MemoryKind = "observed"
	MemoryReported MemoryKind = "reported"
	MemoryInferred MemoryKind = "inferred"
	MemorySecret   MemoryKind = "secret"
)

// MemoryRecord 记录来源与视角；推断不自动升级为世界事实。
//
// 修订采用 copy-on-write：纠正/置顶/隐藏都**不改动原记录**，而是在当前分支上
// 新建一条覆盖记录，用 Supersedes 指向被它取代的记忆。
// 原记录挂在共同祖先上会被多条分支共享，原地修改会让另一分支也看到修订，
// 违反 T17（「A 中隐藏或纠正共同来源记忆 → B 的对应记忆不受影响」）。
type MemoryRecord struct {
	MemoryID     string     `json:"memoryId"`
	SourceNodeID string     `json:"sourceNodeId"`
	Kind         MemoryKind `json:"kind"`
	OwnerIDs     []string   `json:"ownerIds,omitempty"`
	Content      string     `json:"content"`
	EntityIDs    []string   `json:"entityIds,omitempty"`
	Confidence   float64    `json:"confidence,omitempty"`
	// Importance 是 1~10 的重要度，参与检索评分（契约 §8.1 的 importance01）
	// 与 recency 的半衰期分档（低/中/高 = 10/30/90 回合）。0 视为默认 5。
	Importance int  `json:"importance,omitempty"`
	Pinned     bool `json:"pinned"`
	// Supersedes 是这条覆盖记录所取代的记忆 ID（空表示这是原始记录）。
	// 指向方向是「新 → 旧」：覆盖记录挂在新分支上，因此只影响新分支所在的路径。
	Supersedes string          `json:"supersedes,omitempty"`
	Hidden     bool            `json:"hidden"`
	Evidence   *MemoryEvidence `json:"evidence,omitempty"`
	// MergedFrom preserves the sources of an organizer aggregate. Originals are
	// hidden by path-local overlays, never physically deleted.
	MergedFrom []string `json:"mergedFrom,omitempty"`
	SecretID   string   `json:"secretId,omitempty"`
	// SubjectKey 是该记忆所回答的主题问题键（如 npc_liel.status，T3.1）。
	// 同一分支路径上同一有效主题只保留最新值，新事实自动覆盖旧事实。
	SubjectKey string `json:"subjectKey,omitempty"`
	// CreatedTurn 记录该记忆最初产生的来源回合数（T3.2）。
	CreatedTurn int `json:"createdTurn,omitempty"`
	// ValidFromTurn 记录该事实生效的起始回合数（T3.2）。
	ValidFromTurn int `json:"validFromTurn,omitempty"`
	// ValidUntilTurn 记录该事实失效的回合数（0 表示当前仍生效，T3.2）。
	ValidUntilTurn int `json:"validUntilTurn,omitempty"`
}

// NormalizeSubjectKey 规范化主题键（借鉴 Reasonix 知识冲突模型）。
// 小写、点号分隔，移除非法标点（仅允许 [a-z0-9_-]），空片段合并。
// 若处理后无有效字符则返回空串，避免格式错误铸造出意外的独立键。
func NormalizeSubjectKey(s string) string {
	var segments []string
	for _, seg := range strings.Split(strings.ToLower(strings.TrimSpace(s)), ".") {
		var b strings.Builder
		for _, r := range seg {
			switch {
			case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
				b.WriteRune(r)
			case r == ' ', r == '_':
				b.WriteRune('_')
			}
		}
		if cleaned := strings.Trim(b.String(), "_-"); cleaned != "" {
			segments = append(segments, cleaned)
		}
	}
	return strings.Join(segments, ".")
}

// 记忆重要度的取值域（技术契约 §8.1）：importance01 = (importance-1)/9。
const (
	// DefaultMemoryImportance 是未指定时的重要度（中档）。
	// 选 5 而不是 1：默认档位不该让所有未标注的记忆都在重要度项上垫底。
	DefaultMemoryImportance = 5
	MinMemoryImportance     = 1
	MaxMemoryImportance     = 10
)

// ClampMemoryImportance 把重要度收敛到合法区间（0 视为默认值）。
func ClampMemoryImportance(v int) int {
	if v <= 0 {
		return DefaultMemoryImportance
	}
	if v < MinMemoryImportance {
		return MinMemoryImportance
	}
	if v > MaxMemoryImportance {
		return MaxMemoryImportance
	}
	return v
}

// ApplyMemoryOverlays 计算给定记录集合在**当前路径**上的有效记忆。
//
// 规则：
//  1. Supersedes 链式覆盖：一条记录若被集合内另一条可见记录 Supersedes 指向，则不再生效。
//  2. SubjectKey 唯一值约束（T3.1）：同一路径上若存在多个相同的非空 SubjectKey，
//     自动仅保留最新生效的一条（比较 ValidFromTurn -> CreatedTurn -> 降序 MemoryID），其余视为被取代。
//
// 输入必须是已经按路径过滤过的集合（例如祖先链范围内的记忆），
// 否则会把别的分支上的覆盖记录算进来。
func ApplyMemoryOverlays(records []*MemoryRecord) []*MemoryRecord {
	replaced := make(map[string]bool, len(records))
	for _, r := range records {
		if r.Supersedes != "" {
			replaced[r.Supersedes] = true
		}
	}

	// SubjectKey 约束：同 SubjectKey 仅保留最新生效的一条记录
	subjectLatest := make(map[string]*MemoryRecord)
	for _, r := range records {
		if replaced[r.MemoryID] || r.SubjectKey == "" {
			continue
		}
		key := NormalizeSubjectKey(r.SubjectKey)
		if key == "" {
			continue
		}
		if existing, ok := subjectLatest[key]; ok {
			if r.ValidFromTurn > existing.ValidFromTurn ||
				(r.ValidFromTurn == existing.ValidFromTurn && r.CreatedTurn > existing.CreatedTurn) ||
				(r.ValidFromTurn == existing.ValidFromTurn && r.CreatedTurn == existing.CreatedTurn && r.MemoryID > existing.MemoryID) {
				replaced[existing.MemoryID] = true
				subjectLatest[key] = r
			} else {
				replaced[r.MemoryID] = true
			}
		} else {
			subjectLatest[key] = r
		}
	}

	out := make([]*MemoryRecord, 0, len(records))
	for _, r := range records {
		if replaced[r.MemoryID] {
			continue
		}
		out = append(out, r)
	}
	return out
}

// ---- 摘要 ----

// SummaryArtifact 是覆盖历史区间的派生材料（技术契约 §9.2）。
//
// 它只是**叙述性回顾**：结构化资产、承诺、线索与秘密一律来自状态投影，
// 不能从摘要正文反向恢复（T23）。采用条件见 SummariesOnPath。
type SummaryArtifact struct {
	SummaryID       string `json:"summaryId"`
	FromNodeID      string `json:"fromNodeId"`
	ToNodeID        string `json:"toNodeId"`
	SourceHash      string `json:"sourceHash"`
	VisibilityScope string `json:"visibilityScope"`
	Text            string `json:"text"`
	// ModelConfigVersion 记录生成摘要用的模型配置：摘要质量随模型变化，
	// 不记录就无法解释"为什么这段摘要比那段差"。
	ModelConfigVersion string `json:"modelConfigVersion,omitempty"`
	// SummaryVersion 是摘要自身的版本（同一区间可用新模型重新生成）。
	SummaryVersion int `json:"summaryVersion,omitempty"`
}

// ---- 书签 ----

// Bookmark 是展示元数据，不影响剧情因果。
type Bookmark struct {
	BookmarkID string `json:"bookmarkId"`
	SessionID  string `json:"sessionId"`
	NodeID     string `json:"nodeId"`
	Title      string `json:"title"`
}

// ---- 出站事件 ----

// OutboxEvent 在业务事务内写入，重复投递由消费者去重。
type OutboxEvent struct {
	EventID     string    `json:"eventId"`
	AggregateID string    `json:"aggregateId"`
	Sequence    int64     `json:"sequence"`
	Type        string    `json:"type"`
	PayloadJSON string    `json:"payloadJson"`
	CreatedAt   time.Time `json:"createdAt"`
}
