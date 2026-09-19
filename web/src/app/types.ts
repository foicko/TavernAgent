// API 类型镜像后端契约（camelCase；与 internal/domain 一致）。
export interface TextBlock {
  kind: "narration" | "dialogue" | "inner_monologue";
  speakerId?: string | null;
  text: string;
}

export interface Option {
  optionId: string;
  intent: "aggressive" | "clever" | "emotional" | "chaotic";
  text: string;
  actionRef?: string;
}

export interface ActionView { actionId: string; label: string; attribute: string; dc: number; itemIds?: string[] }
export interface CheckResult {
  rollId: string; actionId: string; attribute: string; attributeModifier: number;
  natural: number; total: number; dc: number;
  outcome: "success" | "failure" | "critical_success" | "critical_failure";
  permanentEffect?: string;
}

export interface TurnContent {
  inputKind: string;
  inputText: string;
  /** 玩家注记（非叙事）：只约束这一次演绎，不会出现在正文里。 */
  inputNote?: string;
  /** 本回合采用的选项呈现模式（auto/always/never）。 */
  optionsMode?: string;
  /** 模型给出但按“只在关键节点”规则未呈现的选项条数。 */
  suppressedOptions?: number;
  /** 本轮生效的状态变化摘要（由服务端从领域事件推导，不由模型书写）。 */
  changes?: StateChange[];
  /** 本轮实际注入上下文的记忆（当时的文本快照）。 */
  injectedMemories?: MemoryRef[];
  // 后端在 contentJson 里附带的溯源信息（回合 ID），历史卡片据此关联思考过程。
  provenance?: { turnId?: string };
	checks?: CheckResult[];
  blocks: TextBlock[];
  options: Option[];
  mood?: { characterId: string; moodCode: string; text: string } | null;
}

/**
 * 一条已生效的世界变化。由服务端从领域事件推导，不是模型的叙述。
 * delta 有方向时（关系、物品数量）前端据此上色。
 */
export interface StateChange {
  kind: "relationship" | "mood" | "goal" | "promise" | "item" | "scene" | "milestone" | "secret";
  label: string;
  text: string;
  delta?: number;
}

/**
 * 本轮参考的一条记忆。text 是**当时的文本快照**：记忆会被修订，
 * 只存 ID 的话回看旧回合看到的就不是当时真正影响演绎的那句话了。
 */
export interface MemoryRef {
  memoryId: string;
  text: string;
  kind?: string;
}

export interface PlotNode {
  nodeId: string;
  sessionId: string;
  parentId: string;
  kind: string;
  depth: number;
  turnNumber: number;
  contentJson: string;
  createdAt: string;
}

export interface BranchView {
  branchId: string;
  name: string;
  headNodeId: string;
  version: number;
	activeTurnId?: string;
}

export interface RelationValue {
  affection: number;
  trust: number;
  alertness: number;
}

export interface ItemInstance {
  instanceId: string;
  templateId?: string;
  name: string;
  ownerId?: string;
  quantity: number;
  protection?: number;
  keepsake?: boolean;
  unique?: boolean;
}

export interface CharacterInfo {
  characterId: string;
  name: string;
  description?: string;
  participant?: boolean;
  avatar?: string;
  personality?: string;
  scenario?: string;
  // 以下字段沿用线上（V2 兼容）命名：世界状态、卡片 JSON 与剧情包用的是同一套名字，
  // 前端不再各自翻译一遍，避免「线上叫 mes_example、界面按 mesExample 读」而永远取空。
  mes_example?: string;
  system_prompt?: string;
  post_history_instructions?: string;
  creator_notes?: string;
  tags?: string[];
  creator?: string;
  character_version?: string;
  nickname?: string;
}

export interface PromiseItem {
  promiseId: string;
  participantIds?: string[];
  content: string;
  sourceNodeId: string;
  state: "proposed" | "active" | "fulfilled" | "broken" | "cancelled";
  settledByReceiptId?: string;
}

export interface WorldState {
  relationships: Record<string, RelationValue>;
  items: Record<string, ItemInstance>;
  promises: Record<string, PromiseItem>;
  moods: Record<string, { moodCode: string; text: string }>;
  characters: Record<string, CharacterInfo>;
  scene?: { sceneId: string; title?: string } | null;
}

