// Package ports 定义存储、模型、时钟等端口接口。
// 数据库、网络与 GUI 不得成为领域包依赖；应用层编排这些端口（技术契约 §1）。
package ports

import (
	"context"
	"errors"
	"math/rand"
	"time"

	"tavernagent/internal/domain"
)

// ErrTruncatedStream 是供应商明确截断信号（finish_reason=length 等）。
// 应用层据此进入 awaiting_continuation，而非视为失败。
var ErrTruncatedStream = errors.New("stream truncated by provider")

// ErrNonStreamingResponse 表示供应商返回 2xx，但正文里没有任何流式事件
// （网关返回 HTML 错误页、JSON 错误体或空响应都属于这一类）。
//
// 它必须与 ErrTruncatedStream 分开：截断意味着"模型写了一半"，可以拿已发布的
// 草稿帧续写；而非流式响应连一个帧都没有，续写无从谈起，只能归类为供应商不可用。
// 此前二者混用，网关 200 错误页会被当成截断，用户看到一个空草稿的续写态。
var ErrNonStreamingResponse = errors.New("provider returned no streaming events")

// ErrNotFound 表示目标对象不存在。
// 端口级语义：应用层据此返回 404，不必依赖具体适配器的错误类型。
var ErrNotFound = errors.New("not found")

// ErrNoState 表示该节点没有可用的状态投影（祖先链上找不到任何快照）。
//
// 与 ErrNotFound 区分：节点本身存在，只是没有状态需要投影——例如外部导入的
// 稀疏数据。这种"无状态"属于正常降级场景，调用方应按全新状态继续，
// 而不是当成故障报错。
var ErrNoState = errors.New("no state projection")

// ErrSessionBusy 表示会话仍有进行中的回合（受理/排队/生成/校验中），此刻删除
// 会与在途写入竞争，因此拒绝；应用层据此返回 409。
var ErrSessionBusy = errors.New("session has an in-flight turn")

// ErrCardLibraryFull 表示角色卡库已达上限。它是有意的硬上限而非静默淘汰：
// 卡片是用户资产，宁可显式拒绝并让用户自己删，也不要像浏览器缓存那样悄悄丢卡。
var ErrCardLibraryFull = errors.New("character card library is full")

// CommitPlan 是提交前的完整计划：校验通过后在一个短事务内写入（C03）。
type CommitPlan struct {
	AttemptID       string // optional for direct edits; fences a superseded model attempt
	TurnID          string
	ExpectedHeadID  string
	ExpectedVersion int64
	BaseStateHash   string
	RulesetVersion  string
	Node            *domain.PlotNode
	Events          []*domain.DomainEvent
	// NewStateHash 是提交后状态投影的校验值。
	NewStateHash string
	// NewStateJSON 是提交后状态投影的内容（权威快照）。
	NewStateJSON string
	// SettleReceipts 是本次结算的动作收据（status → committed）。
	SettleReceipts []string
	// Memories 是本轮产生的记忆记录，与节点/事件在同一事务内落库。
	// SourceNodeID 由提交方在生成节点 ID 后补齐。
	Memories []*domain.MemoryRecord
	// MentionedMemoryIDs 是本次生成注入了上下文的记忆 ID（契约 §8.1）。
	// 提交方会复核「提交的正文是否确实提到它」，只有确有依据的才落
	// memory_mentions —— 注入不等于提及。
	MentionedMemoryIDs []string
}

// CommitResult 是提交事务的返回。
type CommitResult struct {
	Committed    bool
	AlreadyDone  bool
	NewHeadID    string
	NewVersion   int64
	ConflictCode string // HEAD_CONFLICT / ""
}

// ---- 存储端口（按消费者拆分）----
//
// 拆分的意义：让每个消费者在签名里表达它**真正需要什么**，而不是随手拿到
// 整个数据库。Store 仍组合全部能力，适配器一次实现即可——拆的是"读得懂的
// 边界"，不是"多份实现"。

// LorebookMatch identifies an entry by its position within a versioned template.
// Entry IDs are optional and are not globally unique across different books.
type LorebookMatch struct {
	BookIndex  int                  `json:"bookIndex"`
	EntryIndex int                  `json:"entryIndex"`
	BookName   string               `json:"bookName"`
	Entry      domain.LorebookEntry `json:"entry"`
}

