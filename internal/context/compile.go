package context

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// 世界书与记忆注入预算的默认值（首版可调）。
const (
	defaultMaxLorebookEntries  = 6    // 单轮最多注入的世界书条目数
	defaultMaxLorebookRunes    = 1800 // 单轮注入世界书内容的总字符上限
	defaultMinLorebookKeyRunes = 2    // 短于此长度的 key 不参与命中（单字 key 会命中几乎所有文本）

	// defaultHistoryCeiling 是未显式配置历史窗口时的兜底轮数。
	// 不再支持"无限历史"：那正是把整条祖先链搬进内存并逐个解析的来源。
	defaultHistoryCeiling = 200

	defaultMaxMemories            = 5    // 单轮最多注入的记忆条数（契约 §8.1 初始召回目标）
	defaultMaxMemoryRunes         = 1200 // 单轮注入记忆内容的总字符上限
	defaultMinMemoryBigramOverlap = 2    // 记忆与上下文的最少双字重合数（低于此视为不相关）
)

// CompilerOptions 控制编译行为（首版最小集）。
type CompilerOptions struct {
	MaxHistoryTurns int // 最近 N 个已提交回合进入正文历史

	// 世界书注入预算（关键词命中）。<=0 时取默认值。
	MaxLorebookEntries  int
	MaxLorebookRunes    int
	MinLorebookKeyRunes int

	// 记忆注入预算（相关度召回）。<=0 时取默认值。
	MaxMemories            int
	MaxMemoryRunes         int
	MinMemoryBigramOverlap int

	// 上下文预算（技术契约 §9.1）。
	// ContextWindow 来自配置或供应商信息。生产回合通过 WithBudget 绑定
	// 本次请求预算，未知窗口回退 8192、输出预留 2048。仅直接使用基础
	// Compiler（如未配置预算的单测）时 <=0 表示不裁剪。
	ContextWindow  int
	ReservedOutput int
	SafetyMargin   int

	// TokenCalibration 是 token 估算的校准系数：预算决策用 raw*factor。
	// 由真实用量（turn_usage.reported / estimated）反推后注入，<=0 表示不校准。
	// 账本里记录的仍是未校准值，否则系数会自我抵消（见 MessageTokens 注释）。
	TokenCalibration float64

	// CompactionPolicy 上下文压缩与修剪策略（M4b）。
	CompactionPolicy CompactionPolicy

	// SplitDynamicContext 启用 KV Cache 提示词前缀优化。
	// 为 true 时，静态系统规则/人设放在首条 system 消息中（保持前缀不变以复用缓存），
	// 动态的世界书、记忆、骰点检定、解锁秘密、当前背包/好感等信息拆分到正文后的动态上下文提示中。
	//
	// 默认 true。此前默认 false 导致该优化在生产从未生效：合并模式下动态状态与
	// 静态人设同处一条 system 消息，而前缀缓存按位置匹配——system 内部一旦出现差异，
	// 其后的开场白与全量历史即使字节相同也无法命中缓存。需要单 system 消息兼容时
	// 可显式置 false（TestSplitDynamicContext 覆盖该路径）。
	SplitDynamicContext bool

	// TemporalKeywords 时序意图识别词表（T2.1）。
	// 当用户输入命中词表中的时间指示词时进入时序检索模式，强化最新事实并对同实体记忆取最新者置顶。
	TemporalKeywords []string

	// AllowUnrankedFallback 允许未命中词法/实体的保底记忆以低分参与候选（T2.2，防语义改写漏召回）。
	AllowUnrankedFallback bool

	// MaxUnrankedFallback 未命中词法/实体的保底记忆最大保留条数（默认 2）。
	MaxUnrankedFallback int

	// MinFallbackImportance 保底记忆的最低重要度门槛（默认 7）。低于此门槛的非命中记忆不进入上下文，防止叙事噪声。
	MinFallbackImportance int

	// OnBudget 在预算裁剪后回调，回报裁剪了什么（观测与排障用）。
	OnBudget func(report BudgetReport)

	// DisableMemoryInjection / DisableLorebookInjection / DisableSummaries 是
	// 消融开关的落点（ADS-7.8-01），只由 Ablation.Apply 在启动阶段置位。
	//
	// 为什么不复用 "预算设成 0 即禁用"：预算字段 <=0 的语义是"取默认值"，
	// 用它表达禁用会让"我想关掉"和"我忘了配"变成同一种状态——评估里
	// 一个拼错的开关名就会静默变成"其实没关"。
	DisableMemoryInjection   bool
	DisableLorebookInjection bool
	DisableSummaries         bool

	// OnPhase 在编译各阶段结束时回调，用于把"准备耗时"归因到具体阶段。
	// 默认 nil：不做任何计时，生产路径零开销。
	OnPhase func(phase string, d time.Duration)
}