export interface SessionView {
	/** 当前查看节点的导演进度；完整大纲通过独立接口读取。 */
	director?: DirectorSummary;
  sessionId: string;
  characterId: string;
	actions?: ActionView[];
  title: string;
  rootNodeId: string;
  branch: BranchView; // 写入位置（提交基准）
  branches?: BranchView[];
  viewNodeId?: string; // 查看位置（浏览历史时与分支头不同）
  viewNode?: PlotNode;
  headNode: PlotNode;
  nodes?: PlotNode[]; // 祖先链（root 在前），用于重建完整故事
  /** parentId → 该父节点下的回合候选；仅在没有多个版本时省略（重生成产生）。 */
  candidateGroups?: Record<string, PlotNode[]>;
  state: WorldState | null;
  /** 世界观/秘密区块：已揭示的带内容，未揭示的只有标题（内容不下发，防剧透）。 */
  secrets?: SecretView[];
  /** 剧情大纲区块（M4f）：主线里程碑 + 角色目标 + 当前场景。 */
  outline?: OutlineView;
  /** 当前路径上生效的演义交接快照与摘要（M4b）。 */
  activeSummary?: SummaryArtifact | null;
  /** 窗口之外是否还有更早的回合（据此决定能否向上翻页）。 */
  hasMore?: boolean;
  /** 窗口内最早的回合节点，用作向上翻页游标。 */
  oldestTurnId?: string;
}

export interface SummaryArtifact {
  summaryId: string;
  fromNodeId: string;
  toNodeId: string;
  sourceHash: string;
  visibilityScope?: string;
  text: string;
  modelConfigVersion?: string;
  summaryVersion?: number;
}

/** 一条秘密/世界观条目的展示视图（M4d）。 */
export interface SecretView {
  secretId: string;
  title?: string;
  order?: number;
  /** 只在已揭示时非空。 */
  content?: string;
  revealed: boolean;
}

export interface DirectorBeat { beatId: string; title: string; instruction: string; completionCriteria: string }
export interface DirectorPlan { planId?: string; revisionId?: string; title: string; guidance: string; beats: DirectorBeat[] }
export interface DirectorEvidence { blockSeq: number; quote: string }
export interface DirectorState {
  plan: DirectorPlan;
  status: "active" | "paused" | "completed";
  currentBeatId: string;
  progress: Record<string, { status: "completed" | "skipped"; sourceNodeId: string; evidence?: DirectorEvidence[] }>;
  warning?: string;
}
export interface DirectorSummary {
  title: string; revisionId: string; status: DirectorState["status"];
  currentBeatId: string; currentBeatTitle: string; completed: number; total: number; warning?: string;
}
export interface DirectorDraft {
  sessionId: string; branchId: string; version: number; baseRevisionId: string; plan: DirectorPlan; updatedAt: string;
}
export interface DirectorRequest {
  requestId: string; sessionId: string; branchId: string; baseNodeId: string; draftVersion: number;
  text: string; status: "generating" | "completed" | "failed" | "cancelled" | "interrupted";
  reply?: string; candidate?: DirectorPlan; draftApplied: boolean; error?: string; createdAt: string;
}
export interface DirectorView {
  state: DirectorState | null; draft: DirectorDraft | null; requests: DirectorRequest[]; readOnly: boolean; viewNodeId: string;
}
export type DirectorAction = "activate" | "pause" | "resume" | "complete" | "skip" | "rewind";

export interface LorebookMatch {
  bookIndex: number;
  entryIndex: number;
  bookName: string;
  entry: { entryId?: string; title?: string; keys: string[]; secondaryKeys?: string[]; content: string; enabled: boolean };
}

export interface LorebookPage {
  entries: LorebookMatch[];
  hasMore: boolean;
  nextOffset: number;
}

/** 分支派生（分叉 / 重生成 / 编辑玩家输入）的受理结果。 */
export interface DeriveResult {
  branch: BranchView;
  branchId: string;
  turnId: string;
  status: string;
  statusUrl?: string;
  eventsUrl?: string;
}

/** 剧情包导入结果（后端分配新会话）。 */
export interface ImportResult {
  sessionId: string;
  title: string;
  rootNodeId: string;
  scope?: { full: boolean; branchId?: string; branchName?: string };
  counts?: Record<string, number>;
  assets?: number;
}

/** 编辑助手正文的结果：同步提交的纯叙事候选。 */
export interface AssistantEditResult {
  branch: BranchView;
  branchId: string;
  nodeId: string;
  mode: string;
}

export interface SessionSummary {
  sessionId: string;
  characterId: string;
  updatedAt?: string;
  title: string;
  createdAt: string;
  rootNodeId?: string;
}

export interface AcceptResult {
  turnId: string;
  status: string;
  statusUrl: string;
  eventsUrl: string;
}

export interface TurnView {
  turnId: string;
	sessionId?: string;
	input?: { kind: string; text: string; actionRef?: string; optionRef?: { nodeId: string; optionId: string } };
  branchId: string;
  status: string;
  mode: string;
  failureCode?: string;
  failureMessage?: string;
  resultNode?: PlotNode | null;
  // 在途正文快照：当前块的行内增量（Sequence 0）不会重放，断线/刷新后靠它补齐。
  draft?: TurnDraftSnapshot | null;
}

export interface TurnDraftSnapshot {
  attemptId: string;
  inFlight?: { seq: number; kind: string; speakerId?: string | null; text: string } | null;
}

export interface SSEEvent {
  id: string;
  event: string;
  data: unknown;
}

// ---- M1-5：模型配置 ----