// SessionStore 是会话与模板的读写。
type SessionStore interface {
	CreateSession(sess *domain.Session, rootNode *domain.PlotNode, branch *domain.Branch, templates []*domain.TemplateVersion) error
	// CreateSessionWithSnapshot commits the complete playable session atomically.
	CreateSessionWithSnapshot(sess *domain.Session, rootNode *domain.PlotNode, branch *domain.Branch, templates []*domain.TemplateVersion, snapshot *domain.StateSnapshot) error
	GetSession(sessionID string) (*domain.Session, error)
	ListSessions() ([]*domain.Session, error)
	GetTemplateVersion(id string) (*domain.TemplateVersion, error)
	// SearchLorebook reads only the immutable template selected by the session.
	// Empty text lists entries in declaration order; offset/limit bound the page.
	SearchLorebook(templateVersionID, text string, offset, limit int) ([]LorebookMatch, error)
	// SetSessionRuleset 更新会话采用的规则版本。
	// 这是规则升级的显式入口（契约 §7 要求升级通过显式迁移/配置事件进行，
	// 而不是让新旧数据混在同一个编译期常量里）。已受理的回合不受影响——
	// 它们保留自己受理时固定的版本。
	SetSessionRuleset(sessionID, version string) error
	// DeleteSession 永久删除会话及其全部故事数据（节点、分支、回合、事件、快照、
	// 记忆、导演草稿与投影等）。跨会话共享的模板（template_versions 按内容寻址）
	// 不属于任何单个会话，因此保留。
	//
	// 仍有未完成回合时返回 ErrSessionBusy，会话不存在时返回 ErrNotFound。
	// 删除不可撤销，调用方负责与用户确认。
	DeleteSession(sessionID string) error
}

// CardStore 是角色卡库的持久化（用户资产）。
//
// 卡库与会话模板是两套语义：卡库行可增删改，模板行被会话引用且不可变。
// 因此「从卡库移除」只作用于本表，不触碰任何 template_versions——
// 已有故事永远能继续，这也是前端删除确认文案所承诺的行为。
//
// 卡库条目按 card_id（角色卡稳定标识）唯一，重复导入同一张卡是覆盖而非新增。
type CardStore interface {
	// ListCards 返回全部卡（按最近更新倒序），CharacterJSON 会被置空。
	ListCards() ([]*domain.CharacterCardEntry, error)
	// GetCard 返回单张卡的完整内容；不存在时返回 ErrNotFound。
	GetCard(cardID string) (*domain.CharacterCardEntry, error)
	// SaveCard 以 card_id 为准 upsert：已存在则更新元数据与内容、保留创建时间。
	// 卡库已满且是新卡时返回 ErrCardLibraryFull。
	SaveCard(card *domain.CharacterCardEntry) error
	// DeleteCard 移除一张卡；不存在时返回 ErrNotFound（调用方按幂等处理）。
	DeleteCard(cardID string) error
	// TouchCard 记录最近一次用于创建会话的时间；不存在时返回 ErrNotFound。
	TouchCard(cardID string, at time.Time) error
}

// StoryStore 是剧情树、分支指针与状态投影的读取。
type StoryStore interface {
	InsertNode(node *domain.PlotNode) error
	GetNode(nodeID string) (*domain.PlotNode, error)
	ListChildren(sessionID, parentID string) ([]*domain.PlotNode, error)
	// RecentTurnNodes 返回某节点之上最近的若干个回合节点（按深度正序）。
	// 用于构造正文历史：只取需要的窗口，不把整条祖先链搬进内存。
	// limit <= 0 时返回空。
	RecentTurnNodes(nodeID string, limit int) ([]*domain.PlotNode, error)
	// ListChildrenOf 批量返回若干父节点各自的子节点（按父节点分组）。
	ListChildrenOf(sessionID string, parentIDs []string) (map[string][]*domain.PlotNode, error)
	ChildWindow(sessionID string, parentIDs []string, limit int, content bool) (map[string][]*domain.PlotNode, bool, error)
	ChildCounts(sessionID string, parentIDs []string) (map[string]int, error)
	AncestorChain(nodeID string, fromRoot bool) ([]*domain.PlotNode, error)
	IsAncestor(ancestorID, nodeID string) (bool, error)
	GetEvents(nodeID string) ([]*domain.DomainEvent, error)

	ListBranches(sessionID string) ([]*domain.Branch, error)
	GetBranch(branchID string) (*domain.Branch, error)
	CreateBranch(b *domain.Branch) error

	// GetSnapshot 读取某节点**直接落盘**的快照；稀疏检查点下大多节点没有。
	GetSnapshot(nodeID string) (*domain.StateSnapshot, error)
	SaveSnapshot(s *domain.StateSnapshot) error
	// StateAt 返回某节点的状态投影（技术契约 §7）：
	// 祖先路径上最近的兼容快照 + 快照之后到该节点的有序事件。
	// 这是读取"某节点状态"的唯一正确入口——不要假设节点自带快照。
	StateAt(nodeID string) (*domain.StateSnapshot, error)
	DirectorAt(nodeID string) (*domain.DirectorState, error)
}