// DefaultOptions 返回初始配置。
// 为现代 128K~300K 上下文模型深度优化：默认加载最近 50 轮完整历史，结合 20 轮心声与 20 轮压缩策略。
func DefaultOptions() CompilerOptions {
	return CompilerOptions{
		MaxHistoryTurns:     50,
		MaxLorebookEntries:  defaultMaxLorebookEntries,
		MaxLorebookRunes:    defaultMaxLorebookRunes,
		MinLorebookKeyRunes: defaultMinLorebookKeyRunes,

		MaxMemories:            defaultMaxMemories,
		MaxMemoryRunes:         defaultMaxMemoryRunes,
		MinMemoryBigramOverlap: defaultMinMemoryBigramOverlap,
		CompactionPolicy:       DefaultCompactionPolicy(),
		SplitDynamicContext:    true,
		TemporalKeywords:       defaultTemporalKeywords(),
		AllowUnrankedFallback:  true,
		MaxUnrankedFallback:    2,
		MinFallbackImportance:  7,
	}
}

// Compiler 编译上下文请求。
type Compiler struct {
	store               ports.CompileDeps
	options             CompilerOptions
	planningInstruction string
	planningMessages    []ports.ChatMessage
}

// MinInputBudget 是小窗口测试的参考值，不是 InputBudget 的强制下限。
const MinInputBudget = 2048

// InputBudget 计算输入预算（技术契约 §9.1）：
//
//	inputBudget = contextWindow - reservedOutput - safetyMargin
//
// ContextWindow 未知（<=0）时返回 0，调用方据此**跳过**预算裁剪，
// 生产调用先由 WithBudget 设置保守窗口。检查轮次不能代替预算计算。
func (o CompilerOptions) InputBudget() int {
	if o.ContextWindow <= 0 {
		return 0
	}
	b := o.ContextWindow
	if o.ReservedOutput > 0 {
		b -= o.ReservedOutput
	}
	if o.SafetyMargin > 0 {
		b -= o.SafetyMargin
	}
	return b
}

// WithTokenCalibration 在 attempt 副本上设置估算校准系数（见 calibration 说明）。
func (c *Compiler) WithTokenCalibration(factor float64) *Compiler {
	copy := *c
	copy.options.TokenCalibration = factor
	return &copy
}

// WithBudgetObserver 在 attempt 副本上注册预算裁剪回调（观测用）。
//
// 存在的理由：BudgetReport 一直算得很清楚，却没有任何生产消费者——
// "这一轮悄悄丢掉了哪些历史"在运行期完全不可见。装配处把回调接到运行读数上。
func (c *Compiler) WithBudgetObserver(onBudget func(BudgetReport)) *Compiler {
	copy := *c
	copy.options.OnBudget = onBudget
	return &copy
}

// WithPhaseObserver 在 attempt 副本上注册阶段耗时回调（观测用）。
//
// Compile 内部本来就按阶段计时（beginPhase/endPhase），但生产路径没有消费者，
// 于是"准备一条请求慢在哪一段"只能靠猜。装配处把回调接到运行读数上，
// 得到的是比整轮耗时更细、又不必引入完整追踪栈的现场。
func (c *Compiler) WithPhaseObserver(onPhase func(phase string, d time.Duration)) *Compiler {
	copy := *c
	copy.options.OnPhase = onPhase
	return &copy
}

// Options 返回当前生效的编译选项快照（只读）。
//
// 供组合根读取消融后的策略：例如压缩服务必须与编译器用同一份 CompactionPolicy，
// 否则"关掉压缩"只关掉了一半——编译侧不再折叠，后台仍在生成摘要。
func (c *Compiler) Options() CompilerOptions {
	return c.options
}