// ProviderConfig 是**槽位解析视图**：槽位只保存 enabled + modelId，
// 其余字段是它引用到的模型实例的解析结果（modelId 为空时跟随主线）。
export interface ProviderConfig {
  slot: string; // primary | assist | reflection
  enabled: boolean;
  modelId?: string;
  resolvedModelId?: string;
  kind?: string; // openai-chat | openai-responses | anthropic-messages
  baseUrl?: string;
  model?: string;
  apiKey?: string; // 只写；读取为脱敏值
  hasApiKey?: boolean;
  temperature?: number | null;
  maxTokens?: number;
  contextWindow?: number;
  reasoningEffort?: string; // low | medium | high；空 = 默认（不发送思考参数）
}

// ModelInstance 是唯一携带连接信息的实体：新建一次，多个槽位引用。
export interface ModelInstance {
  id: string;
  name: string;
  kind: string;
  baseUrl?: string;
  model?: string;
  apiKey?: string; // 只写；读取为脱敏值（留空或回传掩码 = 保留原密钥）
  hasApiKey?: boolean;
  temperature?: number | null;
  maxTokens?: number;
  contextWindow?: number;
  reasoningEffort?: string; // low | medium | high；空 = 默认（不发送思考参数）
}

// ModelCatalog 是 GET /api/v1/config/models 的返回。
export interface ModelCatalog {
  models: ModelInstance[];
  slots: ProviderConfig[];
}

export interface ProbeResult {
  slot: string;
  ok: boolean;
  kind?: string;
  model?: string;
  connectMsg?: string;
  formatTested?: boolean;
  formatOk?: boolean;
  formatMsg?: string;
  latencyMs?: number;
}

// ---- M3：记忆（来源化认知记录）----

// MemoryView 是后端记忆管理视图。effective 表示该记录在查询所用分支上是否生效：
// 被覆盖记录取代的原记录为 false（修订采用 copy-on-write，原记录保持不可变）。
export interface MemoryView {
  memoryId: string;
  sourceNodeId: string;
  kind: "observed" | "reported" | "inferred" | "secret";
  ownerIds?: string[];
  content: string;
  entityIds?: string[];
  confidence?: number;
  pinned: boolean;
  // 该记录取代了哪条记忆（空 = 原始记录）
  supersedes?: string;
  hidden: boolean;
  effective: boolean;
  protected?: boolean;
  importance?: number;
  mergedFrom?: string[];
  evidence?: { sourceNodeId?: string; sourceQuote?: string; reasoning?: string; confidence: "high" | "medium" | "low"; autoDowngraded?: boolean };
  subjectKey?: string;
  createdTurn?: number;
  validFromTurn?: number;
  validUntilTurn?: number;
}

export interface MemoryUsage {
  used: number;
  protected: number;
  limit: number;
  reclaimable: number;
  headroom: number;
  tier: "Normal" | "Notice" | "Degraded" | "Critical";
}

// ---- M1-2：导入预览 ----

export interface CardPreview {
  /** 服务端卡库中的稳定 ID（导入成功时返回）。有它就可以只传 cardId 建会话。 */
  cardId?: string;
  shortName?: string;
  role?: string;
  format: string;
  /** 卡片 spec_version（原生卡为空）。 */
  specVersion?: string;
  /** 来源：png | json。 */
  source?: string;
  name: string;
  avatar?: string;
  description?: string;
  personality?: string;
  scenario?: string;
  firstMes?: string;
  mesExample?: string;
  systemPrompt?: string;
  postHistoryInstructions?: string;
  creatorNotes?: string;
  tags?: string[];
  creator?: string;
  characterVersion?: string;
  nickname?: string;
  supported: string[];
  ignored: string[];
  warnings: string[];
  characterJson: string;
  openings: { variantId: string; title: string; text: string }[];
}

/** 卡库摘要（列表接口返回，不含兆级 characterJson）。 */
export interface CardLibrarySummary {
  cardId: string;
  name: string;
  shortName?: string;
  avatar?: string;
  format?: string;
  role?: string;
  createdAt?: string;
  updatedAt?: string;
  lastUsedAt?: string;
}
/** 分支图谱（M4e）：以目标节点为中心的一圈子图。 */
export interface GraphNode {
  nodeId: string;
  parentId?: string;
  kind: string;
  turnNumber?: number;
  depth: number;
  /** >0 表示有未展开的子树（据此渲染"可展开"标记）。 */
  childCount?: number;
  isCandidate?: boolean;
}

export interface GraphView {
  sessionId: string;
  targetNodeId: string;
  nodes: GraphNode[];
  truncated?: boolean;
}

/** 剧情大纲（M4f）。 */
export interface OutlineView {
  milestones: OutlineEntry[];
  goals: GoalEntry[];
  scene?: string;
}

export interface OutlineEntry {
  milestoneId: string;
  description: string;
}

export interface GoalEntry {
  goalId: string;
  characterId: string;
  text: string;
}