// ReceiptAtNode 是编译层读取收据所需的最小视图：收据本身 + 其所属结果节点的
// ID 与回合序号（回合序号用于账本 "Turn #N" 标注）。
type ReceiptAtNode struct {
	NodeID     string
	TurnNumber int
	Receipt    *domain.ActionReceipt
}

// EventAtNode 是编译层读取路径事件所需的视图：事件本身 + 所属节点与回合序号。
type EventAtNode struct {
	NodeID     string
	TurnNumber int
	Event      *domain.DomainEvent
}

// TurnStore 是回合生命周期：受理、尝试、草稿帧、分支锁与提交事务。
type TurnStore interface {
	TransitionTurn(change TurnTransition) (*domain.TurnRequest, bool, error)
	InterruptedTurns() ([]*domain.TurnRequest, error)
	CreateTurnRequest(req *domain.TurnRequest) error
	GetTurn(turnID string) (*domain.TurnRequest, error)
	// FindTurnByIdempotency 返回既有请求或 nil。
	FindTurnByIdempotency(sessionID, key string) (*domain.TurnRequest, error)
	UpdateTurnResult(turnID string, status domain.TurnStatus, resultNodeID, failureCode, failureMessage string) error
	SetCancelIntent(turnID string, cancel bool) error
	HasCancelIntent(turnID string) (bool, error)
	CreateAttempt(a *domain.TurnAttempt) error
	ListAttempts(turnID string) ([]*domain.TurnAttempt, error)
	// ResumeTurn atomically claims an awaiting continuation exactly once.
	ResumeTurn(turnID string) (bool, error)
	AppendDraftFrame(attemptID string, seq int, payload, payloadHash string) error
	GetDraftFrames(attemptID string) ([]*domain.DraftFrame, error)
	SaveReceipt(r *domain.ActionReceipt) error
	// PrepareReceipt checks for an existing result under a write transaction.
	// draw is called only when that action instance has never been rolled.
	PrepareReceipt(ctx context.Context, receipt *domain.ActionReceipt, draw func() (string, error)) (*domain.ActionReceipt, error)
	GetReceipts(turnID string) ([]*domain.ActionReceipt, error)
	// ReceiptsAtResultNodes 按结果剧情节点 ID 批量取已结算收据，供上下文编译
	// 组装历史大检定账本（M4h）。通过 turn_requests.result_node_id 反查，
	// 避免编译层自行维护 节点→回合 映射。
	ReceiptsAtResultNodes(resultNodeIDs []string) ([]ReceiptAtNode, error)
	ReceiptsOnPath(nodeID string) ([]ReceiptAtNode, error)
	EventsOnPath(nodeID string, eventTypes ...domain.EventType) ([]EventAtNode, error)
	// FindReceiptByRoll 按行动实例（rollId + 基准父节点）查找既有收据。
	//
	// 它支撑契约 §5.2 的"文字重生成复用原检定结果"：rollId 是
	// (父节点, 动作, 规则版本) 的确定性函数，因此同一个行动实例在同一
	// 基准上再次被请求时，查得到就复用，不再掷骰。
	FindReceiptByRoll(rollID, baseHeadID string) (*domain.ActionReceipt, error)
	// SettleReceipts 把给定收据置为 committed。只改状态、不改结果——
	// 结果在准备阶段就已定下，结算只是让它生效（并防止同一实例重复生效）。
	SettleReceipts(receiptIDs []string) error
	// ClaimActiveTurn 尝试把 active_turn_id 原子设为 turnID；返回是否成功。
	ClaimActiveTurn(branchID, turnID string) (bool, error)
	// ReleaseActiveTurn 仅在 active_turn_id==turnID 时清空（失败/取消/放弃时解除分支锁）。
	ReleaseActiveTurn(branchID, turnID string) error
	// CommitTurn 在一个短事务内执行提交 8 步，返回冲突或已提交信息。
	CommitTurn(plan *CommitPlan) (*CommitResult, error)
}