// WithBudget returns an attempt-local compiler; concurrent turns never mutate
// shared compiler options. Unknown providers use an explicit conservative limit.
func (c *Compiler) WithBudget(window, reserved int) *Compiler {
	copy := *c
	if window <= 0 {
		window = 8192
	}
	if reserved <= 0 {
		reserved = 2048
	}
	copy.options.ContextWindow = window
	copy.options.ReservedOutput = reserved
	copy.options.SafetyMargin = max(512, window/20)
	return &copy
}

// EstimateTokens 估算一段文本的 token 数。
//
// 契约明确禁止"用中文字符数除固定常数当作准确 token 数"——不同供应商的
// 分词器差异很大。这里是结合安全余量使用的启发式估算，不能保证是严格
// 上界；真实用量应从供应商回传的 usage 观测校准。
//
// 规则：CJK 每字符 1 token（现代分词器对中文常见字接近 1:1，生僻词会更贵）；
// 拉丁/数字/标点按 3 字符 1 token；空白不计。
func EstimateTokens(s string) int {
	if s == "" {
		return 0
	}
	cjk, other := 0, 0
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Han, r), unicode.Is(unicode.Hiragana, r),
			unicode.Is(unicode.Katakana, r), unicode.Is(unicode.Hangul, r):
			cjk++
		case r == ' ', r == '\n', r == '\r', r == '\t':
			// 空白不计：分词器几乎不为空白单独出 token
		default:
			other++
		}
	}
	return cjk + (other+2)/3
}

// New 创建编译器。世界书与记忆相关预算为 0 时补默认值；
// MaxHistoryTurns 保持既有语义（<=0 表示不截断历史）。
func New(store ports.Store, options CompilerOptions) *Compiler {
	if options.MaxLorebookEntries <= 0 {
		options.MaxLorebookEntries = defaultMaxLorebookEntries
	}
	if options.MaxLorebookRunes <= 0 {
		options.MaxLorebookRunes = defaultMaxLorebookRunes
	}
	if options.MinLorebookKeyRunes <= 0 {
		options.MinLorebookKeyRunes = defaultMinLorebookKeyRunes
	}
	if options.MaxMemories <= 0 {
		options.MaxMemories = defaultMaxMemories
	}
	if options.MaxMemoryRunes <= 0 {
		options.MaxMemoryRunes = defaultMaxMemoryRunes
	}
	if options.MinMemoryBigramOverlap <= 0 {
		options.MinMemoryBigramOverlap = defaultMinMemoryBigramOverlap
	}
	return &Compiler{store: store, options: options}
}

// Compile 在给定基准节点与状态下构造请求消息。
// 玩家身份、开场白与世界书均从根节点读取，不由调用方传递——
// 避免调用方传错字面量（历史缺陷：playerName 被硬编码为 "player"）。
// Compile 构造模型请求。
//
// checks 是**准备阶段已经定下**的检定结果（技术契约 §5.2）。模型只能引用它们
// 并按结果演绎，**不能自己编一个结果**——随机性在后端，模型与客户端都无权指定。
// 传空切片表示本回合没有检定。
func (c *Compiler) Compile(ctx context.Context, sessionID, nodeID, inputText string, state *domain.WorldState, checks []domain.CheckResult) (ports.ChatRequest, error) {
	req := ports.ChatRequest{
		Model:     "primary",
		MaxTokens: 2048,
	}
	t0 := c.beginPhase("session_context")
	sc := c.loadSessionContext(sessionID)
	if c.planningInstruction == "" {
		var err error
		sc.Director, err = c.store.DirectorAt(nodeID)
		if err != nil {
			return req, err
		}
	}
	c.endPhase("session_context", t0)

	// 正文历史：只取最近若干轮，窗口裁剪在 SQL 侧完成。
	// 早期实现先拉整条祖先链再截断，代价随链长线性增长（万级节点实测 125ms，
	// 见 docs/PERF_BASELINE.md），而其中绝大多数节点随后就被丢掉了。
	t0 = c.beginPhase("ancestor_chain")
	historyLimit := c.options.MaxHistoryTurns
	if historyLimit <= 0 {
		historyLimit = defaultHistoryCeiling
	}
	ancestors, err := c.store.RecentTurnNodes(nodeID, historyLimit)
	if err != nil {
		return req, err
	}
	c.endPhase("ancestor_chain", t0)

	// 世界书命中与记忆召回共用同一段检索文本（开场白 + 最近正文 + 当前输入）。
	t0 = c.beginPhase("lorebook")
	haystack := buildHaystack(sc.OpeningText, ancestors, inputText)
	lore := c.matchLorebook(sc, haystack)
	if c.options.DisableLorebookInjection {
		lore = nil
	}
	c.endPhase("lorebook", t0)

	// 记忆召回：候选池与词法排名都由检索层给出（SQL 侧路径过滤 + FTS5），
	// 不再把整条路径的记忆搬进 Go 逐条打分——那是万级记忆下 134ms 的来源。
	t0 = c.beginPhase("memory_recall")
	currentTurn := 0
	if node, nerr := c.store.GetNode(nodeID); nerr == nil && node != nil {
		currentTurn = node.TurnNumber
	}
	memories, err := c.recall(nodeID, haystack, memoryQueryText(sc.OpeningText, ancestors, inputText), state, currentTurn, inputText)
	if err != nil {
		return req, err
	}
	if c.options.DisableMemoryInjection {
		// 消融：记忆不进上下文。检索本身照跑——它便宜且能保证耗时口径可比，
		// 只把"是否注入"这一个变量隔离出来。
		memories = nil
	}
	c.endPhase("memory_recall", t0)

	// 注入清单随请求上行：提交时据此判定 lastMeaningfulMentionTurn（§8.1）。
	req.InjectedMemoryIDs = memoryIDsOf(memories)

	// 相关摘要（技术契约 §9.2）：只采用来源区间仍在当前路径上的（T24）。
	// 只折叠摘要确实覆盖且位于保护尾部之外的回合；区间之间的空缺保留原文。
	var summaries []*domain.SummaryArtifact
	if !c.options.DisableSummaries {
		if sums, serr := c.store.SummariesOnPath(nodeID); serr == nil && len(sums) > 0 {
			summaries = sums
		}
	}

	// 历史大检定账本（M4h）：把祖先路径上已结算的判定收据编译成不可被
	// 叙事反转的硬下界，作为后续注入的权威真值（受 fitToBudget 预算约束）。
	t0 = c.beginPhase("ledger")
	ledger, secretUnlockTurns, err := c.compileLedger(nodeID, state, sc)
	if err != nil {
		return req, err
	}
	for i := range sc.Secrets {
		if turn, ok := secretUnlockTurns[sc.Secrets[i].SecretID]; ok && turn > 0 {
			sc.Secrets[i].UnlockTurn = turn
		}
	}
	c.endPhase("ledger", t0)
	// The exact same resolved tail controls folding, trimming and the pressure
	// compaction request. A compiler is attempt-local, never mutated globally.
	local := *c
	mandatory := c.EstimateMessages(c.buildMessages(nil, nil, nil, nil, checks, sc, state, inputText, ledger, 0))
	local.options.CompactionPolicy = ResolveTailPolicy(ancestors, c.options.CompactionPolicy, c.options.InputBudget()-mandatory, c.options.ContextWindow > 0, c.options.TokenCalibration)
	c = &local
	if len(summaries) > 0 {
		ancestors, err = c.foldSummarizedHistory(ancestors, summaries)
		if err != nil {
			return req, err
		}
	}

	// 上下文预算（技术契约 §9.1）：按分配顺序收敛可压缩材料。
	// 未配置窗口时跳过——预算未知时臆测上限比明确说"未启用"更危险。
	t0 = c.beginPhase("budget")
	lore, memories, ancestors, summaries, breport, berr := c.fitBundleBudget(lore, memories, ancestors, summaries, checks, sc, state, inputText, ledger)
	if berr != nil {
		return req, berr
	}
	c.endPhase("budget", t0)
	if c.options.OnBudget != nil {
		c.options.OnBudget(breport)
	}
	req.InjectedMemoryIDs = memoryIDsOf(memories)
	req.InputBudget = breport.Budget
	req.EstimatedInputTokens = breport.Total
	req.NeedsCompaction = breport.PrepareCompression
	req.ProtectedTurns = c.options.CompactionPolicy.TailWindowTurns
	if c.options.ReservedOutput > 0 {
		req.MaxTokens = c.options.ReservedOutput
	}
	req.Messages = c.buildMessages(lore, memories, ancestors, summaries, checks, sc, state, inputText, ledger, breport.DroppedHistory)
	return req, nil
}