// MemoryRecallQuery 是可见候选的检索参数（技术契约 §8.1）。
type MemoryRecallQuery struct {
	// MatchExpr 是 FTS5 的 MATCH 表达式（由 internal/search 构造并转义）。
	// 空表示不做词法检索：此时池只由置顶/隐藏/覆盖记录构成。
	// 单字与无词元查询会得到空表达式——契约 §8.1 要求这类查询走实体兜底，
	// 而不是把单字塞进索引（单字命中几乎所有文本，噪声大于收益）。
	MatchExpr string
	RawText   string
	OwnerIDs  []string
	SecretIDs []string
	EntityIDs []string
	// Limit 是词法命中的上限（<=0 取适配器默认值）。
	Limit int
	// FallbackLimit 保底候选条数（T2.2）。>0 时增加重要性保底路，<=0 时不启用。
	FallbackLimit int
}

// MemoryCandidate 是可见候选及其词法排名。
type MemoryCandidate struct {
	Memory *domain.MemoryRecord
	// SourceTurn 是产生这条记忆的回合序号。它同时是 recency 的初始值：
	// 产生记忆的那次提交本身就在正文里写了这条内容，构成一次明确提及依据。
	SourceTurn int
	// LastMentionTurn 是这条记忆在当前路径上最近一次被明确提及的回合
	// （契约 §8.1 的 lastMeaningfulMentionTurn）；0 表示没有提及记录。
	// 它随候选一起返回而不是单独查一次：提及表要与**同一个**祖先集合连接，
	// 分两次查询就要多走一遍 O(链深) 的递归遍历（实测约 13ms）。
	LastMentionTurn int
	// Rank 是 BM25（**越小越相关**），HasRank=false 表示无词法命中。
	// 契约要求未经归一化的 BM25 不得直接与 1~10 的重要度相加。
	Rank    float64
	HasRank bool
}

// MemoryStore 是来源化认知记录（M3）。
type MemoryStore interface {
	ProjectedMemories(nodeID string) ([]*domain.MemoryRecord, error)
	// 记忆挂在 source_node_id 上，因此「当前路径可见性」由祖先链决定（分支隔离）。
	// 模型产出的记忆走 CommitTurn 事务（见 CommitPlan.Memories），不做独立写入，
	// 避免记忆与产生它的节点不在同一事务里。
	ListMemories(sessionID string) ([]*domain.MemoryRecord, error)
	ActiveMemoryCount(nodeID string) (int, error)
	// MemoriesInChain 返回挂在给定节点集合上的记忆。传当前节点的祖先链即可
	// 得到该路径可见的记忆；分支之外的记忆不会出现（T16/T17）。
	// 返回的是原始记录（含被覆盖项），覆盖关系由 domain.ApplyMemoryOverlays 解析。
	MemoriesInChain(nodeIDs []string) ([]*domain.MemoryRecord, error)
	// MemoriesOnPath 返回挂在「从根到该节点」路径上的全部记忆（**不设上限**）。
	// 可见性过滤在 SQL 侧完成，调用方不必先取整条祖先链再筛。
	//
	// 它是词法索引不可用时的兜底候选来源：此时没有排名可下推，只能把这条
	// 路径上的记忆全取回来在 Go 侧算重合度。索引可用时请用 RecallMemories
	// ——后者的候选池由 FTS5 排名限定，代价与记忆总数无关。
	// 返回的候选一律 HasRank=false（本路径不产出词法排名）。
	MemoriesOnPath(nodeID string) ([]*MemoryCandidate, error)
	// RecallMemories 返回**可见候选池**：在当前路径可见的记忆里，
	// 取词法命中按 BM25 排序的前 limit 条，并并入置顶、隐藏与覆盖记录。
	//
	// 为什么把「并集」放在 SQL 侧：cover（覆盖记录）必须出现在池里，
	// 否则 Go 侧的 copy-on-write 解析不知道原记录已被取代，会把被纠正的
	// 旧记忆继续召回。隐藏记录也必须进池——它是「原记录被隐藏」的载体。
	RecallMemories(nodeID string, q MemoryRecallQuery) ([]*MemoryCandidate, error)

	// LexicalIndexAvailable 报告词法索引（FTS5）是否可用。
	// 不可用时检索退回 Go 侧双字重合（仍是纯词法，不走模糊语义猜测）。
	LexicalIndexAvailable() bool

	// CreateMemory 写入单条记忆记录，仅供用户发起的覆盖记录使用
	// （纠正/置顶/隐藏）。这些操作不与回合绑定，但必须挂在当前分支头节点上，
	// 才能只影响该路径（T17）。模型产出的记忆仍然只能走 CommitTurn。
	CreateMemory(m *domain.MemoryRecord) error
	// UpdateMemory 更新既有记忆记录。注意：修订不走这里，
	// 修订是 copy-on-write（新建覆盖记录），原地更新会泄漏到其它分支。
	// 本方法保留给不涉及路径语义的字段维护（如迁移/管理脚本）。
	UpdateMemory(record *domain.MemoryRecord) error
}

// ArchiveStore 是剧情包的导出与导入（M3 · T21/T22）。
type ArchiveStore interface {
	// ExportSession 以一致读取出会话的完整可携带快照。
	// branchID 非空时只导出该分支（仍包含它的全部必要祖先与相关模板）。
	ExportSession(sessionID, branchID string) (*domain.SessionBundle, error)
	// ImportSession 在一个事务内写入整包，并一致重映射全部 ID（含
	// contentJson 里对被引用节点的引用），返回新会话。
	// 模板按 contentHash 复用既有版本，避免主键冲突。
	ImportSession(bundle *domain.SessionBundle) (*domain.Session, error)
}

// OutboxStore 是出站事件。
type OutboxStore interface {
	AppendOutbox(events []*domain.OutboxEvent) error
	PollOutbox(aggregateID string, afterSequence int64, limit int) ([]*domain.OutboxEvent, error)
}

// OpsStore 是运维与自检。
type OpsStore interface {
	// Checkpoint 把 WAL 内容合并回主库。
	// 性能观测前调用它可以让起点可复现：WAL 越大，读路径的检查成本越高，
	// 同一份数据集在不同 WAL 状态下测出的延迟会明显不同。
	Checkpoint() error
	SystemInfo() (SystemInfo, error)
	Close() error
}

// ---- 按服务组合的依赖视图 ----
// 这些组合接口不是新实现，只是把"某个服务需要哪几组能力"写成可读的类型。

// SessionDeps 是会话用例需要的存储能力。
//
// 包含 CardStore 是因为建会话现在可以直接引用卡库中的卡（cardId）：
// 前端不必再回传兆级 characterJson，卡库与模板在此收敛到同一条读路径。
type SessionDeps interface {
	SessionStore
	StoryStore
	SummaryStore
	CardStore
}

// TurnDeps 是回合编排（受理 → 生成 → 校验 → 提交）需要的存储能力。
type TurnDeps interface {
	SessionStore
	StoryStore
	TurnStore
	MemoryStore
}

// SummaryStore 是历史区间摘要（M4b）的存取。
type SummaryStore interface {
	// SaveSummary 写入一条摘要产物。同一区间允许存在多条（不同
	// SummaryVersion / 模型配置），读取时取版本最高的。
	SaveSummary(a *domain.SummaryArtifact) error
	// SummariesOnPath 返回来源区间完全落在「根到该节点」路径上的摘要。
	// 采用条件（契约 §9.2）在此下推到 SQL：跨分支的摘要其 to_node 不在
	// 当前路径上，自然被排除——这正是 T24 要的行为。
	SummariesOnPath(nodeID string) ([]*domain.SummaryArtifact, error)
}

// CompileDeps 是上下文编译需要的存储能力。
type CompileDeps interface {
	SessionStore
	StoryStore
	MemoryStore
	SummaryStore
	TurnStoreCaptive
}

// TurnStoreCaptive 是编译链路需要的回合存储能力子集：读取判定收据
// 以编译历史大检定账本（M4h），而不暴露回合写入等副作用。
type TurnStoreCaptive interface {
	ReceiptsAtResultNodes(resultNodeIDs []string) ([]ReceiptAtNode, error)
	ReceiptsOnPath(nodeID string) ([]ReceiptAtNode, error)
	EventsOnPath(nodeID string, eventTypes ...domain.EventType) ([]EventAtNode, error)
}

// MemoryDeps 是记忆用例需要的存储能力（分支与节点用于校验归属与解析覆盖）。
type MemoryDeps interface {
	SessionStore
	StoryStore
	MemoryStore
	MemoryBatchStore
}