// beginPhase 记录阶段起点。未启用 OnPhase 时返回零值，endPhase 会直接跳过——
// 因此未开启观测时 Compile 不付出任何计时成本。
func (c *Compiler) beginPhase(name string) time.Time {
	if c.options.OnPhase == nil {
		return time.Time{}
	}
	return time.Now()
}

func (c *Compiler) endPhase(name string, start time.Time) {
	if c.options.OnPhase == nil || start.IsZero() {
		return
	}
	c.options.OnPhase(name, time.Since(start))
}

// loadSessionContext 从会话根节点读取编译所需的稳定信息：
// 开场白、玩家身份与世界书。
// 走根节点快照而非卡片文件，保证 T20 的语义：模板后续变更只影响新会话。
func (c *Compiler) loadSessionContext(sessionID string) sessionContext {
	sc := sessionContext{Rules: domain.DefaultRuleset()}
	sess, err := c.store.GetSession(sessionID)
	if err != nil || sess == nil || sess.RootNodeID == "" {
		return sc
	}
	root, err := c.store.GetNode(sess.RootNodeID)
	if err != nil || root == nil {
		return sc
	}
	var rc struct {
		OpeningText string `json:"openingText"`
		Templates   map[string]struct {
			TemplateVersionID string `json:"templateVersionId"`
			Content           struct {
				Name string `json:"name"`
				Role string `json:"role"`
			} `json:"content"`
		} `json:"templates"`
	}
	if err := json.Unmarshal([]byte(root.ContentJSON), &rc); err != nil {
		return sc
	}
	sc.OpeningText = rc.OpeningText
	if ref := rc.Templates["rules"]; ref.TemplateVersionID != "" {
		if tv, err := c.store.GetTemplateVersion(ref.TemplateVersionID); err == nil {
			if err := json.Unmarshal([]byte(tv.Content), &sc.Rules); err != nil {
				slog.Warn("模板规则解析失败，按空规则继续", "templateVersionId", ref.TemplateVersionID, "error", err)
			}
		}
	}
	if p, ok := rc.Templates["player"]; ok {
		sc.PlayerName = strings.TrimSpace(p.Content.Name)
		sc.PlayerRole = strings.TrimSpace(p.Content.Role)
	}

	// 秘密定义在角色模板（卡片 JSON）里。这里只取 id/标题/内容三项；
	// 是否注入由调用方按 WorldState.UnlockedSecrets 过滤——
	// 编译层不评估揭示条件，那属于提交路径（M4d）。
	if ch, ok := rc.Templates["character"]; ok && ch.TemplateVersionID != "" {
		if tv, terr := c.store.GetTemplateVersion(ch.TemplateVersionID); terr == nil && tv != nil && tv.Content != "" {
			var card struct {
				Secrets []struct {
					SecretID string `json:"secretId"`
					Title    string `json:"title"`
					Content  string `json:"content"`
				} `json:"secrets"`
			}
			if json.Unmarshal([]byte(tv.Content), &card) == nil {
				for _, sec := range card.Secrets {
					if sec.Content == "" {
						continue
					}
					sc.Secrets = append(sc.Secrets, secretEntry{
						SecretID: sec.SecretID, Title: sec.Title, Content: sec.Content,
					})
				}
			}
		}
	}

	ref, ok := rc.Templates["lorebook"]
	if !ok || ref.TemplateVersionID == "" {
		return sc
	}
	tv, err := c.store.GetTemplateVersion(ref.TemplateVersionID)
	if err != nil || tv == nil || tv.Kind != domain.TemplateLorebook {
		return sc
	}
	var books []domain.Lorebook
	if err := json.Unmarshal([]byte(tv.Content), &books); err != nil {
		return sc
	}
	sc.Lorebooks = books
	return sc
}

func itoa(n int) string { return strconv.Itoa(n) }