// MemoryBatch appends a non-dialogue node. It never changes an existing node,
// snapshot or shared memory. CAS and the active-turn lock guard late workers.
type MemoryBatch struct {
	BatchID, SessionID, BranchID, ExpectedHeadID, SourceTurnID, PayloadHash, Reason string
	ExpectedVersion                                                                 int64
	NodeID                                                                          string
	Memories                                                                        []*domain.MemoryRecord
	Events                                                                          []*domain.DomainEvent
}

type MemoryBatchStore interface {
	CommitMemoryBatch(ctx context.Context, batch *MemoryBatch) (*CommitResult, error)
	FindMemoryBatch(batchID, sessionID, branchID, payloadHash string) (*CommitResult, error)
	FindCognitiveBatch(branchID, sourceTurnID string) (*CommitResult, error)
}

// Store 是权威持久层的完整端口（适配器实现它，即自动满足上面所有小接口）。
type Store interface {
	DirectorStore
	SessionStore
	StoryStore
	TurnStore
	MemoryStore
	ArchiveStore
	OutboxStore
	OpsStore
	SummaryStore
	MemoryBatchStore
	UsageStore
	CardStore
}

// SystemInfo 是 SQLite 运行时探测结果（FTS5 能力等）。
type SystemInfo struct {
	DriverVersion  string
	FTS5Available  bool
	AttributeError string
}

// ---- Model provider ----

// ProviderCapabilities 描述供应商能力（技术契约 §10）。
type ProviderCapabilities struct {
	ID               string
	Streaming        bool
	StructuredOutput bool
	Continuation     bool
	ContextWindow    int // 0 表示未知，使用保守上限
	TokenizerKnown   bool
}

// ChatMessage 是供应商请求中的消息。
type ChatMessage struct {
	Role    string // system | user | assistant
	Content string
}

// ChatRequest 是模型生成请求。
type ChatRequest struct {
	Task      string `json:"-"`
	TurnID    string `json:"-"`
	AttemptID string `json:"-"`
	Model     string
	MaxTokens int
	Messages  []ChatMessage
	// InjectedMemoryIDs 是本次编译实际注入上下文的记忆 ID。
	//
	// 它随请求向上回传，供提交时判定 lastMeaningfulMentionTurn（契约 §8.1）：
	// 只有「被注入」且「提交的正文确实提到」的记忆才算一次有意义提及。
	// 仅被召回（但正文没提）不计——契约明确要求召回动作本身不更新。
	InjectedMemoryIDs []string `json:"-"`
	// InjectedMemories 是同一批记忆的展示快照（ID + 当时的文本）。
	//
	// 与 ID 清单同源同批，但用途不同：ID 清单用于提交时核验“正文有没有真提到它”，
	// 快照则落进节点内容，供读者回看“这次演绎当时参考了哪几句话”。
	// 两者必须一起产生，否则展示与判定就会各说各话。
	InjectedMemories     []domain.MemoryRef `json:"-"`
	InputBudget          int                `json:"-"`
	EstimatedInputTokens int                `json:"-"`
	NeedsCompaction      bool               `json:"-"`
	ProtectedTurns       int                `json:"-"`
}

// TokenUsage 是模型调用的真实计量（来自供应商 usage 对象）。
//
// 与 EstimatedInputTokens 的区别：后者是编译侧按字符启发式**估算**的输入 token，
// 前者是供应商**实际结算**的数字。两者同时落库（turn_usage），用于校准估算系数——
// 没有真实读数时，压缩阈值、预算裁剪与缓存优化都无法被验证是否有效。
type TokenUsage struct {
	// Prompt 是输入 token。供应商若区分缓存，则为**未命中**部分的输入 token。
	Prompt int
	// Completion 是输出 token。
	Completion int
	// Cached 是命中前缀缓存的输入 token。供应商未提供时记 0（不可臆测）。
	// 这是判断提示词前缀是否稳定的唯一直接证据（对应 SplitDynamicContext 优化）。
	Cached int
	// Reported 为 false 表示供应商本次未回传 usage，字段全为 0。
	// 调用方不得把 0 解释为"真实消耗为 0"。
	Reported bool
}

// TurnUsageRecord 是一次模型调用的用量落库记录（schemaV11 / turn_usage）。
//
// Prompt/Completion/Cached 是供应商回传的真实值；Estimated 是编译侧估算的输入
// token。两个都存，才能用真实值校准估算系数——只存其一无法发现估算漂移。
type TurnUsageRecord struct {
	TurnID            string
	AttemptID         string
	Model             string
	Provider          string
	Prompt            int
	Completion        int
	Cached            int
	Estimated         int
	Reported          bool
	CreatedAt         string // RFC3339（UTC）
	Task              string
	Slot              string
	ConfigFingerprint string
	LatencyMS         int64
	FirstTokenMS      int64
	Outcome           string
}

// UsageTotals 是累计用量，供观测端点使用。
type UsageTotals struct {
	Calls          int
	Prompt         int
	Completion     int
	Cached         int
	Estimated      int
	ReportedCalls  int
	LastRecordedAt string
}

// UsageStore 是模型用量的存取端口。
type UsageStore interface {
	// RecordTurnUsage 记录一次调用的用量。同一 attempt 重复调用以最后一次为准。
	RecordTurnUsage(rec TurnUsageRecord) error
	// RecentTurnUsage 返回最近 limit 条记录（按创建时间倒序；
	// 时间戳相同时按写入先后倒序，保证调用方拿到的 [0] 就是最近一条）。
	RecentTurnUsage(limit int) ([]TurnUsageRecord, error)
	// TurnUsageTotals 返回累计用量。
	TurnUsageTotals() (UsageTotals, error)
}

// StreamSink 承载一次流式生成的回调。
//
// OnChunk 交付模型输出的原始帧字节（适配器不做内容再加工，语义与改动前一致）。
// OnUsage 在供应商回传 usage 时调用：可为 nil；可能被调用多次（增量上报），
// 调用方应取最后一次的累积值。
type StreamSink struct {
	OnChunk func([]byte) error
	OnUsage func(TokenUsage)
}

// Chunk 安全调用 OnChunk。
func (s StreamSink) Chunk(b []byte) error {
	if s.OnChunk == nil {
		return nil
	}
	return s.OnChunk(b)
}

// Usage 安全调用 OnUsage。
func (s StreamSink) Usage(u TokenUsage) {
	if s.OnUsage == nil {
		return
	}
	s.OnUsage(u)
}

// OnChunkSink 把裸回调包装成 StreamSink，便于旧调用点迁移。
func OnChunkSink(onChunk func([]byte) error) StreamSink {
	return StreamSink{OnChunk: onChunk}
}

// Random 是随机源（技术契约 §1 列出的端口之一）。
//
// 注入而不是直接用全局随机：检定结果必须可确定性重放与测试，
// 而且"重生成不重掷"这条契约要求需要一个能被替换的随机源来验证。
type Random interface {
	// Intn 返回 [0, n) 的整数，与 math/rand 语义一致。
	Intn(n int) int
}

// RealRandom 是默认的真实随机源。
type RealRandom struct{}

// Intn 使用全局随机源（已自动播种）。
func (RealRandom) Intn(n int) int { return rand.Intn(n) }

// ModelProvider 抽象模型供应商。Stream 交付原始帧协议字节流。
//
// sink 同时承载内容增量与 usage 回传：把用量做成必选参数而不是可选接口，
// 是为了让"忘记采集"在编译期就失败，而不是上线后才发现没有读数。
type ModelProvider interface {
	Capabilities(ctx context.Context) (ProviderCapabilities, error)
	Stream(ctx context.Context, req ChatRequest, sink StreamSink) error
}

// ProviderFactory 按名字返回已配置的供应商。
type ProviderFactory func(name string) (ModelProvider, error)

// ---- 时钟 ----

// Clock 抽象时间以便测试与确定性。
type Clock interface {
	Now() time.Time
}

// RealClock 是生产时钟。
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

type thinkingCallbackKey struct{}

// WithThinkingCallback 向 Context 注入思考过程回调（推理模型如 DeepSeek-R1 / o1 / o3 专用）。
func WithThinkingCallback(ctx context.Context, fn func(text string)) context.Context {
	return context.WithValue(ctx, thinkingCallbackKey{}, fn)
}

// ThinkingCallbackFromContext 从 Context 中获取思考过程回调。若未注入则返回 nil。
func ThinkingCallbackFromContext(ctx context.Context) func(text string) {
	if fn, ok := ctx.Value(thinkingCallbackKey{}).(func(string)); ok {
		return fn
	}
	return nil
}
